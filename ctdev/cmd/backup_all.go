package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/backupscreen"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

// flagBackupAll is `ctdev backup --all`: the destination-wide view (CTD-87).
// It works from any directory and writes nothing into the current folder.
var flagBackupAll bool

func init() {
	backupCmd.Flags().BoolVar(&flagBackupAll, "all", false,
		"every backed-up folder in the destination, from every computer, to list and download")
}

// runBackupAll is the --all screen: pick this computer's destination (or set
// one up), then its header and menu until Quit.
func runBackupAll(ctx context.Context, m *backup.Machine, cwd string, out io.Writer) error {
	if isBatchMode() {
		return allStatusOnly(ctx, m, out)
	}
	d, err := destinationForAll(ctx, m)
	if errors.Is(err, errPromptCancelled) {
		return cancelToClean(err)
	}
	if err != nil {
		return err
	}
	for {
		o, err := d.Overview(ctx)
		if err != nil {
			return err
		}
		fmt.Println()
		screen := backupscreen.NewAll(d.Name, d.Backend(), o)
		if err := runScreen(screen); err != nil {
			return err
		}
		if screen.Chosen() != backupscreen.ListBackups {
			return nil
		}
		err = listAllFolders(ctx, m, d, cwd, o)
		if errors.Is(err, errPromptCancelled) {
			if ctx.Err() != nil {
				return nil
			}
			fmt.Println("\n  Cancelled.")
			err = nil
		}
		if err != nil {
			fmt.Println("\n  " + styles.Error.Render(err.Error()))
		}
	}
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
		if i > 0 {
			text += "\n"
		}
		text += backupscreen.AllHeader(d.Name, d.Backend(), o) + backupscreen.FolderList(o.Folders, m.Hostname)
	}
	if f, ok := out.(*os.File); !ok || !term.IsTerminal(f.Fd()) {
		text = ansi.Strip(text)
	}
	fmt.Fprint(out, text)
	return nil
}

// destinationForAll is the destination --all shows: this computer's only
// one, a choice among several, or with none, one connected or created now.
// Either way it's saved to ~/.config/ctdev only.
func destinationForAll(ctx context.Context, m *backup.Machine) (*backup.Destination, error) {
	names, err := m.Destinations()
	if err != nil {
		return nil, err
	}
	switch len(names) {
	case 0:
		fmt.Println("\n  This computer has no backup destination yet.")
		fmt.Println("    1) Connect to an existing destination (Emergency Kit)")
		fmt.Println("    2) Create a new destination")
		fmt.Printf("  %s ", styles.Dimmed.Render("Choice [1]:"))
		choice, err := promptChoiceCtx(ctx, 1)
		if err != nil {
			return nil, err
		}
		switch choice {
		case 1:
			return connectDestination(ctx, m)
		case 2:
			// No folder: --all sets up a destination, not the current folder.
			return createDestination(ctx, m, "", "")
		}
		return nil, fmt.Errorf("invalid choice %d", choice)
	case 1:
		return m.Destination(names[0])
	}
	fmt.Println("\n  Which destination?")
	for i, n := range names {
		fmt.Printf("    %d) %s\n", i+1, n)
	}
	fmt.Printf("  %s ", styles.Dimmed.Render("Choice [1]:"))
	choice, err := promptChoiceCtx(ctx, 1)
	if err != nil {
		return nil, err
	}
	if choice < 1 || choice > len(names) {
		return nil, fmt.Errorf("invalid choice %d", choice)
	}
	return m.Destination(names[choice-1])
}

// listAllFolders is List backups under --all: every folder, grouped by
// computer, then the chosen folder's backups, as List backups shows them in
// a folder. A download goes where DownloadFromAll says, relative to cwd.
func listAllFolders(ctx context.Context, m *backup.Machine, d *backup.Destination, cwd string, o backup.Overview) error {
	for {
		if len(o.Folders) == 0 {
			fmt.Println("\n  " + d.Name + " has no backups.")
			return nil
		}
		fmt.Println()
		list := backupscreen.NewFolders("Folders in "+d.Name, o.Folders, m.Hostname)
		if err := runScreen(list); err != nil {
			return err
		}
		f, ok := list.Chosen()
		if !ok {
			return nil
		}
		download := func(d *backup.Destination, snap backup.Snapshot) (string, error) {
			return m.DownloadFromAll(ctx, d, cwd, snap)
		}
		if err := browseBackups(ctx, m, []*backup.Destination{d}, f.Ref, f.Path, download); err != nil {
			return err
		}
		// A delete changes what's listed.
		var err error
		if o, err = d.Overview(ctx); err != nil {
			return err
		}
	}
}

// offerLink is Configure's "Connect to an existing folder's backups", for a
// folder new to the destination: picked from the destination's folders,
// grouped by computer, it gives cfg that folder's ID, so the history
// continues here. Only folders with an ID can be continued; one-offs have none.
func offerLink(ctx context.Context, m *backup.Machine, d *backup.Destination, where string, cfg *backup.Config) error {
	o, err := d.Overview(ctx)
	if err != nil {
		return err
	}
	var linkable []backup.FoundFolder
	for _, f := range o.Folders {
		if f.Ref.FolderID != "" {
			linkable = append(linkable, f)
		}
	}
	if len(linkable) == 0 {
		return nil
	}
	fmt.Println("\n  Backup history:")
	fmt.Println("    1) Start a new history for " + where)
	fmt.Println("    2) Connect to an existing folder's backups")
	fmt.Printf("  %s ", styles.Dimmed.Render("Choice [1]:"))
	choice, err := promptChoiceCtx(ctx, 1)
	if err != nil {
		return err
	}
	switch choice {
	case 1:
		return nil
	case 2:
	default:
		return fmt.Errorf("invalid choice %d", choice)
	}
	fmt.Println()
	list := backupscreen.NewFolders("Continue which folder's backups in "+where+"?", linkable, m.Hostname)
	if err := runScreen(list); err != nil {
		return err
	}
	f, ok := list.Chosen()
	if !ok {
		return errPromptCancelled
	}
	cfg.ID = f.Ref.FolderID
	fmt.Printf("  %s continues the backups of %s on %s.\n", where, f.Path, f.Host)
	return nil
}
