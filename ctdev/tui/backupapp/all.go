package backupapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/backupscreen"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// errToKitMenu ends a Computers flow that offers to finish under Emergency
// Kit and password: the --all loop opens that menu next.
var errToKitMenu = errors.New("finish under Emergency Kit and password")

// allFlow is `ctdev backup --all`: this computer's destination, chosen or set
// up, then its dashboard until Quit. It works from any directory and writes
// nothing into the current folder.
func allFlow(ctx context.Context, ui UI, d Deps) error {
	dest, err := destinationForAll(ctx, ui, d)
	if err != nil {
		return err
	}
	var o backup.Overview
	read, stale := false, true
	// The stored figure reads every backup, so it's taken once a session
	// rather than on every return to the dashboard.
	stored := int64(-1)
	focus := ""
	for {
		if stale {
			page := Page{}
			if !read {
				page = allPage(dest)
			}
			next, err := readOverview(ctx, ui, dest, page)
			switch {
			case err == nil:
				o, read = next, true
			case errors.Is(err, ErrCancelled), errors.Is(err, ErrBack) && !read:
				return err
			case errors.Is(err, ErrBack):
				// The refresh was cancelled: the last read stands.
			default:
				// Revoked credentials, after another computer's lost-computer
				// flow, are fixed by pasting the new kit.
				if dest, err = unreadable(ctx, ui, d, dest, err); err != nil {
					return err
				}
				read = false
				continue
			}
			stale = false
		}
		if stored < 0 {
			var err error
			if stored, err = readStored(ctx, ui, dest, o); err != nil {
				return err
			}
		}
		a, err := ui.Dashboard(ctx, allDashboard(dest, o, stored, focus))
		switch {
		case errors.Is(err, ErrBack):
			continue
		case err != nil:
			return err
		case a.Key == "q":
			return nil
		}
		focus, stale = a.Key, true
		for key := a.Key; key != ""; {
			var err error
			switch key {
			case "l":
				err = listAllFolders(ctx, ui, d, dest, o)
			case "c":
				err = manageComputers(ctx, ui, d, dest)
			case "e":
				dest, err = kitAndPassword(ctx, ui, d, dest)
			}
			key = ""
			switch {
			case errors.Is(err, errToKitMenu):
				key = "e"
			case errors.Is(err, ErrCancelled):
				return err
			case err != nil && !errors.Is(err, ErrBack):
				sayError(ui, err)
			}
		}
	}
}

// allPage is the destination's page: its name, and its backend under it.
func allPage(dest *backup.Destination) Page {
	return Page{Title: dest.Name, Subtitle: styles.Dimmed.Render("all backups on this destination · ") + styles.Value.Render(dest.Backend())}
}

// readOverview reads what the destination holds with a spinner over page:
// the loading screen first, then the screen being left.
func readOverview(ctx context.Context, ui UI, dest *backup.Destination, page Page) (backup.Overview, error) {
	r := Running{Label: "Refreshing…"}
	if page.Title != "" {
		r = Running{Page: page, Label: "Reading " + dest.Name + "…",
			Detail: "Listing its computers and every folder backed up to it.", CancelLabel: "Quit"}
	}
	var o backup.Overview
	err := ui.Run(ctx, r, func(ctx context.Context, _ func(Progress)) error {
		var err error
		o, err = dest.Overview(ctx)
		return err
	})
	return o, err
}

// readStored is the space the destination's backups take, or 0 with none.
func readStored(ctx context.Context, ui UI, dest *backup.Destination, o backup.Overview) (int64, error) {
	if o.FolderCount == 0 {
		return 0, nil
	}
	page := allPage(dest)
	page.Body = "  " + styles.Success.Render("✓") + " " +
		styles.Value.Render("Read "+plural(o.FolderCount, "folder")+" from "+plural(o.Computers, "computer"))
	var stored int64
	err := ui.Run(ctx, Running{Page: page, Label: "Adding up what " + dest.Name + " stores…",
		Detail: "reads every backup once; returning to this screen later doesn't repeat it", CancelLabel: "Quit"},
		func(ctx context.Context, _ func(Progress)) error {
			var err error
			stored, err = dest.StoredAll(ctx)
			return err
		})
	return stored, err
}

