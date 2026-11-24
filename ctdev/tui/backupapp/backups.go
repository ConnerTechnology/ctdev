package backupapp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/backupscreen"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// listing is a folder's backups across its destinations, newest first, each
// with the destination it's in, and the space they take there.
type listing struct {
	backups []listed
	stored  int64
}

type listed struct {
	snap backup.Snapshot
	dest *backup.Destination
}

// readListing lists ref's backups in each of dests.
func readListing(ctx context.Context, dests []*backup.Destination, ref backup.Ref) (listing, error) {
	var l listing
	for _, d := range dests {
		found, err := d.Snapshots(ctx, ref)
		if err != nil {
			return listing{}, err
		}
		for _, snap := range found {
			l.backups = append(l.backups, listed{snap, d})
		}
		if len(found) > 0 {
			n, err := d.Stored(ctx, found)
			if err != nil {
				return listing{}, err
			}
			l.stored += n
		}
	}
	slices.SortStableFunc(l.backups, func(a, b listed) int { return b.snap.Time.Compare(a.snap.Time) })
	return l, nil
}

// in is the listing's backups in d.
func (l listing) in(d *backup.Destination) []backup.Snapshot {
	var snaps []backup.Snapshot
	for _, b := range l.backups {
		if b.dest == d {
			snaps = append(snaps, b.snap)
		}
	}
	return snaps
}

// folderDestinations are where the folder's backups are: its destination
// once configured, and for one that isn't, every destination this computer
// has, since a one-off goes to whichever was chosen.
func folderDestinations(m *backup.Machine, s backup.Status) ([]*backup.Destination, error) {
	if s.Config != nil {
		if s.DestinationMissing {
			return nil, fmt.Errorf("destination %s isn't set up on this computer: choose Configure to connect to it", s.Config.Destination)
		}
		d, err := m.Destination(s.Config.Destination)
		if err != nil {
			return nil, err
		}
		return []*backup.Destination{d}, nil
	}
	names, err := m.Destinations()
	if err != nil {
		return nil, err
	}
	dests := make([]*backup.Destination, 0, len(names))
	for _, n := range names {
		d, err := m.Destination(n)
		if err != nil {
			return nil, err
		}
		dests = append(dests, d)
	}
	return dests, nil
}

// listBackups is List backups: the folder's backups, one backup's details,
// download and delete.
func listBackups(ctx context.Context, ui UI, d Deps, s backup.Status) error {
	m := d.Machine
	dests, err := folderDestinations(m, s)
	if err != nil {
		return err
	}
	download := func(ctx context.Context, dest *backup.Destination, snap backup.Snapshot) (string, error) {
		return m.Download(ctx, dest, s.Target, snap)
	}
	return browseBackups(ctx, ui, d, dests, m.Ref(s.Target, s.Config), backup.DisplayPath(m.Home, s.Target), download)
}

// browseBackups lists ref's backups in dests under where (~/...), then a
// backup's details with Download as zip and Delete this backup, until esc.
// It's List backups both in a folder and under --all, whose download
// differs.
func browseBackups(ctx context.Context, ui UI, d Deps, dests []*backup.Destination, ref backup.Ref, where string,
	download func(ctx context.Context, dest *backup.Destination, snap backup.Snapshot) (string, error)) error {
	title := where + " › Backups"
	oneOff := ref.FolderID == ""
	r := Running{Page: Page{Title: title, Subtitle: storedIn(dests, oneOff, "")},
		Label:       "Listing the backups of " + where + "…",
		Detail:      "Asks " + names(dests) + " for the folder's backups, then the space they take.",
		CancelLabel: "Back"}
	cursor := 0
	for {
		var l listing
		err := ui.Run(ctx, r, func(ctx context.Context, _ func(Progress)) error {
			var err error
			l, err = readListing(ctx, dests, ref)
			return err
		})
		if err != nil {
			return err
		}
		r = Running{Label: "Refreshing…", CancelLabel: "Back"}

		if len(l.backups) == 0 {
			_, err := ui.Dashboard(ctx, Dashboard{
				Page: Page{Title: title, Subtitle: storedIn(dests, oneOff, styles.Value.Render("0 backups")+styles.Dimmed.Render(" in ")),
					Body: "  " + styles.Value.Render(where+" has no backups.")},
				Back: "Back", Help: "esc back"})
			if err == nil {
				err = ErrBack
			}
			return err
		}
		for deleted := false; !deleted; {
			cursor = min(cursor, len(l.backups)-1)
			i, err := ui.Pick(ctx, backupsPicker(title, dests, oneOff, l, cursor))
			if err != nil {
				return err
			}
			cursor = i
			if deleted, err = backupDetail(ctx, ui, d, title, l.backups[i], i == 0, download); err != nil {
				return err
			}
		}
	}
}

