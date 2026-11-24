package backupapp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// removals is a Scheduler that counts RemoveIfIdle calls and fails them
// with err.
type removals struct {
	n   int
	err error
}

func (r *removals) Install(context.Context) error { return nil }
func (r *removals) RemoveIfIdle(context.Context) (bool, error) {
	r.n++
	return r.err == nil, r.err
}

// The space line says when the weekly maintenance frees the space, or
// that it's within the hour, or nothing about when (V9).
func TestSpaceFreedSaysWhen(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.Local)
	for _, c := range []struct {
		next time.Time
		want string
	}{
		{time.Time{}, "Space is freed at the next weekly maintenance."},
		{now, "Space is freed at the weekly maintenance, which is due within the hour."},
		{now.Add(5 * 24 * time.Hour), "Space is freed at the next weekly maintenance, due Oct 27."},
	} {
		if got := spaceFreedOn(c.next, now); got != c.want {
			t.Errorf("next %v: %q, want %q", c.next, got, c.want)
		}
	}
}

// Stop backing up refuses, before asking anything, when it can't see the
// folder's backups, so the chance to delete them is never skipped.
func TestStopBackingUpRefusesWhenTheBackupsCantBeRead(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m, err := backup.ThisMachine(sysutil.Opts{Stdout: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(m.Home, "Projects", "website")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := backup.NewConfig("example-backups")
	if err := backup.WriteConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]backup.Status{
		"unreadable": {Folder: backup.Folder{Cwd: dir, Root: dir, Config: &cfg}, Target: dir,
			Destination: "example-backups", ReadErr: errors.New("example failure")},
		"not set up here": {Folder: backup.Folder{Cwd: dir, Root: dir, Config: &cfg}, Target: dir,
			Destination: "example-backups", DestinationMissing: true},
	} {
		ui := &fakeUI{t: t}
		err := stopBackingUp(context.Background(), ui, Deps{Machine: m, Cwd: dir, Scheduler: &removals{}}, s)
		switch {
		case err == nil:
			t.Errorf("%s: want a refusal", name)
		case name == "unreadable" && !strings.Contains(err.Error(), "can't read the backups of ~/Projects/website"):
			t.Errorf("%s: error %q doesn't say why", name, err)
		case name == "not set up here" && !strings.Contains(err.Error(), "isn't set up on this computer"):
			t.Errorf("%s: error %q doesn't say why", name, err)
		}
		if len(ui.Requests) > 0 {
			t.Errorf("%s: asked %+v before refusing", name, ui.Requests)
		}
		if _, err := os.Stat(filepath.Join(dir, backup.ConfigFileName)); err != nil {
			t.Errorf("%s: the config file is gone: %v", name, err)
		}
	}
}
