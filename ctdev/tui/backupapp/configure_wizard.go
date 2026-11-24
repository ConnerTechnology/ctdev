package backupapp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/backupscreen"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// wizard is Configure's state between its steps.
type wizard struct {
	d     Deps
	s     backup.Status
	where string

	cfg      backup.Config
	ownID    string // cfg.ID unless another folder's history is continued
	dest     *backup.Destination
	linkable map[string][]backup.FoundFolder // by destination, read once each
	link     *backup.FoundFolder             // the folder whose history continues
}

func newWizard(d Deps, s backup.Status) *wizard {
	w := &wizard{d: d, s: s, where: backup.DisplayPath(d.Machine.Home, s.Target), linkable: map[string][]backup.FoundFolder{}}
	if s.Config != nil {
		w.cfg = *s.Config
	} else {
		w.cfg = backup.NewConfig("")
	}
	w.ownID = w.cfg.ID
	return w
}

// errSkip is a step that doesn't apply, passed over in either direction.
var errSkip = errors.New("skip")

// run takes the steps in order: done moves on, ErrBack goes back one, and
// back from the first leaves the wizard.
func (w *wizard) run(ctx context.Context, ui UI) error {
	steps := []func(context.Context, UI) error{w.chooseDestination, w.history, w.schedule, w.keepAndSave}
	dir := 1
	for i := 0; i < len(steps); i += dir {
		switch err := steps[i](ctx, ui); {
		case errors.Is(err, errSkip):
		case errors.Is(err, ErrBack):
			if i == 0 {
				return ErrBack
			}
			dir = -1
		case err != nil:
			return err
		default:
			dir = 1
		}
	}
	return nil
}

// total is the wizard's step count: Backup history is a step only for a new
// folder whose destination holds folders to continue, or might.
func (w *wizard) total() int {
	if w.s.Config != nil {
		return 3
	}
	if w.dest != nil {
		if list, read := w.linkable[w.dest.Name]; read && len(list) == 0 {
			return 3
		}
	}
	return 4
}

func (w *wizard) page(n int, section string) Page {
	return Page{Title: w.where,
		Subtitle: styles.Dimmed.Render(fmt.Sprintf("Configure · step %d of %d · ", n, w.total())) + styles.Value.Render(section)}
}

// chooseDestination is W3 and W4: one of this computer's destinations, or a
// new one created or connected here.
func (w *wizard) chooseDestination(ctx context.Context, ui UI) error {
	m := w.d.Machine
	for {
		names, err := m.Destinations()
		if err != nil {
			return err
		}
		current := ""
		if w.s.Config != nil {
			current = w.s.Config.Destination
		}
		preferred := w.cfg.Destination
		intro := ""
		if current != "" && !slices.Contains(names, current) {
			intro = "  " + styles.Warning.Render("This folder backs up to "+current+", which isn't set up on this computer.") + "\n\n"
		}
		intro += "  " + styles.Value.Render("Back up to which destination?")

		items := make([]Item, 0, len(names)+2)
		cursor := -1
		for i, n := range names {
			backend := ""
			if dest, err := m.Destination(n); err == nil {
				backend = dest.Backend()
			}
			cells := []string{n, backend}
			if n == current {
				cells = append(cells, "(current)")
			}
			items = append(items, Item{Cells: cells})
			if n == preferred || (cursor < 0 && preferred == "") {
				cursor = i
			}
		}
		create, connect := len(names), len(names)+1
		dry := ""
		if m.Opts.DryRun {
			dry = "not in a dry run"
		}
		items = append(items,
			Item{Cells: []string{"Create a new destination"}, Dimmed: dry != "", Detail: dry},
			Item{Cells: []string{"Connect to an existing destination (Emergency Kit)"}, Dimmed: dry != "", Detail: dry})
		switch {
		case cursor >= 0:
		case preferred != "":
			cursor = connect // its destination isn't set up here, so its kit is what's needed
		default:
			cursor = create
		}

		i, err := ui.Pick(ctx, Picker{Page: w.page(1, "Where backups go"), Intro: intro, Items: items, Cursor: cursor, Radio: true,
			Enter: "Next", Back: "Cancel", Help: "↑/↓ choose · enter next"})
		if err != nil {
			return err
		}
		var dest *backup.Destination
		switch i {
		case create:
			dest, err = newDestination(ctx, ui, w.d, func(string) Page { return w.page(1, "New destination") }, preferred,
				func(address string) (string, error) { return m.CheckNewAddress(address, w.s.Target) })
		case connect:
			dest, err = connectWith(ctx, ui, w.d, func(string) Page { return w.page(1, "Connect with an Emergency Kit") })
		default:
			dest, err = m.Destination(names[i])
		}
		if errors.Is(err, ErrBack) {
			continue
		}
		if err != nil {
			return err
		}
		if w.dest == nil || w.dest.Name != dest.Name {
			w.link, w.cfg.ID = nil, w.ownID
		}
		w.dest, w.cfg.Destination = dest, dest.Name
		return nil
	}
}

