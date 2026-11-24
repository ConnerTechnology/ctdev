package backupapp

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/backupscreen"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
	"github.com/charmbracelet/x/ansi"
)

// folderActions are the dashboard's actions by key. Quit (q) ends the flow.
var folderActions = map[string]func(context.Context, UI, Deps, backup.Status) error{
	"b": backUpNow,
	"l": listBackups,
	"c": configureFolder,
	"s": stopBackingUp,
	"D": deleteAllBackups,
}

// folderFlow is `ctdev backup`: the folder's dashboard, until Quit. After
// each action the status is read again. Focus goes to Quit after an action
// that worked, and stays on one that didn't, to try again.
func folderFlow(ctx context.Context, ui UI, d Deps) error {
	s, err := readStatus(ctx, ui, d, loadingPage(d))
	if err != nil {
		return err
	}
	focus, retry := "", false
	for {
		a, err := ui.Dashboard(ctx, folderDashboard(s, d.Machine.Home, d.Cwd, focus, retry))
		switch {
		case errors.Is(err, ErrBack):
			continue
		case err != nil:
			return err
		case a.Key == "q":
			return nil
		}
		err = folderActions[a.Key](ctx, ui, d, s)
		focus, retry = a.Key, false
		switch {
		case errors.Is(err, ErrCancelled):
			return err
		case errors.Is(err, ErrBack):
		case err != nil:
			sayError(ui, err)
			retry = true
		default:
			focus = "q"
		}
		if next, err := readStatus(ctx, ui, d, Page{}); err == nil {
			s = next
		} else if errors.Is(err, ErrCancelled) {
			return err
		}
	}
}

// readStatus reads the folder's status with a spinner over page: the
// loading screen first, then the dashboard being refreshed.
func readStatus(ctx context.Context, ui UI, d Deps, page Page) (backup.Status, error) {
	r := Running{Page: page, Label: "Refreshing…"}
	if page.Title != "" {
		r = Running{Page: page, Label: "Reading " + backup.DisplayPath(d.Machine.Home, d.Cwd) + "…",
			Detail:      "Counting its files, then asking its destination for its backups and the space they take.",
			CancelLabel: "Quit"}
	}
	var s backup.Status
	err := ui.Run(ctx, r, func(ctx context.Context, _ func(Progress)) error {
		s = d.Machine.Status(ctx, d.Cwd)
		return ctx.Err()
	})
	return s, err
}

func loadingPage(d Deps) Page {
	return Page{Title: backup.DisplayPath(d.Machine.Home, d.Cwd), Subtitle: styles.Dimmed.Render("reading the folder and its backups")}
}

// sayError shows an action's error. A busy destination is a wait, not a
// failure, so it's yellow.
func sayError(ui UI, err error) {
	if errors.Is(err, backup.ErrDestinationBusy) {
		ui.Say(Warning, "The destination is busy (weekly maintenance or a delete holds its lock).")
		ui.Say(Warning, "Try again in a few minutes.")
		return
	}
	ui.Say(Error, err.Error())
}

// actionable refuses a status with no folder to act on, or whose config file
// can't be read: acting would back up the wrong place or replace the file.
func actionable(s backup.Status) error {
	switch {
	case s.Target == "":
		return errors.New("no folder to act on")
	case s.ConfigErr != nil:
		return fmt.Errorf("fix the folder's config file first: %w", s.ConfigErr)
	}
	return nil
}