// allDashboard is the destination's home screen: what it holds, and
// List backups · Computers · Emergency Kit and password · Quit.
func allDashboard(dest *backup.Destination, o backup.Overview, stored int64, focus string) Dashboard {
	bold, dim := styles.Bold.Render, styles.Dimmed.Render
	hosts := map[string]bool{}
	for _, f := range o.Folders {
		hosts[f.Host] = true
	}
	from := "from " + plural(len(hosts), "computer")
	if o.FolderCount == 0 {
		from = "nothing backed up here yet"
	}
	page := allPage(dest)
	page.Body = strings.Join([]string{
		Row("COMPUTERS", lead(bold(fmt.Sprint(o.Computers)))+dim("with a key to "+dest.Name)),
		Row("FOLDERS", lead(bold(fmt.Sprint(o.FolderCount)))+dim(from)),
		Row("STORED", lead(bold(backupscreen.Bytes(stored)))+dim("every backup of every folder")),
	}, "\n")
	return Dashboard{Page: page, Focus: focus, Help: "←/→ choose · enter open · or press the letter", Actions: []Action{
		{Key: "l", Label: "List backups", Dimmed: len(o.Folders) == 0},
		{Key: "c", Label: "Computers"},
		{Key: "e", Label: "Emergency Kit and password"},
		{Key: "q", Label: "Quit"},
	}}
}

// destinationForAll is the destination --all shows: this computer's only
// one, a choice among several, or with none, one connected or created now.
// Either way it's saved to ~/.config/ctdev, and nothing goes into the
// current folder; a new local one is named in full (absolute or ~/), since
// --all runs from any directory.
func destinationForAll(ctx context.Context, ui UI, d Deps) (*backup.Destination, error) {
	m := d.Machine
	names, err := m.Destinations()
	if err != nil {
		return nil, err
	}
	dim, val := styles.Dimmed.Render, styles.Value.Render
	switch len(names) {
	case 0:
		return setUpForAll(ctx, ui, d)
	case 1:
		return m.Destination(names[0])
	}
	dests := make([]*backup.Destination, len(names))
	items := make([]Item, len(names))
	for i, n := range names {
		if dests[i], err = m.Destination(n); err != nil {
			return nil, err
		}
		items[i] = Item{Cells: []string{val(n), dim(dests[i].Backend())}}
	}
	i, err := ui.Pick(ctx, Picker{
		Page:  Page{Title: "Backup destination", Subtitle: dim("this computer backs up to ") + val(plural(len(names), "destination"))},
		Intro: "  " + styles.Bold.Render("Which destination?"), Items: items, Enter: "Open", Quit: true,
		Help: "↑/↓ choose · enter open"})
	if err != nil {
		return nil, err
	}
	return dests[i], nil
}

// setUpForAll is the choice a computer with no destination gets: connect to
// one with its kit, or create one. A failed attempt comes back to it.
func setUpForAll(ctx context.Context, ui UI, d Deps) (*backup.Destination, error) {
	dim, val := styles.Dimmed.Render, styles.Value.Render
	p := Picker{
		Page: Page{Title: "Backup destination", Badge: Badge{Attention, "NOT SET UP"},
			Subtitle: dim("nothing set up on ") + val(d.Machine.Hostname)},
		Intro: "  " + styles.Bold.Render("This computer has no backup destination yet."),
		Items: []Item{
			{Cells: []string{val("Connect to an existing destination"), dim("with its Emergency Kit and master password")}},
			{Cells: []string{val("Create a new destination"), dim("Backblaze B2, S3, SFTP or a local path")}},
		},
		Enter: "Continue", Quit: true, Help: "↑/↓ choose · enter continue",
	}
	for {
		i, err := ui.Pick(ctx, p)
		if err != nil {
			return nil, err
		}
		p.Cursor = i
		var dest *backup.Destination
		if i == 0 {
			dest, err = connectDestination(ctx, ui, d)
		} else {
			dest, err = createDestination(ctx, ui, d, "", d.Machine.CheckNewAddressFromAll)
		}
		switch {
		case err == nil:
			return dest, nil
		case errors.Is(err, ErrCancelled):
			return nil, err
		case !errors.Is(err, ErrBack):
			sayError(ui, err)
		}
	}
}