// history is W11 and W12, for a new folder only: start a new history, or
// continue one of the destination's folders by taking its ID.
// backup.Machine.Linkable says which can be continued.
func (w *wizard) history(ctx context.Context, ui UI) error {
	if w.s.Config != nil {
		return errSkip
	}
	list, read := w.linkable[w.dest.Name]
	if !read {
		err := ui.Run(ctx, Running{Page: w.page(2, "Backup history"), Label: "Reading " + w.dest.Name + "…",
			Detail: "looking for folders whose backups this one could continue"},
			func(ctx context.Context, _ func(Progress)) error {
				o, err := w.dest.Overview(ctx)
				if err == nil {
					list, err = w.d.Machine.Linkable(o)
				}
				return err
			})
		if err != nil {
			return err
		}
		w.linkable[w.dest.Name] = list
	}
	if len(list) == 0 {
		return errSkip
	}
	for {
		page := w.page(2, "Backup history")
		connect := Item{Cells: []string{"Connect to an existing folder's backups"}}
		cursor := 0
		if w.link != nil {
			connect.Detail = fmt.Sprintf("%s continues the backups of %s on %s.", w.where, w.link.Path, w.link.Host)
			cursor = 1
		}
		i, err := ui.Pick(ctx, Picker{Page: page,
			Intro: "  " + styles.Value.Render("Start a new history, or continue the backups of an existing folder?"),
			Items: []Item{{Cells: []string{"Start a new history for " + w.where}}, connect}, Cursor: cursor, Radio: true,
			Enter: "Next", Help: "↑/↓ choose · enter next"})
		switch {
		case err != nil:
			return err
		case i == 0:
			w.link, w.cfg.ID = nil, w.ownID
			return nil
		case w.link != nil:
			return nil
		}
		page.Title = w.where + " › Backup history"
		j, err := ui.Pick(ctx, Picker{Page: page,
			Intro:   "  " + styles.Value.Render("Continue which folder's backups in "+w.where+"?"),
			Columns: []Column{{Title: "FOLDER"}, {Title: "LATEST"}, {Title: "SIZE", Right: true}, {Title: "BACKUPS", Right: true}},
			Items:   w.folderItems(list), Enter: "Continue", Help: "↑/↓ choose · enter continue · esc back"})
		if errors.Is(err, ErrBack) {
			continue // a new history, as before the list
		}
		if err != nil {
			return err
		}
		w.link = &list[j]
		w.cfg.ID = w.link.Ref.FolderID
	}
}

// folderItems are the folders to continue, grouped by computer, this one
// marked.
func (w *wizard) folderItems(list []backup.FoundFolder) []Item {
	items := make([]Item, len(list))
	for i, f := range list {
		group := f.Host
		if f.Host == w.d.Machine.Hostname {
			group += " (this computer)"
		}
		items[i] = Item{Group: group, Cells: []string{f.Path, Absolute(f.Latest), backupscreen.Bytes(f.Size), strconv.Itoa(f.Backups)}}
	}
	return items
}

// schedule is W13: how often.
func (w *wizard) schedule(ctx context.Context, ui UI) error {
	items := make([]Item, len(backup.Schedules))
	cursor := 0
	for i, s := range backup.Schedules {
		items[i] = Item{Cells: []string{capitalize(string(s))}}
		if s == w.cfg.Schedule {
			cursor = i
		}
	}
	note := ""
	if w.s.LoggedInOnly {
		note = "Scheduled backups run only while you're logged in."
	}
	i, err := ui.Pick(ctx, Picker{Page: w.page(w.total()-1, "How often"), Intro: "  " + styles.Value.Render("How often?"),
		Items: items, Cursor: cursor, Radio: true, Note: note, Enter: "Next", Help: "↑/↓ choose · enter next"})
	if err != nil {
		return err
	}
	w.cfg.Schedule = backup.Schedules[i]
	return nil
}

