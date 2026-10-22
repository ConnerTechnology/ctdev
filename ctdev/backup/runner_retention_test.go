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

// retentionStandin gives m a destination whose restic backs up and, while
// the file it returns exists, fails every forget.
func retentionStandin(t *testing.T, m *Machine) (failForget string) {
	t.Helper()
	dir := t.TempDir()
	failForget = filepath.Join(dir, "fail-forget")
	script := filepath.Join(dir, "restic")
	body := `#!/bin/sh
case "$1" in
version) echo "restic 0.19.1 compiled with go1.26.4 on linux/amd64" ;;
backup) echo '{"message_type":"summary","snapshot_id":"0123456789abcdef","total_files_processed":1,"total_bytes_processed":1,"data_added":1,"backup_end":"2026-10-22T12:00:00Z"}' ;;
snapshots) echo '[]' ;;
forget) if [ -e "` + failForget + `" ]; then echo "Fatal: example failure" >&2; exit 1; fi ;;
esac
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(failForget, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	m.Restic = script
	env := m.destinationFile("example-backups")
	if err := os.MkdirAll(filepath.Dir(env), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env, []byte("RESTIC_REPOSITORY="+filepath.Join(dir, "repo")+"\nCTDEV_COMPUTER_KEY=example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return failForget
}

// A keep-rules failure doesn't make the backup it followed a failure: the
// run succeeds, the folder isn't failing and nothing notifies, and the
// failure is recorded on its own until a later run's keep rules succeed.
func TestARetentionFailureIsRecordedApartFromTheBackup(t *testing.T) {
	m, root, n := failingMachine(t)
	failForget := retentionStandin(t, m)

	results, err := m.RunScheduled(context.Background())
	if err != nil {
		t.Errorf("err = %v, want none: the backup succeeded", err)
	}
	if len(results) != 1 || results[0].Outcome != BackedUp || results[0].Snapshot.ShortID != "01234567" ||
		results[0].RetentionErr == nil {
		t.Errorf("results = %+v, want the backup made and the keep-rules failure", results)
	}
	r, _, _ := m.lastRun(root)
	if !r.LastSuccess.Equal(noon) || r.Failed() || !strings.Contains(r.RetentionError, "example failure") {
		t.Errorf("state = %+v, want a success at %v with the keep-rules failure apart", r, noon)
	}
	if h := health(t, m); h[0].State != HealthOK {
		t.Errorf("health = %q, want ok", h[0].State)
	}
	if len(n.sent) != 0 {
		t.Errorf("notifications = %v, want none", n.sent)
	}

	// The next run's keep rules succeed, and the failure is cleared.
	if err := os.Remove(failForget); err != nil {
		t.Fatal(err)
	}
	at(m, noon.Add(25*time.Hour))
	if _, err := m.RunScheduled(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r, _, _ := m.lastRun(root); r.RetentionError != "" {
		t.Errorf("state = %+v, want the keep-rules failure cleared", r)
	}
}

// Back up now records a keep-rules failure the same way a scheduled run does.
func TestRetainRecordsAndClearsTheFailure(t *testing.T) {
	m, root, _ := failingMachine(t)
	failForget := retentionStandin(t, m)
	d, err := m.Destination("example-backups")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ReadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Retain(context.Background(), d, root, cfg); err == nil {
		t.Fatal("want the forget's failure")
	}
	if r, _, _ := m.lastRun(root); !strings.Contains(r.RetentionError, "example failure") {
		t.Errorf("state = %+v, want the keep-rules failure", r)
	}
	if err := os.Remove(failForget); err != nil {
		t.Fatal(err)
	}
	if err := m.Retain(context.Background(), d, root, cfg); err != nil {
		t.Fatal(err)
	}
	if r, _, _ := m.lastRun(root); r.RetentionError != "" {
		t.Errorf("state = %+v, want the keep-rules failure cleared", r)
	}
}