// backupsPicker is the list of backups, with a column for the destination
// when there's more than one.
func backupsPicker(title string, dests []*backup.Destination, oneOff bool, l listing, cursor int) Picker {
	val, dim := styles.Value.Render, styles.Dimmed.Render
	lead := val(backups(len(l.backups), false)) + dim(" · ") + val(backupscreen.Bytes(l.stored)) + dim(" stored in ")
	p := Picker{
		Page:    Page{Title: title, Subtitle: storedIn(dests, oneOff, lead)},
		Columns: []Column{{Title: "WHEN"}, {Title: "SIZE", Right: true}, {Title: "NEW", Right: true}, {Title: "FILES", Right: true}},
		Cursor:  cursor,
	}
	many := len(dests) > 1
	if many {
		p.Columns = append(p.Columns, Column{Title: "DESTINATION"})
	}
	for i, b := range l.backups {
		s := b.snap
		cells := []string{Absolute(s.Time), backupscreen.Bytes(s.Size), backupscreen.Bytes(s.Added), count(s.Files)}
		if many {
			cells = append(cells, b.dest.Name)
		}
		if i == 0 {
			cells = append(cells, styles.Success.Render("latest"))
		}
		p.Items = append(p.Items, Item{Cells: cells})
	}
	return p
}

// storedIn is a list's subtitle: lead (styled), then the destinations, and
// the backend when there's one. One-offs say so first. With no lead it's
// the loading screen's.
func storedIn(dests []*backup.Destination, oneOff bool, lead string) string {
	val, dim := styles.Value.Render, styles.Dimmed.Render
	s := lead
	switch {
	case lead == "" && oneOff:
		s = dim("one-off backups in ")
	case lead == "":
		s = dim("backed up to ")
	case oneOff:
		s = dim("one-off backups · ") + lead
	}
	for i, d := range dests {
		if i > 0 {
			s += dim(", ")
		}
		s += val(d.Name)
	}
	if len(dests) == 1 {
		s += dim(" · " + dests[0].Backend())
	}
	return s
}

func names(dests []*backup.Destination) string {
	n := make([]string, len(dests))
	for i, d := range dests {
		n[i] = d.Name
	}
	return strings.Join(n, ", ")
}

