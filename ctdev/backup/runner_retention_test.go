package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A scheduled run applies the folder's keep rules after its backup, with a
// real restic: keeping one hourly backup leaves one after two runs made in
// the same hour.
func TestRunScheduledAppliesRetention(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	m.Notifier = &fakeNotifier{}
	d, _ := createExample(t, m)
	root := exampleFolder(t, m)
	cfg := NewConfig(d.Name)
	cfg.Schedule, cfg.Keep = Hourly, Keep{Hourly: 1}
	at(m, noon)
	if err := m.Configure(ctx, d, root, cfg); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		at(m, noon.Add(time.Duration(i)*time.Hour))
		if err := os.WriteFile(filepath.Join(root, "change.txt"), []byte{byte('a' + i)}, 0o644); err != nil {
			t.Fatal(err)
		}
		results, err := m.RunScheduled(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 || results[0].Outcome != BackedUp {
			t.Fatalf("run %d: %+v, want a backup", i, results)
		}
	}
	snaps, err := d.Snapshots(ctx, m.Ref(root, &cfg))
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 {
		t.Errorf("%d backups after two runs keeping 1 hourly, want 1", len(snaps))
	}
}

// A keep-rules failure is the run's error, but the backup it followed
// stands: it stays the last success.
func TestARetentionFailureDoesntUndoTheBackup(t *testing.T) {
	m, root, n := failingMachine(t)
	script := filepath.Join(t.TempDir(), "restic")
	body := `#!/bin/sh
case "$1" in
version) echo "restic 0.19.1 compiled with go1.26.4 on linux/amd64" ;;
backup) echo '{"message_type":"summary","snapshot_id":"0123456789abcdef","total_files_processed":1,"total_bytes_processed":1,"data_added":1,"backup_end":"2026-10-22T12:00:00Z"}' ;;
forget) echo "Fatal: example failure" >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	m.Restic = script
	env := m.destinationFile("example-backups")
	if err := os.MkdirAll(filepath.Dir(env), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env, []byte("RESTIC_REPOSITORY="+filepath.Join(t.TempDir(), "repo")+"\nCTDEV_COMPUTER_KEY=example\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	results, err := m.RunScheduled(context.Background())
	if err == nil || !strings.Contains(err.Error(), "keep rules") {
		t.Errorf("err = %v, want the keep-rules failure", err)
	}
	if len(results) != 1 || results[0].Outcome != BackedUp || results[0].Snapshot.ShortID != "01234567" {
		t.Errorf("results = %+v, want the backup made", results)
	}
	r, _, _ := m.lastRun(root)
	if !r.LastSuccess.Equal(noon) || !strings.Contains(r.Error, "keep rules") {
		t.Errorf("state = %+v, want a success at %v with the keep-rules error", r, noon)
	}
	if len(n.sent) != 1 {
		t.Errorf("notifications = %v, want one failing", n.sent)
	}
}
