package backupapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
)

// fakeScheduler counts installs instead of touching this computer's timers.
type fakeScheduler struct {
	installs int
	err      error
}

func (f *fakeScheduler) Install(context.Context) error { f.installs++; return f.err }

func (f *fakeScheduler) RemoveIfIdle(context.Context) (bool, error) { return false, nil }

// Configure refuses a status with no folder to act on, before any screen.
func TestConfigureRefusesAnEmptyTarget(t *testing.T) {
	ui := &fakeUI{t: t}
	var s backup.Status // what a status that couldn't find the folder looks like
	if err := configureFolder(context.Background(), ui, Deps{Machine: &backup.Machine{}}, s); err == nil || err.Error() != "no folder to act on" {
		t.Errorf("want \"no folder to act on\", got %v", err)
	}
	if len(ui.Requests) != 0 {
		t.Errorf("showed %+v", ui.Requests)
	}
}

// A folder inside a backed-up one is never configured: the warning lands on
// the dashboard and the wizard doesn't open (W1). Backed-up folders inside
// this one are listed, and the wizard opens only once that's confirmed (W2).
func TestConfigureChecksTheFoldersAroundIt(t *testing.T) {
	home := t.TempDir()
	projects := filepath.Join(home, "Projects")
	website := filepath.Join(projects, "website")
	if err := os.MkdirAll(website, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := backup.WriteConfig(website, backup.NewConfig("example-backups")); err != nil {
		t.Fatal(err)
	}
	m := &backup.Machine{Home: home, Dir: filepath.Join(home, ".config", "ctdev", "backup")}
	d := Deps{Machine: m, Cwd: projects}

	ui := &fakeUI{t: t, Confirms: []any{false}}
	if err := configureFolder(context.Background(), ui, d, backup.Status{Target: projects}); !errors.Is(err, ErrBack) {
		t.Errorf("declined: %v, want ErrBack", err)
	}
	c := requestsOf[Confirm](ui)
	if len(c) != 1 || c[0].Question != "Configure ~/Projects anyway?" || !strings.Contains(c[0].Page.Body, "~/Projects/website") {
		t.Errorf("confirm = %+v", c)
	}

	if err := backup.WriteConfig(projects, backup.NewConfig("example-backups")); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(website, "notes")
	if err := os.MkdirAll(notes, 0o755); err != nil {
		t.Fatal(err)
	}
	ui = &fakeUI{t: t}
	if err := configureFolder(context.Background(), ui, d, backup.Status{Target: notes}); !errors.Is(err, ErrBack) {
		t.Errorf("inside a backed-up folder: %v, want ErrBack", err)
	}
	if got := ui.said(Warning); !slices.Equal(got, []string{"Already backed up as part of ~/Projects/website."}) || len(ui.Requests) != 0 {
		t.Errorf("said %q, showed %+v", got, ui.Requests)
	}
}

// Once Configure saves, the dashboard focuses Configure while the folder
// isn't scheduled, Back up now for a folder new to backups, else Quit (W16,
// W17).
func TestFocusAfterConfigure(t *testing.T) {
	scheduled := configured()
	unscheduled := configured()
	unscheduled.SchedulerInstalled = false
	for _, c := range []struct {
		before, after backup.Status
		want          string
	}{
		{unconfigured(), scheduled, "b"},
		{configured(), scheduled, "q"},
		{configured(), unscheduled, "c"},
		{unconfigured(), unscheduled, "c"},
		{unconfigured(), unconfigured(), "c"}, // a dry run saves nothing
	} {
		if got := focusAfterConfigure(c.before, c.after); got != c.want {
			t.Errorf("focus %q, want %q (before configured %v, after %+v)", got, c.want, c.before.Config != nil, c.after.SchedulerInstalled)
		}
	}
}