// backupDetail is one backup's screen, with Download as zip and Delete this
// backup, until esc or a delete. deleted means the list has changed.
func backupDetail(ctx context.Context, ui UI, d Deps, title string, b listed, latest bool,
	download func(context.Context, *backup.Destination, backup.Snapshot) (string, error)) (deleted bool, err error) {
	m, snap, dest := d.Machine, b.snap, b.dest
	when := Absolute(snap.Time)
	val, dim := styles.Value.Render, styles.Dimmed.Render
	sub := dim("backup ") + val(snap.ShortID) + dim(" in ") + val(dest.Name) + dim(" · "+dest.Backend())
	if latest {
		sub += dim(" · ") + styles.Success.Render("latest")
	}
	page := Page{Title: title + " › " + when, Subtitle: sub, Body: detailRows(snap)}
	focus := "z"
	for {
		a, err := ui.Dashboard(ctx, Dashboard{Page: page, Focus: focus, Actions: []Action{
			{Key: "z", Label: "Download as zip"}, {Key: "D", Label: "Delete this backup"}}, Back: "Back"})
		switch {
		case errors.Is(err, ErrBack):
			return false, nil
		case err != nil:
			return false, err
		}
		focus = a.Key

		if a.Key == "z" {
			var path string
			err := ui.Run(ctx, Running{Label: "Downloading the backup from " + when + "…",
				Detail: "As a zip; restic reports no progress for a download."},
				func(ctx context.Context, _ func(Progress)) error {
					var err error
					path, err = download(ctx, dest, snap)
					return err
				})
			switch {
			case errors.Is(err, ErrCancelled):
				return false, err
			case errors.Is(err, ErrBack):
			case err != nil:
				ui.Say(Error, "Download failed: "+err.Error())
			case !m.Opts.DryRun:
				ui.Say(OK, "✓ Saved "+backup.DisplayPath(m.Home, path))
			}
			continue
		}

		ok, err := ui.Confirm(ctx, Confirm{Question: "Delete the backup from " + when + "?",
			Consequence: dim(fmt.Sprintf("%s · %s · %s · made on %s", snap.ShortID, backupscreen.Bytes(snap.Size), backupscreen.Files(snap.Files), snap.Host)),
			Yes:         "Delete", Destructive: true, Help: "y deletes · any other answer keeps it"})
		if err != nil {
			return false, err
		}
		if !ok {
			ui.Say(Info, "Nothing was deleted.")
			continue
		}
		var freed string
		err = ui.Run(ctx, Running{Label: "Deleting backup " + snap.ShortID + "…",
			Detail: "restic forget on " + dest.Name + "; waits up to 30 minutes if the destination is busy."},
			func(ctx context.Context, _ func(Progress)) error {
				if err := dest.Delete(ctx, snap); err != nil {
					return err
				}
				freed = spaceFreed(ctx, dest)
				return nil
			})
		switch {
		case errors.Is(err, ErrCancelled):
			return false, err
		case errors.Is(err, ErrBack):
			ui.Say(Warning, "Cancelled: the backup may already be deleted.")
			return true, nil
		case err != nil:
			sayError(ui, err)
			continue
		}
		if !m.Opts.DryRun {
			ui.Say(OK, "✓ Deleted backup "+snap.ShortID+".")
			ui.Say(Info, freed)
		}
		return true, nil
	}
}

// detailRows are a backup's status rows.
func detailRows(s backup.Snapshot) string {
	val := styles.Value.Render
	rows := []string{
		Row("SIZE", val(backupscreen.Bytes(s.Size))),
		Row("FILES", val(backupscreen.Files(s.Files))),
		Row("NEW DATA", val(backupscreen.Bytes(s.Added))),
		Row("CHANGES", val(fmt.Sprintf("%s changed · %s added", count(s.FilesChanged), count(s.FilesNew)))),
		Row("DURATION", val(duration(s.Duration))),
		Row("COMPUTER", val(s.Host)),
		Row("ID", val(s.ShortID)),
	}
	return strings.Join(rows, "\n")
}

func duration(d time.Duration) string {
	if d < time.Second {
		return "under a second"
	}
	return d.Round(time.Second).String()
}

// spaceFreed is said after every delete: forget doesn't free space, the
// weekly maintenance's prune does (CTD-87), so it says when that's due, as
// the destination's maintenance marker has it. Deletes from more than one
// destination, or a marker that can't be read, get no date.
func spaceFreed(ctx context.Context, dests ...*backup.Destination) string {
	if len(dests) != 1 {
		return spaceFreedOn(time.Time{}, time.Now())
	}
	next, err := dests[0].NextMaintenance(ctx)
	if err != nil {
		return spaceFreedOn(time.Time{}, time.Now())
	}
	return spaceFreedOn(next, time.Now())
}

// spaceFreedOn says when space is freed, with the next maintenance due at
// next; zero means unknown.
func spaceFreedOn(next, now time.Time) string {
	switch {
	case next.IsZero():
		return "Space is freed at the next weekly maintenance."
	case next.Sub(now) < time.Hour:
		return "Space is freed at the weekly maintenance, which is due within the hour."
	}
	return "Space is freed at the next weekly maintenance, due " + next.Local().Format("Jan 2") + "."
}

// backups counts n backups: "4 backups", "1 backup", or with all, "all 4
// backups" and "the only backup".
func backups(n int, all bool) string {
	switch {
	case n == 1 && all:
		return "the only backup"
	case n == 1:
		return "1 backup"
	case all:
		return fmt.Sprintf("all %d backups", n)
	}
	return fmt.Sprintf("%d backups", n)
}