// backUpNow backs up the whole folder after a confirmation, with restic's
// progress. A folder that isn't configured is backed up once, to a
// destination of this computer's, and gets no config file or schedule.
func backUpNow(ctx context.Context, ui UI, d Deps, s backup.Status) error {
	if err := actionable(s); err != nil {
		return err
	}
	m := d.Machine
	var dest *backup.Destination
	var err error
	heading, detail := "Back up now", ""
	if s.Config != nil {
		if s.DestinationMissing {
			return fmt.Errorf("destination %s isn't set up on this computer: choose Configure to connect to it", s.Config.Destination)
		}
		dest, err = m.Destination(s.Config.Destination)
	} else {
		heading, detail = "Back up once", ", once"
		dest, err = destinationForOnce(ctx, ui, d, s)
	}
	if err != nil {
		return err
	}

	where := backup.DisplayPath(m.Home, s.Target)
	consequence := styles.Value.Render(fmt.Sprintf("All of %s · %s · %s", where, backupscreen.Bytes(s.Size), backupscreen.Files(s.Files))) +
		"  " + styles.Dimmed.Render("→") + "  " + styles.Value.Render(dest.Name)
	if s.Config == nil {
		consequence += "\n" + styles.Dimmed.Render("A one-off: no schedule, and kept until you delete it.")
	}
	ok, err := ui.Confirm(ctx, Confirm{Question: heading, Consequence: consequence, Yes: "Back up"})
	if err != nil {
		return err
	}
	if !ok {
		return ErrBack
	}

	page := folderPage(s, m.Home, d.Cwd)
	page.Badge = Badge{Busy, "BACKING UP"}
	var snap backup.Snapshot
	err = ui.Run(ctx, Running{Page: page, Label: "Backing up", Detail: "to " + dest.Name + detail, CancelLabel: "Cancel — no backup is made"},
		func(ctx context.Context, progress func(Progress)) error {
			// Someone is waiting, so a locked destination gets a minute,
			// not the half hour a scheduled backup waits.
			var err error
			snap, err = m.BackupNow(ctx, dest, s.Target, s.Config, progress)
			return err
		})
	switch {
	case errors.Is(err, ErrBack):
		ui.Say(Info, "Cancelled — no backup was made.")
		return ErrBack
	case errors.Is(err, ErrCancelled), errors.Is(err, backup.ErrDestinationBusy):
		return err
	case err != nil:
		return fmt.Errorf("backup failed: %w", err)
	}
	if !m.Opts.DryRun {
		ui.Say(OK, fmt.Sprintf("✓ Backed up %s (%s), %s new, as %s.",
			backupscreen.Files(snap.Files), backupscreen.Bytes(snap.Size), backupscreen.Bytes(snap.Added), snap.ShortID))
		if snap.Warning != "" {
			ui.Say(Warning, "Note: "+snap.Warning+".")
		}
	}
	if s.Config == nil {
		return nil // one-offs are kept until deleted: no keep rules
	}
	// The folder's keep rules apply after each successful backup, to its own
	// backups only. Retain records a failure apart from the backup.
	err = ui.Run(ctx, Running{Label: "Applying the keep rules…",
		Detail: "restic forget on " + dest.Name + "; waits up to 30 minutes if the destination is busy."},
		func(ctx context.Context, _ func(Progress)) error { return m.Retain(ctx, dest, s.Target, *s.Config) })
	switch {
	case err == nil || m.Opts.DryRun:
	case errors.Is(err, ErrCancelled):
		return err
	case errors.Is(err, ErrBack), errors.Is(err, backup.ErrDestinationBusy):
		ui.Say(Info, "The keep rules apply at the next scheduled run.")
	default:
		ui.Say(Warning, "Applying the keep rules failed, so older backups weren't thinned: "+err.Error())
	}
	return nil
}

// destinationForOnce is this computer's destination for a one-off backup:
// the only one, a choice among several, or a new one set up now.
func destinationForOnce(ctx context.Context, ui UI, d Deps, s backup.Status) (*backup.Destination, error) {
	m := d.Machine
	names, err := m.Destinations()
	if err != nil {
		return nil, err
	}
	switch len(names) {
	case 0:
		ok, err := ui.Confirm(ctx, Confirm{Question: "Back up once",
			Consequence: styles.Value.Render("This computer has no backup destination yet. Set one up first."), Yes: "Set one up"})
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrBack
		}
		return createDestinationForOnce(ctx, ui, d, s)
	case 1:
		return m.Destination(names[0])
	}
	dests := make([]*backup.Destination, len(names))
	items := make([]Item, len(names))
	for i, n := range names {
		if dests[i], err = m.Destination(n); err != nil {
			return nil, err
		}
		items[i] = Item{Cells: []string{n, dests[i].Backend()}}
	}
	i, err := ui.Pick(ctx, Picker{Heading: "Back up once", Hint: "to which destination?", Items: items, Radio: true,
		Enter: "Next", Back: "Cancel", Help: "↑/↓ choose · enter next · esc cancel"})
	if err != nil {
		return nil, err
	}
	return dests[i], nil
}

