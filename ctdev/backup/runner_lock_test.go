package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A second scheduled run while one holds the lock exits quietly.
func TestASecondRunScheduledExitsWhileOneRuns(t *testing.T) {
	m, root, n := failingMachine(t)
	other := &Machine{Dir: m.Dir}
	unlock, err := other.lock(false)
	if err != nil {
		t.Fatal(err)
	}
	results, err := m.RunScheduled(context.Background())
	unlock()
	if err != nil || len(results) != 0 || len(n.sent) != 0 {
		t.Errorf("while locked: results %v, err %v, notifications %v; want nothing", results, err, n.sent)
	}
	if r, ok, _ := m.lastRun(root); ok {
		t.Errorf("while locked, state was written: %+v", r)
	}
	if results, _ := m.RunScheduled(context.Background()); len(results) != 1 {
		t.Errorf("after the lock is released: %v, want the folder run", results)
	}
}

// A dry run says what would happen and writes nothing: no state, no lock
// file, no schedule change.
func TestRunScheduledDryRunWritesNothing(t *testing.T) {
	m, root, n := failingMachine(t)
	gone := filepath.Join(m.Home, "gone")
	listed(t, m, gone, NewConfig("example-backups"))
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	// The config now says weekly; a real run would copy that to the list.
	cfg, err := ReadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Schedule = Weekly
	writeConfig(t, root, cfg)
	before, err := os.ReadFile(m.foldersFile())
	if err != nil {
		t.Fatal(err)
	}
	// listed took the lock to write folders.json; start from no lock file.
	if err := os.Remove(m.lockFile()); err != nil {
		t.Fatal(err)
	}

	m.Opts.DryRun = true
	results, _ := m.RunScheduled(context.Background())
	outcome := map[string]Outcome{}
	for _, r := range results {
		outcome[r.Path] = r.Outcome
	}
	if outcome[gone] != Missing || outcome[root] != Failed {
		t.Errorf("outcomes = %v, want gone missing and the folder failed (no destination)", outcome)
	}
	for _, f := range []string{m.stateFile(), m.lockFile()} {
		if _, err := os.Stat(f); err == nil {
			t.Errorf("a dry run wrote %s", f)
		}
	}
	if after, _ := os.ReadFile(m.foldersFile()); string(after) != string(before) {
		t.Errorf("a dry run changed folders.json:\n%s", after)
	}
	if len(n.sent) != 0 {
		t.Errorf("a dry run notified: %v", n.sent)
	}
}

// A folder moved and configured in its new place replaces its old entry,
// rather than leaving it missing for ever.
func TestAMovedFolderConfiguredAgainIsNotLeftMissing(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	m.Notifier = &fakeNotifier{}
	d, _ := createExample(t, m)
	old := exampleFolder(t, m)
	cfg := NewConfig(d.Name)
	at(m, noon.Add(-2*time.Hour))
	if err := m.Configure(ctx, d, old, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RunScheduled(ctx); err != nil {
		t.Fatal(err)
	}

	moved := filepath.Join(m.Home, "Sites", "website")
	if err := os.MkdirAll(filepath.Dir(moved), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(old, moved); err != nil {
		t.Fatal(err)
	}
	at(m, noon.Add(-time.Hour))
	if _, err := m.RunScheduled(ctx); err == nil {
		t.Fatal("the run after the move didn't report the folder missing")
	}
	if h := health(t, m); len(h) != 1 || h[0].State != HealthMissing {
		t.Fatalf("after the move: %+v, want the old place missing", h)
	}

	at(m, noon)
	if err := m.Configure(ctx, d, moved, cfg); err != nil {
		t.Fatal(err)
	}
	results, err := m.RunScheduled(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Path != moved || results[0].Outcome != BackedUp {
		t.Errorf("after Configure in the new place: %+v, want only it, backed up", results)
	}
	h := health(t, m)
	if len(h) != 1 || h[0].Path != moved || h[0].State != HealthOK {
		t.Errorf("health = %+v, want only the new place, OK", h)
	}
	if runs, _ := m.runs(); len(runs) != 1 {
		t.Errorf("state.json still has the old place: %v", runs)
	}
}

// A copy of a folder (the same ID at a path that still exists) isn't
// dropped when the other is configured.
func TestAddFolderKeepsACopyThatStillExists(t *testing.T) {
	m := testMachine(t, "example-laptop")
	cfg := NewConfig("example-backups")
	a, b := filepath.Join(m.Home, "a"), filepath.Join(m.Home, "b")
	listed(t, m, a, cfg)
	listed(t, m, b, cfg)
	if h := health(t, m); len(h) != 2 {
		t.Errorf("health = %+v, want both copies", h)
	}
}