// deleteAllBackups is Delete all: after y and typing the folder's
// name, it forgets every backup of the folder.
func deleteAllBackups(ctx context.Context, ui UI, d Deps, s backup.Status) error {
	m := d.Machine
	dests, err := folderDestinations(m, s)
	if err != nil {
		return err
	}
	where := backup.DisplayPath(m.Home, s.Target)
	// List now and delete exactly what was listed, so the count you confirm
	// is the count that goes, even if a backup ran since the status.
	var l listing
	err = ui.Run(ctx, Running{Label: "Listing the backups of " + where + "…",
		Detail: "Asks " + names(dests) + " for the folder's backups, then the space they take."},
		func(ctx context.Context, _ func(Progress)) error {
			var err error
			l, err = readListing(ctx, dests, m.Ref(s.Target, s.Config))
			return err
		})
	if err != nil {
		return err
	}
	total := len(l.backups)
	if total == 0 {
		ui.Say(Plain, where+" has no backups.")
		return ErrBack
	}
	question := "Delete " + backups(total, true) + " of " + where + "?"
	consequence := fmt.Sprintf("%s · %s stored in %s.", backups(total, false), backupscreen.Bytes(l.stored), names(dests))
	if s.Config != nil {
		consequence += " The folder stays backed up."
	}
	ok, err := ui.Confirm(ctx, Confirm{Question: question, Consequence: styles.Dimmed.Render(consequence),
		Yes: "Delete all", Destructive: true, Help: "y goes on · any other answer keeps them"})
	if err == nil && ok {
		ok, err = ui.Confirm(ctx, Confirm{Question: question, TypeToConfirm: filepath.Base(s.Target), Yes: "Delete all"})
	}
	if err != nil {
		return err
	}
	if !ok {
		ui.Say(Info, "Nothing was deleted.")
		return ErrBack
	}

	page := folderPage(s, m.Home, d.Cwd)
	page.Badge = Badge{Busy, "DELETING"}
	var freed string
	err = ui.Run(ctx, Running{Page: page, Label: "Deleting " + backups(total, false) + " of " + where + "…",
		Detail: "restic forget on " + names(dests) + "; waits up to 30 minutes if the destination is busy."},
		func(ctx context.Context, _ func(Progress)) error {
			var from []*backup.Destination
			for _, dest := range dests {
				snaps := l.in(dest)
				if len(snaps) == 0 {
					continue
				}
				if err := dest.DeleteBackups(ctx, snaps); err != nil {
					return err
				}
				from = append(from, dest)
			}
			freed = spaceFreed(ctx, from...)
			return nil
		})
	switch {
	case errors.Is(err, ErrBack):
		ui.Say(Warning, "Cancelled: some of the backups may already be deleted.")
		return ErrBack
	case err != nil:
		return err
	}
	if !m.Opts.DryRun {
		ui.Say(OK, fmt.Sprintf("✓ Deleted %s of %s.", backups(total, false), where))
		ui.Say(Info, freed)
	}
	return nil
}

