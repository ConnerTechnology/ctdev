//go:build unix

package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	exampleStatus1 = `{"message_type":"status","percent_done":0.25,"total_files":4,"files_done":1,"total_bytes":4000,"bytes_done":1000,"seconds_remaining":30,"current_files":["/notes.txt"]}`
	exampleStatus2 = `{"message_type":"status","percent_done":0.5,"total_files":4,"files_done":2,"total_bytes":4000,"bytes_done":2000,"seconds_remaining":15}`
	exampleSummary = `{"message_type":"summary","snapshot_id":"0123456789abcdef","total_files_processed":4,"total_bytes_processed":4000,"data_added":900,"backup_end":"2026-10-22T14:15:00Z"}`
)

var exampleProgress = []BackupProgress{
	{Percent: 0.25, FilesDone: 1, FilesTotal: 4, BytesDone: 1000, BytesTotal: 4000, SecondsLeft: 30, CurrentFile: "/notes.txt"},
	{Percent: 0.5, FilesDone: 2, FilesTotal: 4, BytesDone: 2000, BytesTotal: 4000, SecondsLeft: 15},
}

// backupWithProgress backs up the failingMachine's folder with a stand-in
// restic whose backup prints two status lines and the summary, then runs
// after, collecting the progress reported.
func backupWithProgress(t *testing.T, after string) (*Machine, string, Snapshot, []BackupProgress, error) {
	t.Helper()
	m, root, _ := failingMachine(t)
	standinDestination(t, m, `backup)
  echo '`+exampleStatus1+`'
  echo '`+exampleStatus2+`'
  echo '`+exampleSummary+`'
  `+after+` ;;`)
	d, err := m.Destination("example-backups")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ReadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []BackupProgress
	snap, err := m.BackupNow(context.Background(), d, root, &cfg, func(p BackupProgress) { got = append(got, p) })
	return m, root, snap, got, err
}

func TestBackupNowReportsProgressAndStillReadsTheSummary(t *testing.T) {
	m, root, snap, got, err := backupWithProgress(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, exampleProgress) {
		t.Errorf("progress = %+v, want %+v", got, exampleProgress)
	}
	if snap.ID != "0123456789abcdef" || snap.Files != 4 || snap.Size != 4000 || snap.Added != 900 || snap.Warning != "" {
		t.Errorf("snapshot = %+v, want the summary's", snap)
	}
	if r, _, _ := m.lastRun(root); r.Failed() {
		t.Errorf("state = %+v, want a successful run", r)
	}
}

// Exit 3 is restic's "snapshot made, some files unreadable".
func TestPartialBackupWithProgressKeepsTheSummaryAndWarns(t *testing.T) {
	_, _, snap, got, err := backupWithProgress(t, "exit 3")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(exampleProgress) {
		t.Errorf("progress = %+v, want %d reports", got, len(exampleProgress))
	}
	if snap.ID != "0123456789abcdef" || snap.Warning != errPartial.Error() {
		t.Errorf("snapshot = %+v, want the summary with a warning", snap)
	}
}

// Under a full-screen program, restic leads its own session, so it has no
// controlling terminal for an SFTP destination's ssh to take over; otherwise
// it stays in ctdev's, where ssh can ask for a host key or a passphrase.
func TestResticLeavesTheTerminalOnlyWhenAsked(t *testing.T) {
	for _, noTerminal := range []bool{false, true} {
		m, root, _ := failingMachine(t)
		m.NoTerminal = noTerminal
		// A new session makes restic its own process group's leader too, and
		// BSD ps (macOS) has no sid keyword, so the group stands in for it.
		marker := filepath.Join(t.TempDir(), "session")
		standinDestination(t, m, `backup)
  ps -o pgid= -p $$ | tr -d ' ' > "`+marker+`"
  echo $$ >> "`+marker+`"
  echo '`+exampleSummary+`' ;;`)
		d, err := m.Destination("example-backups")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.BackupNow(context.Background(), d, root, nil, nil); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(marker)
		if err != nil {
			t.Fatal(err)
		}
		f := strings.Fields(string(data))
		if len(f) != 2 {
			t.Fatalf("restic's session and pid: %q", data)
		}
		if leads := f[0] == f[1]; leads != noTerminal {
			t.Errorf("NoTerminal %v: restic's session and pid %q, want it to lead its own session: %v", noTerminal, data, noTerminal)
		}
	}
}

// Cancelling interrupts restic, so it can remove its lock, instead of
// killing it, with or without the terminal.
func TestCancelledBackupInterruptsRestic(t *testing.T) {
	for _, noTerminal := range []bool{false, true} {
		m, root, _ := failingMachine(t)
		m.NoTerminal = noTerminal
		dir := t.TempDir()
		fifo, marker := filepath.Join(dir, "started"), filepath.Join(dir, "signal")
		if err := syscall.Mkfifo(fifo, 0o600); err != nil {
			t.Fatal(err)
		}
		standinDestination(t, m, `backup)
  trap 'echo INT > "`+marker+`"; kill $!; exit 130' INT
  sleep 60 >/dev/null 2>&1 &
  echo started > "`+fifo+`"
  wait $! ;;`)
		d, err := m.Destination("example-backups")
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			_, _ = os.ReadFile(fifo) // blocks until the backup has started
			cancel()
		}()
		if _, err := m.BackupNow(ctx, d, root, nil, nil); err == nil {
			t.Fatal("want the cancelled backup to fail")
		}
		cancel()
		if got, _ := os.ReadFile(marker); string(got) != "INT\n" {
			t.Errorf("NoTerminal %v: restic got %q, want an interrupt", noTerminal, got)
		}
	}
}

// Cancelling a Back up now stops restic, and that isn't a failed backup:
// the folder keeps its last recorded run.
func TestCancelledBackupIsNotRecordedAsFailed(t *testing.T) {
	m, root, _ := failingMachine(t)
	if err := m.recordRun(root, noon.Add(-time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	standinDestination(t, m, `backup)
  echo '`+exampleStatus1+`'
  exec sleep 30 ;;`)
	d, err := m.Destination("example-backups")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ReadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err = m.BackupNow(ctx, d, root, &cfg, func(BackupProgress) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	r, _, err := m.lastRun(root)
	if err != nil {
		t.Fatal(err)
	}
	if r.Failed() || !r.LastAttempt.Equal(noon.Add(-time.Hour)) {
		t.Errorf("last run = %+v, want the earlier success untouched", r)
	}
}

// Cancelling the keep rules after a Back up now isn't a retention failure.
func TestCancelledRetainIsNotRecorded(t *testing.T) {
	m, root, _ := failingMachine(t)
	if err := m.recordRun(root, noon.Add(-time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	standinDestination(t, m, `forget) echo "Fatal: example failure" >&2; exit 1 ;;`)
	d, err := m.Destination("example-backups")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Retain(ctx, d, root, NewConfig("example-backups")); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if r, _, _ := m.lastRun(root); r.RetentionError != "" || r.RetentionDeferred {
		t.Errorf("last run = %+v, want no retention outcome recorded", r)
	}
}