// unreadable is --all's offer when the destination can't be read, as after
// another computer's lost-computer flow revoked the credentials this one
// has: paste the new kit, or quit with the error.
func unreadable(ctx context.Context, ui UI, d Deps, dest *backup.Destination, readErr error) (*backup.Destination, error) {
	page := allPage(dest)
	switch {
	case errors.Is(readErr, backup.ErrWrongPassword):
		// A new kit can't help: this computer has to connect again.
		err := fmt.Errorf("this computer's key no longer opens %s: it may have been removed from another computer", dest.Name)
		page.Badge = Badge{Danger, "NO ACCESS"}
		_, derr := ui.Dashboard(ctx, Dashboard{Page: page, Message: []Line{{Error, capitalize(err.Error()) + "."}},
			Actions: []Action{{Key: "q", Label: "Quit"}}, Help: "q, esc or enter quits"})
		if errors.Is(derr, ErrCancelled) {
			return nil, derr
		}
		return nil, err
	case errors.Is(readErr, backup.ErrNoRestic), ctx.Err() != nil:
		return nil, readErr
	}
	page.Badge = Badge{Danger, "CAN'T READ"}
	offer := Dashboard{Page: page, Message: []Line{
		{Error, "Can't read " + dest.Name + ": " + readErr.Error()},
		{Plain, "If its credentials were replaced on another computer, paste the new Emergency Kit."},
	}, Actions: []Action{{Key: "p", Label: "Paste an updated Emergency Kit"}, {Key: "q", Label: "Quit"}},
		Help: "←/→ choose · enter open · or press the letter"}
	for {
		a, err := ui.Dashboard(ctx, offer)
		switch {
		case errors.Is(err, ErrBack):
			continue
		case err != nil:
			return nil, err
		case a.Key == "q":
			return nil, readErr
		}
		next, err := pasteUpdatedKit(ctx, ui, d)
		switch {
		case err == nil:
			return next, nil
		case errors.Is(err, ErrCancelled):
			return nil, err
		case !errors.Is(err, ErrBack):
			sayError(ui, err)
		}
	}
}

// listAllFolders is List backups under --all: every folder, grouped by
// computer, then the chosen folder's backups, as List backups shows them in
// a folder. A download goes where DownloadFromAll says, relative to the
// current directory.
func listAllFolders(ctx context.Context, ui UI, d Deps, dest *backup.Destination, o backup.Overview) error {
	download := func(ctx context.Context, dest *backup.Destination, snap backup.Snapshot) (string, error) {
		return d.Machine.DownloadFromAll(ctx, dest, d.Cwd, snap)
	}
	cursor := 0
	for {
		if len(o.Folders) == 0 {
			ui.Say(Info, dest.Name+" has no backups.")
			return nil
		}
		i, err := ui.Pick(ctx, foldersPicker(dest.Name, o.Folders, d.Machine.Hostname, cursor, time.Now()))
		if errors.Is(err, ErrBack) {
			return nil
		}
		if err != nil {
			return err
		}
		cursor = i
		f := o.Folders[i]
		if err := browseBackups(ctx, ui, d, []*backup.Destination{dest}, f.Ref, f.Path, download); err != nil && !errors.Is(err, ErrBack) {
			return err
		}
		// A delete changes what's listed.
		next, err := readOverview(ctx, ui, dest, Page{})
		switch {
		case err == nil:
			o = next
		case !errors.Is(err, ErrBack):
			return err
		}
	}
}

// foldersPicker lists the folders under a heading per computer, in the
// order Overview gives them (this computer first, and marked).
func foldersPicker(name string, folders []backup.FoundFolder, this string, cursor int, now time.Time) Picker {
	dim, val := styles.Dimmed.Render, styles.Value.Render
	items := make([]Item, len(folders))
	for i, f := range folders {
		group := f.Host
		if f.Host == this {
			group += " (this computer)"
		}
		items[i] = Item{Group: group, Cells: []string{val(f.Path), val(Relative(f.Latest, now)), dim(Absolute(f.Latest)),
			val(backupscreen.Bytes(f.Size)), dim(plural(f.Backups, "backup"))}}
	}
	return Picker{
		Page: Page{Title: crumb(name, "Folders"),
			Subtitle: dim("Folders in ") + val(name) + dim(" · by computer, this one first")},
		Columns: []Column{{Title: "FOLDER"}, {Title: "LAST BACKUP"}, {}, {Title: "SIZE", Right: true}, {Title: "BACKUPS"}},
		Items:   items, Cursor: cursor,
	}
}

// crumb is a breadcrumb title: "example-backups › Computers".
func crumb(parts ...string) string { return strings.Join(parts, " › ") }

// plural is "1 folder" or "3 folders".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return count(n) + " " + noun + "s"
}