// folderDashboard is the folder's home screen for its status. focus is the
// action to focus, "" for the state's own; retry relabels Back up now after
// it failed.
func folderDashboard(s backup.Status, home, cwd, focus string, retry bool) Dashboard {
	d := Dashboard{Page: folderPage(s, home, cwd), Message: folderMessage(s)}
	broken := s.ConfigErr != nil || s.Target == ""
	missing := s.Config != nil && s.DestinationMissing
	noBackups := !s.HasBackups() || missing
	backUp := Action{Key: "b", Label: "Back up now", Dimmed: missing || broken}
	if s.Failed() || retry {
		backUp.Label = "Try again"
	}
	list := Action{Key: "l", Label: "List backups", Dimmed: noBackups || broken}
	configure := Action{Key: "c", Label: "Configure", Dimmed: broken}
	stop := Action{Key: "s", Label: "Stop backing up", Dimmed: missing || broken || s.Config == nil}
	deleteAll := Action{Key: "D", Label: "Delete all", Dimmed: noBackups || broken}
	quit := Action{Key: "q", Label: "Quit"}
	if s.Config == nil && !broken {
		backUp.Label = "Back up now (once)"
		d.Actions = []Action{configure, backUp, list, stop, deleteAll, quit}
	} else {
		d.Actions = []Action{backUp, list, configure, stop, deleteAll, quit}
	}
	switch {
	case focus != "":
		d.Focus = focus
	case broken:
		d.Focus = "q"
	case s.Failed():
		d.Focus = "b"
	case s.Config == nil || missing || !s.Scheduled || !s.SchedulerInstalled:
		d.Focus = "c"
	}
	if broken {
		d.Help = "q quit"
	}
	return d
}

// folderPage is the title, badge, subtitle and status rows.
func folderPage(s backup.Status, home, cwd string) Page {
	p := Page{Title: backup.DisplayPath(home, s.Target), Badge: folderBadge(s), Body: folderRows(s)}
	if s.Target == "" {
		p.Title = backup.DisplayPath(home, cwd)
	}
	if s.Root != "" && s.Root != s.Cwd {
		p.Context = "you're in " + backup.DisplayPath(home, s.Cwd)
	}
	dim, val := styles.Dimmed.Render, styles.Value.Render
	backend := ""
	if s.Backend != "" {
		backend = dim(" · " + s.Backend)
	}
	switch {
	case s.ConfigErr != nil:
		p.Subtitle = dim("backed-up folder · its config file can't be read")
	case s.Target == "":
		p.Subtitle = dim("the folder can't be read")
	case s.Config != nil && s.DestinationMissing:
		p.Subtitle = dim("backed up to ") + styles.Error.Render(s.Destination) + dim(" · not set up on this computer")
	case s.Config != nil:
		p.Subtitle = dim("backed up to ") + val(s.Destination) + backend
	case s.Destination == "":
		p.Subtitle = dim("not backed up · no destination on this computer yet")
	case strings.Contains(s.Destination, ", "):
		p.Subtitle = dim("not backed up · this computer's destinations are ") + val(s.Destination) + backend
	default:
		p.Subtitle = dim("not backed up · this computer's destination is ") + val(s.Destination) + backend
	}
	return p
}

// folderBadge names the worst of the folder's problems: red beats yellow.
func folderBadge(s backup.Status) Badge {
	switch {
	case s.ConfigErr != nil:
		return Badge{Danger, "CONFIG CAN'T BE READ"}
	case s.Target == "":
		return Badge{Danger, "CAN'T READ THE FOLDER"}
	case s.Config != nil && s.DestinationMissing:
		return Badge{Danger, "DESTINATION NOT SET UP"}
	case s.Failed():
		return Badge{Danger, "LAST BACKUP FAILED"}
	case s.ReadErr != nil:
		return Badge{Danger, "CAN'T READ BACKUPS"}
	case s.Config == nil:
		return Badge{Attention, "NOT BACKED UP"}
	case !s.Scheduled || !s.SchedulerInstalled:
		return Badge{Attention, "NOT SCHEDULED"}
	case s.Overdue:
		return Badge{Attention, "OVERDUE"}
	case s.Last != nil && s.Last.RetentionError != "":
		return Badge{Attention, "KEEP RULES FAILED"}
	case s.DataCheck != nil && s.DataCheck.Failed():
		return Badge{Attention, "DATA CHECK FAILED"}
	case s.DataCheckOverdue:
		return Badge{Attention, "DATA CHECK OVERDUE"}
	}
	return Badge{Healthy, "HEALTHY"}
}

