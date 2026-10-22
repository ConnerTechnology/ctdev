package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The status of a scheduled folder says whether it's overdue, whether this
// computer's runner is installed, and on Linux that it runs only while
// logged in.
func TestStatusOfAScheduledFolder(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		m := testMachine(t, "example-laptop")
		m.GOOS, m.Exe, m.lingerDir = goos, exampleExe, t.TempDir()
		at(m, noon.Add(-10*24*time.Hour))
		root := exampleFolder(t, m)
		listed(t, m, root, NewConfig("example-backups"))
		at(m, noon)
		if err := m.recordRun(root, noon.Add(-49*time.Hour), nil); err != nil {
			t.Fatal(err)
		}

		s := m.Status(context.Background(), root)
		if !s.Scheduled || !s.Overdue || s.SchedulerInstalled {
			t.Errorf("%s: scheduled %v, overdue %v, runner installed %v; want scheduled, overdue, no runner",
				goos, s.Scheduled, s.Overdue, s.SchedulerInstalled)
		}
		if s.LoggedInOnly != (goos == "linux") {
			t.Errorf("%s: LoggedInOnly = %v", goos, s.LoggedInOnly)
		}

		writeSchedulerFiles(t, m)
		if goos == "linux" {
			enableTimer(t, m)
		}
		if err := m.recordRun(root, noon.Add(-time.Hour), nil); err != nil {
			t.Fatal(err)
		}
		s = m.Status(context.Background(), root)
		if s.Overdue || !s.SchedulerInstalled {
			t.Errorf("%s: after a recent success with the runner in place: overdue %v, installed %v", goos, s.Overdue, s.SchedulerInstalled)
		}
		if want := noon.Add(23 * time.Hour); !s.NextDue.Equal(want) {
			t.Errorf("%s: next due %v, want %v", goos, s.NextDue, want)
		}
	}
}

// With lingering on, a user timer keeps running after logout, so the
// logged-in-only note would be wrong. ctdev only reads it.
func TestStatusWithLingerHasNoLoggedInNote(t *testing.T) {
	m := testMachine(t, "example-laptop")
	m.GOOS, m.lingerDir = "linux", t.TempDir()
	root := exampleFolder(t, m)
	listed(t, m, root, NewConfig("example-backups"))
	if !m.Status(context.Background(), root).LoggedInOnly {
		t.Fatal("no linger: want the logged-in-only note")
	}
	if err := os.WriteFile(filepath.Join(m.lingerDir, m.Username), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if m.Status(context.Background(), root).LoggedInOnly {
		t.Error("lingering is on: the logged-in-only note is wrong")
	}
}