// keeps are the keep rules' fields, in order.
var keeps = []struct {
	key, label string
	n          func(*backup.Keep) *int
}{
	{"hourly", "HOURLY", func(k *backup.Keep) *int { return &k.Hourly }},
	{"daily", "DAILY", func(k *backup.Keep) *int { return &k.Daily }},
	{"weekly", "WEEKLY", func(k *backup.Keep) *int { return &k.Weekly }},
	{"monthly", "MONTHLY", func(k *backup.Keep) *int { return &k.Monthly }},
	{"yearly", "YEARLY", func(k *backup.Keep) *int { return &k.Yearly }},
}

// keepAndSave is W14 and W15: what to keep, then saving the folder's config
// and installing the scheduler, with the result said on the dashboard.
func (w *wizard) keepAndSave(ctx context.Context, ui UI) error {
	m := w.d.Machine
	saveErr := ""
	for {
		page := w.page(w.total(), "What to keep")
		fields := make([]Field, len(keeps))
		for i, k := range keeps {
			fields[i] = Field{Key: k.key, Label: k.label, Kind: Number, Required: true, Value: strconv.Itoa(*k.n(&w.cfg.Keep))}
		}
		v, err := ui.Form(ctx, Form{Page: page, Intro: "  " + styles.Value.Render("How many backups to keep:"),
			Fields: fields, Error: saveErr, Submit: "Save"})
		if err != nil {
			return err
		}
		for _, k := range keeps {
			*k.n(&w.cfg.Keep), _ = strconv.Atoi(v[k.key]) // the form takes only whole numbers
		}

		detail := "writing the folder's config"
		if w.s.Config == nil && w.s.Backups > 0 {
			detail = "adding the folder's earlier backups to its history"
		}
		page.Body = w.summary()
		var schedErr error
		err = ui.Run(ctx, Running{Page: page, Label: "Saving…", Detail: detail}, func(ctx context.Context, _ func(Progress)) error {
			if err := m.Configure(ctx, w.dest, w.s.Target, w.cfg); err != nil {
				return err
			}
			// Rewritten every time, so a ctdev that has moved is picked up.
			schedErr = w.d.Scheduler.Install(ctx)
			return nil
		})
		switch {
		case errors.Is(err, ErrBack):
			saveErr = ""
			continue
		case errors.Is(err, ErrCancelled):
			return err
		case err != nil:
			saveErr = capitalize(err.Error())
			continue
		}
		if m.Opts.DryRun {
			return schedErr
		}
		ui.Say(OK, fmt.Sprintf("✓ %s backs up %s to %s.", w.where, w.cfg.Schedule, w.dest.Name))
		if schedErr != nil {
			// The folder is configured; only the hourly runner is missing,
			// and the dashboard says "not scheduled" until it's fixed.
			ui.Say(Warning, "Couldn't schedule backups on this computer: "+schedErr.Error())
			ui.Say(Info, "Use Back up now, and choose Configure again to retry.")
		} else {
			ui.Say(Info, "Scheduled backups run hourly; NEXT shows when the next one is due.")
		}
		return nil
	}
}

// summary is what Saving saves.
func (w *wizard) summary() string {
	val := styles.Value.Render
	k := w.cfg.Keep
	rows := []string{Row("DESTINATION", val(w.dest.Name))}
	switch {
	case w.link != nil:
		rows = append(rows, Row("HISTORY", val("continues "+w.link.Path+" on "+w.link.Host)))
	case w.s.Config == nil:
		rows = append(rows, Row("HISTORY", val("a new history")))
	}
	rows = append(rows, Row("SCHEDULE", val(string(w.cfg.Schedule))),
		Row("KEEPING", val(fmt.Sprintf("%d hourly · %d daily · %d weekly · %d monthly · %d yearly", k.Hourly, k.Daily, k.Weekly, k.Monthly, k.Yearly))))
	return strings.Join(rows, "\n")
}