// folderMessage is the state's lines under the rule.
func folderMessage(s backup.Status) []Line {
	switch {
	case s.ConfigErr != nil:
		return []Line{{Error, "Fix the folder's config file first."}, {Info, s.ConfigErr.Error()}}
	case s.Target == "":
		return nil
	case s.Config != nil && s.DestinationMissing:
		return []Line{{Error, s.Destination + " isn't set up on this computer."}, {Info, "Choose Configure to connect to it."}}
	case s.Failed():
		lines := []Line{{Error, s.Last.Error}}
		if !s.Latest.IsZero() {
			lines = append(lines, Line{Info, "Newest backup still on the destination: " + Absolute(s.Latest) + onHost(s)})
		}
		return lines
	case s.Config == nil:
		return []Line{{Info, "Configure to back it up on a schedule, or back it up once."}}
	case !s.Scheduled || !s.SchedulerInstalled:
		return []Line{{Warning, "Nothing will back this folder up on this computer until it's scheduled."},
			{Info, "Configure adds it to this computer's hourly runner."}}
	case s.Overdue:
		return []Line{{Warning, fmt.Sprintf("Overdue: no success on this computer in twice its %s interval.", s.Config.Schedule)}}
	}
	return nil
}

// folderRows are the status rows: what the folder's backups are now.
func folderRows(s backup.Status) string {
	dim, val, bold, warn, bad := styles.Dimmed.Render, styles.Value.Render, styles.Bold.Render, styles.Warning.Render, styles.Error.Render
	var rows []string
	row := func(label, value string) { rows = append(rows, Row(label, value)) }
	more := func(value string) { rows = append(rows, strings.Repeat(" ", 17)+value) }

	size := val(backupscreen.Bytes(s.Size)) + "  " + dim(backupscreen.Files(s.Files))
	if s.Unreadable > 0 {
		size += dim(" · ") + warn(fmt.Sprintf("couldn't read %d", s.Unreadable))
	}
	switch {
	case s.ConfigErr != nil:
		// The rest would describe a folder that isn't set up, which this is.
		row("SIZE NOW", size)
		return strings.Join(rows, "\n")
	case s.Target == "":
		row("PROBLEM", bad(oneLine(s.ReadErr)))
		return strings.Join(rows, "\n")
	}

	now := s.At
	switch {
	case s.Failed():
		row("LAST BACKUP", lead(bad("failed "+Relative(s.Last.LastAttempt, now)))+dim(Absolute(s.Last.LastAttempt)))
	case !s.Latest.IsZero():
		row("LAST BACKUP", lead(bold(Relative(s.Latest, now)))+dim(Absolute(s.Latest)+" · succeeded"+onHost(s)))
	case s.Last != nil && !s.Last.LastSuccess.IsZero():
		// The destination couldn't be read; this computer's record is all there is.
		row("LAST BACKUP", lead(bold(Relative(s.Last.LastSuccess, now)))+dim(Absolute(s.Last.LastSuccess)+" · succeeded"))
	default:
		row("LAST BACKUP", bold("never"))
	}
	if s.Last != nil && s.Last.Deferred != "" {
		// A locked destination isn't a failure: the next hour tries again.
		more(dim("waiting: destination busy"))
	}

	if s.Config == nil {
		row("NEXT", val("none: this folder isn't configured"))
	} else {
		sched := string(s.Config.Schedule)
		if s.LoggedInOnly {
			sched += " · runs only while you're logged in" // ctdev never enables lingering
		}
		switch {
		case !s.Scheduled || !s.SchedulerInstalled:
			row("NEXT", warn("not scheduled on this computer"))
		case s.Overdue && (s.Last == nil || s.Last.LastSuccess.IsZero()):
			row("NEXT", warn("overdue: never backed up"))
		case s.Overdue:
			row("NEXT", warn("overdue: no backup since "+Absolute(s.Last.LastSuccess)))
		case !s.NextDue.After(now):
			row("NEXT", lead(bold("due now"))+dim(sched))
		default:
			row("NEXT", lead(bold(Relative(s.NextDue, now)))+dim(sched))
		}
	}
	rows = append(rows, "")
	row("SIZE NOW", size)

	switch {
	case s.Config != nil && s.DestinationMissing:
		row("STORED", dim("unknown: "+s.Destination+" can't be read from this computer"))
	case s.Backups == 0 && s.ReadErr != nil:
		row("STORED", dim("unknown: the destination couldn't be read"))
	case s.Backups == 0:
		row("STORED", val("none"))
	default:
		n := fmt.Sprintf("%d backups", s.Backups)
		if s.Backups == 1 {
			n = "1 backup"
		}
		row("STORED", val(backupscreen.Bytes(s.Stored))+"  "+dim(n+" · oldest "+s.Oldest.Local().Format("Jan 2")))
	}

	if s.Config == nil {
		row("KEEPING", val("one-off backups, until you delete them"))
	} else {
		k := s.Config.Keep
		keeping := fmt.Sprintf("%d hourly · %d daily · %d weekly · %d monthly · %d yearly", k.Hourly, k.Daily, k.Weekly, k.Monthly, k.Yearly)
		if s.Last != nil && s.Last.RetentionError != "" {
			// The backup succeeded, so this is a warning, not a failure.
			row("KEEPING", warn(keeping))
			more(warn("retention failed: " + s.Last.RetentionError))
		} else {
			row("KEEPING", val(keeping))
		}
	}

	if s.DataCheckRead {
		// The destination's weekly maintenance, whichever computer ran it.
		switch c := s.DataCheck; {
		case c != nil && c.Failed():
			row("DATA CHECK", bad(c.Finished.Local().Format("Jan 2")+" · "+c.Reason()))
		case s.DataCheckOverdue && c == nil:
			row("DATA CHECK", warn("overdue (never run)"))
		case s.DataCheckOverdue:
			row("DATA CHECK", warn("overdue (last "+c.Finished.Local().Format("Jan 2")+")"))
		case c == nil:
			row("DATA CHECK", val("not run yet"))
		default:
			row("DATA CHECK", styles.Success.Render("✓ ")+val(c.Finished.Local().Format("Jan 2")))
		}
	}
	if s.ReadErr != nil {
		row("PROBLEM", bad(oneLine(s.ReadErr)))
	}
	return strings.Join(rows, "\n")
}

