package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/backupapp"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/backupscreen"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

// flagBackupAll is `ctdev backup --all`: the destination-wide view (CTD-87).
// It works from any directory and writes nothing into the current folder.
var flagBackupAll bool

func init() {
	backupCmd.Flags().BoolVar(&flagBackupAll, "all", false,
		"the whole destination: every backed-up folder from every computer to list and download, "+
			"the computers with access, and the Emergency Kit and master password")
}

// runBackupAll is --all: the destination's full-screen dashboard in a
// terminal, each destination's folders without one.
func runBackupAll(ctx context.Context, m *backup.Machine, cwd string, out io.Writer) error {
	if isBatchMode() {
		return allStatusOnly(ctx, m, out)
	}
	return backupapp.RunAll(ctx, backupapp.Deps{Machine: m, Cwd: cwd, Scheduler: machineScheduler{m}})
}

// allStatusOnly prints, for a pipe or --batch, each destination's header and
// its folders, grouped by computer.
func allStatusOnly(ctx context.Context, m *backup.Machine, out io.Writer) error {
	names, err := m.Destinations()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return errors.New("this computer has no backup destination: run ctdev backup --all in a terminal to connect to one")
	}
	var text string
	for i, name := range names {
		d, err := m.Destination(name)
		if err != nil {
			return err
		}
		o, err := d.Overview(ctx)
		if err != nil {
			return fmt.Errorf("can't read %s: %w", name, err)
		}
		stored, err := storedAll(ctx, d, o)
		if err != nil {
			return fmt.Errorf("can't read %s: %w", name, err)
		}
		if i > 0 {
			text += "\n"
		}
		text += backupscreen.AllHeader(d.Name, d.Backend(), o, stored) + backupscreen.FolderList(o.Folders, m.Hostname)
	}
	if f, ok := out.(*os.File); !ok || !term.IsTerminal(f.Fd()) {
		text = ansi.Strip(text)
	}
	fmt.Fprint(out, text)
	return nil
}

// storedAll is the space the destination's backups take, or 0 with none.
func storedAll(ctx context.Context, d *backup.Destination, o backup.Overview) (int64, error) {
	if o.FolderCount == 0 {
		return 0, nil
	}
	return d.StoredAll(ctx)
}
