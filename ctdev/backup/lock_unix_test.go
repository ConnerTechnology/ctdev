//go:build unix

package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// The state lock excludes another *Machine, as it would another process:
// while one holds it, the lock file can't be taken, and a write from the
// other waits for the unlock and then lands.
func TestStateLockExcludesAnotherMachine(t *testing.T) {
	m := testMachine(t, "example-laptop")
	other := &Machine{Dir: m.Dir}
	unlock, err := other.stateLock()
	if err != nil {
		t.Fatal(err)
	}
	probe, err := os.OpenFile(m.stateLockFile(), os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("holding the state lock made no lock file: %v", err)
	}
	defer probe.Close()
	if err := flock(probe, false); !errors.Is(err, errLocked) {
		t.Fatalf("the state lock file could be taken while another held the lock: %v", err)
	}
	root := filepath.Join(m.Home, "example")
	done := make(chan error)
	go func() { done <- m.recordRun(root, noon, nil) }()
	unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if r, ok, _ := m.lastRun(root); !ok || !r.LastSuccess.Equal(noon) {
		t.Errorf("the waiting write didn't land: %+v", r)
	}
}

// Removing the idle runner holds the state lock from reading the list until
// the runner is gone, so a Configure in another process can't add a folder
// in between and be left with no runner.
func TestRemoveSchedulerIfIdleHoldsTheStateLock(t *testing.T) {
	m, cmds, _ := realMachine(t, "linux")
	held := map[string]bool{}
	cmds.before = func(line string) {
		probe, err := os.OpenFile(m.stateLockFile(), os.O_RDWR, 0)
		if err != nil {
			return
		}
		defer probe.Close()
		held[line] = errors.Is(flock(probe, false), errLocked)
	}
	removed, err := m.RemoveSchedulerIfIdle(context.Background())
	if err != nil || !removed {
		t.Fatalf("removed %v, err %v", removed, err)
	}
	if len(cmds.ran) == 0 {
		t.Fatal("ran nothing")
	}
	for _, line := range cmds.ran {
		if !held[line] {
			t.Errorf("%s ran without the state lock held", line)
		}
	}
}