// stopBackingUp is Stop backing up: after y, the folder's config file is
// deleted and it comes off this computer's schedule. It optionally deletes
// the folder's backups too (CTD-87), from every computer, by the folder ID
// while it's still known; otherwise they're kept. It refuses when it can't
// see the backups, so the chance to delete them is never silently skipped.
func stopBackingUp(ctx context.Context, ui UI, d Deps, s backup.Status) error {
	if s.Config == nil {
		return errors.New("this folder isn't configured, so there's nothing to stop")
	}
	m := d.Machine
	where := backup.DisplayPath(m.Home, s.Root)
	switch {
	case s.DestinationMissing:
		return fmt.Errorf("destination %s isn't set up on this computer, so its backups of %s can't be offered for deletion: choose Configure to connect to it first", s.Config.Destination, where)
	case s.ReadErr != nil:
		return fmt.Errorf("can't read the backups of %s, so Stop backing up can't offer to delete them: %w", where, s.ReadErr)
	}
	dest, err := m.Destination(s.Config.Destination)
	if err != nil {
		return err
	}
	unchanged := func(err error) error {
		if errors.Is(err, ErrBack) {
			ui.Say(Info, "Nothing was changed.")
		}
		return err
	}
	stops := backup.ConfigFileName + " and takes the folder off this computer's schedule."
	ok, err := ui.Confirm(ctx, Confirm{Question: "Stop backing up " + where + "?", Consequence: styles.Dimmed.Render("Removes " + stops),
		Yes: "Stop backing up", Destructive: true, Help: "y stops · any other answer keeps it backed up"})
	if err == nil && !ok {
		err = ErrBack
	}
	if err != nil {
		return unchanged(err)
	}

	// List now, by the folder's ID, and delete exactly what was counted.
	var listed []backup.Snapshot
	err = ui.Run(ctx, Running{Label: "Listing the backups of " + where + "…", Detail: "Asks " + dest.Name + " for the folder's backups."},
		func(ctx context.Context, _ func(Progress)) error {
			var err error
			listed, err = dest.Snapshots(ctx, m.Ref(s.Root, s.Config))
			return err
		})
	if err != nil {
		return unchanged(err)
	}
	deleteToo := false
	if len(listed) > 0 {
		question := "Also delete " + backups(len(listed), true) + " of this folder, from every computer?"
		deleteToo, err = ui.Confirm(ctx, Confirm{Question: question,
			Consequence: styles.Dimmed.Render("Kept backups stay under this folder's ID, and ctdev backup --all lists them."),
			Keep:        "Keep them", Yes: "Delete them too", Destructive: true,
			Help: "enter or n keeps them · y deletes them · esc cancels the stop"})
		if err == nil && deleteToo {
			var typed bool
			typed, err = ui.Confirm(ctx, Confirm{Question: question, TypeToConfirm: filepath.Base(s.Root), Yes: "Stop and delete"})
			if err == nil && !typed {
				err = ErrBack
			}
		}
		if err != nil {
			return unchanged(err)
		}
	}

	if deleteToo {
		page := folderPage(s, m.Home, d.Cwd)
		page.Badge = Badge{Busy, "DELETING"}
		var appeared []backup.Snapshot
		var freed string
		err := ui.Run(ctx, Running{Page: page, Label: "Deleting " + backups(len(listed), false) + " of " + where + "…", Detail: "Then removes " + stops},
			func(ctx context.Context, _ func(Progress)) error {
				var err error
				if appeared, err = m.StopAndDelete(ctx, dest, s.Root, listed); err != nil {
					return err
				}
				freed = spaceFreed(ctx, dest)
				return nil
			})
		switch {
		case errors.Is(err, ErrBack):
			ui.Say(Warning, "Cancelled: some of its backups may already be deleted.")
			return ErrBack
		case err != nil:
			return err
		}
		if !m.Opts.DryRun {
			ui.Say(OK, fmt.Sprintf("✓ Stopped backing up %s and deleted its backups.", where))
			ui.Say(Info, freed)
			// A backup that was running, here or on another computer,
			// finished after the list was confirmed.
			switch n := len(appeared); {
			case n == 1:
				ui.Say(Warning, "A backup was made while deleting, so it's kept under this folder's ID: ctdev backup --all lists it.")
			case n > 1:
				ui.Say(Warning, fmt.Sprintf("%d backups were made while deleting, so they're kept under this folder's ID: ctdev backup --all lists them.", n))
			}
		}
		unscheduleIfIdle(ctx, ui, d)
		return nil
	}

	if err := m.StopBackingUp(s.Root); err != nil {
		return err
	}
	if !m.Opts.DryRun {
		ui.Say(OK, "✓ Stopped backing up "+where+".")
		if n := len(listed); n > 0 {
			// Where the kept backups are found, and how the history resumes.
			kept, them := fmt.Sprintf("Its %d backups are kept under this folder's ID.", n), "them"
			if n == 1 {
				kept, them = "Its backup is kept under this folder's ID.", "it"
			}
			ui.Say(Plain, kept)
			ui.Say(Plain, "ctdev backup --all lists "+them+", and Configure → \"Connect to an existing folder's backups\" resumes the history.")
		}
	}
	unscheduleIfIdle(ctx, ui, d)
	return nil
}

// unscheduleIfIdle removes this computer's hourly runner once Stop backing
// up has taken the last folder off its list. The folder is already stopped,
// so a failure here only warns.
func unscheduleIfIdle(ctx context.Context, ui UI, d Deps) {
	if _, err := d.Scheduler.RemoveIfIdle(ctx); err != nil {
		ui.Say(Warning, "Couldn't remove the hourly backup runner: "+err.Error())
	}
}
