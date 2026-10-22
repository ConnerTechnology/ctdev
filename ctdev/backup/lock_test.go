package backup

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// Configure and Back up now never wait on a scheduled backup: while another
// *Machine (as another process would) holds the run lock for a whole run,
// writes to folders.json and state.json go through at once.
func TestStateWritesDontWaitOnARunningScheduledBackup(t *testing.T) {
	m := testMachine(t, "example-laptop")
	runner := &Machine{Dir: m.Dir}
	unlock, err := runner.runLock()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if !m.runInProgress() {
		t.Fatal("runInProgress doesn't see the other run")
	}
	root := filepath.Join(m.Home, "example")
	if err := m.addFolder(root, newUUID(), Daily); err != nil {
		t.Fatal(err)
	}
	if err := m.recordRun(root, noon, nil); err != nil {
		t.Fatal(err)
	}
	if r, ok, _ := m.lastRun(root); !ok || !r.LastSuccess.Equal(noon) {
		t.Errorf("state write while a run holds the run lock: %+v", r)
	}
	// And a second run can't start.
	if _, err := m.runLock(); !errors.Is(err, errLocked) {
		t.Errorf("a second run lock: %v, want errLocked", err)
	}
}

// Configure drops a same-ID entry whose path is gone only once a scheduled
// run has recorded it missing, and says so. One that was fine at its last
// run, such as a folder on a drive that isn't mounted, stays.
func TestAddFolderDropsOnlyAnEntryRecordedMissing(t *testing.T) {
	m := testMachine(t, "example-laptop")
	out := &bytes.Buffer{}
	m.Opts.Stdout = out
	at(m, noon)
	id := newUUID()
	usb := "/media/example/usb/website" // not mounted
	if err := m.addFolder(usb, id, Daily); err != nil {
		t.Fatal(err)
	}
	if err := m.recordRun(usb, noon.Add(-time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(m.Home, "Sites", "website")
	if err := m.addFolder(moved, id, Daily); err != nil {
		t.Fatal(err)
	}
	if h := health(t, m); len(h) != 2 {
		t.Fatalf("health = %+v, want the unmounted entry kept beside the new one", h)
	}
	if out.Len() != 0 {
		t.Errorf("said %q, want nothing", out)
	}

	if err := m.recordMissing(usb, noon, "the folder isn't there any more"); err != nil {
		t.Fatal(err)
	}
	if err := m.addFolder(moved, id, Daily); err != nil {
		t.Fatal(err)
	}
	h := health(t, m)
	if len(h) != 1 || h[0].Path != moved {
		t.Errorf("health = %+v, want only the new place", h)
	}
	want := "Removed /media/example/usb/website from this computer's list: it moved to ~/Sites/website.\n"
	if out.String() != want {
		t.Errorf("said %q, want %q", out, want)
	}
	if runs, _ := m.runs(); len(runs) != 0 {
		t.Errorf("state.json keeps the old place: %v", runs)
	}
}

// A dry run over a folder already failing neither notifies nor marks the
// notification sent.
func TestDryRunOverAFailingFolderDoesntNotify(t *testing.T) {
	m, root, n := failingMachine(t)
	if err := m.recordRun(root, noon.Add(-25*time.Hour), errors.New("example failure")); err != nil {
		t.Fatal(err)
	}
	m.Opts.DryRun = true
	_, _ = m.RunScheduled(context.Background())
	if len(n.sent) != 0 {
		t.Errorf("a dry run notified: %v", n.sent)
	}
	if r, _, _ := m.lastRun(root); r.Alerted || !r.LastAttempt.Equal(noon.Add(-25*time.Hour)) {
		t.Errorf("a dry run changed the state: %+v", r)
	}
}

// An entry written before schedules were recorded is judged as daily.
func TestAnEmptyScheduleIsDaily(t *testing.T) {
	m := testMachine(t, "example-laptop")
	root := filepath.Join(m.Home, "example")
	if err := writeJSON(m.foldersFile(), foldersFile{Folders: []folderEntry{{Path: root, ID: newUUID(), Added: noon.Add(-72 * time.Hour)}}}); err != nil {
		t.Fatal(err)
	}
	if err := m.recordRun(root, noon.Add(-47*time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	at(m, noon)
	h := health(t, m)
	if len(h) != 1 || h[0].Schedule != Daily || h[0].State != HealthOK {
		t.Fatalf("health = %+v, want daily and OK at 47h", h)
	}
	at(m, noon.Add(2*time.Hour))
	if h := health(t, m); h[0].State != HealthOverdue {
		t.Errorf("at 49h: %q, want overdue", h[0].State)
	}
}