// Row is a status row: a dim label column 12 wide, then the value, styled
// by the caller.
func Row(label, value string) string {
	return "  " + styles.Dimmed.Render(fmt.Sprintf("%-12s", label)) + "   " + value
}

// lead pads a row's leading value so the detail beside it lines up.
func lead(v string) string {
	return v + strings.Repeat(" ", max(18-ansi.StringWidth(v), 2))
}

// Relative is a time from now in words: "just now", "14 hours ago",
// "in 10 hours", "3 days ago".
func Relative(t, now time.Time) string {
	d := now.Sub(t)
	future := d < 0
	if future {
		d = -d
	}
	var n int
	var unit string
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		n, unit = int(math.Round(d.Minutes())), "minute"
	case d < 48*time.Hour:
		n, unit = int(math.Round(d.Hours())), "hour"
	default:
		n, unit = int(math.Round(d.Hours()/24)), "day"
	}
	if n != 1 {
		unit += "s"
	}
	if future {
		return fmt.Sprintf("in %d %s", n, unit)
	}
	return fmt.Sprintf("%d %s ago", n, unit)
}

// Absolute is a time as the dim column beside a relative one shows it.
func Absolute(t time.Time) string { return t.Local().Format("Jan 2, 3:04 PM") }

// onHost names the computer that made the newest backup when it isn't this one.
func onHost(s backup.Status) string {
	if s.LatestHost == "" || s.LatestHost == s.Hostname {
		return ""
	}
	return " on " + s.LatestHost
}

func oneLine(err error) string { return strings.ReplaceAll(err.Error(), "\n", "; ") }
