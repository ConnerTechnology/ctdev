// Package backupscreen is the `ctdev backup` screen: the header line saying
// which backed-up folder you're in, the folder's status, and an arrow-key
// menu whose unavailable items are dimmed and can't be chosen. Status renders
// the same header and lines on their own, for output without a terminal.
// NewAll and Folders are the `ctdev backup --all` screens.
package backupscreen

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// Action is a menu choice.
type Action int

const (
	None Action = iota
	BackUpNow
	ListBackups
	Configure
	StopBackingUp
	DeleteAll
	Quit
	Computers    // under --all
	EmergencyKit // under --all
)

// Item is one menu entry.
type Item struct {
	Action Action
	Label  string
	Dimmed bool // unavailable: shown, but the cursor skips it
}

// Model is the status and menu screen.
type Model struct {
	status string
	items  []Item
	cursor int
	chosen Action
}

// New builds the screen for a folder's status. home turns paths into ~/….
func New(s backup.Status, home string) *Model {
	m := &Model{status: Status(s, home), items: menu(s)}
	m.cursor = m.next(-1, 1)
	return m
}

// menu lists the items for the folder's state (CTD-87): a configured folder
// leads with Back up now, one that isn't with Configure. List and Delete all
// need a backup to act on, and an unconfigured folder has nothing to stop.
func menu(s backup.Status) []Item {
	if s.ConfigErr != nil {
		// Every action would act on a config ctdev can't read; fix it first.
		var items []Item
		for _, it := range menu(backup.Status{Folder: backup.Folder{Config: &backup.Config{}}}) {
			it.Dimmed = it.Action != Quit
			items = append(items, it)
		}
		return items
	}
	noBackups := !s.HasBackups()
	list := Item{Action: ListBackups, Label: "List backups", Dimmed: noBackups}
	deleteAll := Item{Action: DeleteAll, Label: "Delete all backups", Dimmed: noBackups}
	quit := Item{Action: Quit, Label: "Quit"}
	if s.Config != nil {
		return []Item{
			{Action: BackUpNow, Label: "Back up now"},
			list,
			{Action: Configure, Label: "Configure"},
			{Action: StopBackingUp, Label: "Stop backing up"},
			deleteAll,
			quit,
		}
	}
	return []Item{
		{Action: Configure, Label: "Configure"},
		{Action: BackUpNow, Label: "Back up now (once)"},
		list,
		{Action: StopBackingUp, Label: "Stop backing up", Dimmed: true},
		deleteAll,
		quit,
	}
}

// Items are the menu entries, in order.
func (m *Model) Items() []Item { return m.items }

// Chosen is the action picked, or None while the screen is still up.
func (m *Model) Chosen() Action { return m.chosen }

func (m *Model) Init() tea.Cmd { return nil }

// next finds the next selectable item from i in direction dir, staying put
// at either end.
func (m *Model) next(i, dir int) int {
	for j := i + dir; j >= 0 && j < len(m.items); j += dir {
		if !m.items[j].Dimmed {
			return j
		}
	}
	if i < 0 {
		return 0
	}
	return i
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "up", "k", "shift+tab":
		m.cursor = m.next(m.cursor, -1)
	case "down", "j", "tab":
		m.cursor = m.next(m.cursor, 1)
	case "q", "esc", "ctrl+c":
		m.chosen = Quit
		return m, tea.Quit
	case "enter", "space":
		it := m.items[m.cursor]
		if it.Dimmed {
			return m, nil
		}
		m.chosen = it.Action
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) View() tea.View {
	var b strings.Builder
	b.WriteString(m.status)
	b.WriteString("\n")
	if m.chosen != None {
		// Leave the status on screen, without the menu, for what runs next.
		return tea.NewView(b.String())
	}
	for i, it := range m.items {
		switch {
		case i == m.cursor:
			b.WriteString("  " + styles.Cursor.Render("> "+it.Label))
		case it.Dimmed:
			b.WriteString("    " + styles.Dimmed.Render(it.Label))
		default:
			b.WriteString("    " + it.Label)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n  " + styles.Help.Render("↑/↓ move · enter choose · q quit") + "\n")
	return tea.NewView(b.String())
}

// Header is the first line: which backed-up folder this is, and where you
// are in it.
func Header(s backup.Status, home string) string {
	var h string
	switch {
	case s.Root == "":
		return "Not backed up: " + backup.DisplayPath(home, s.Cwd)
	case s.Root == s.Cwd:
		h = "Backed up folder: " + backup.DisplayPath(home, s.Root)
	default:
		h = fmt.Sprintf("Backed up folder: %s  (you're in %s)", backup.DisplayPath(home, s.Root), backup.DisplayPath(home, s.Cwd))
	}
	if s.ConfigErr != nil {
		h += " · config can't be read: " + configReason(s.ConfigErr)
	}
	return h
}

// configReason drops the file's path from a config error: the header has
// already named the folder.
func configReason(err error) string {
	_, reason, ok := strings.Cut(err.Error(), backup.ConfigFileName+": ")
	if !ok {
		return err.Error()
	}
	return reason
}

// Status renders the header and the status lines.
func Status(s backup.Status, home string) string {
	label := styles.Label(14)
	var b strings.Builder
	b.WriteString(styles.Title.Render(Header(s, home)) + "\n")
	row := func(name, value string, bad bool) {
		if bad {
			value = styles.Error.Render(value)
		} else {
			value = styles.Value.Render(value)
		}
		fmt.Fprintf(&b, "  %s %s\n", label.Render(name), value)
	}
	warn := func(name, value string) {
		fmt.Fprintf(&b, "  %s %s\n", label.Render(name), styles.Warning.Render(value))
	}

	size := Bytes(s.Size) + " · " + Files(s.Files)
	if s.Unreadable > 0 {
		size += fmt.Sprintf(" · couldn't read %d", s.Unreadable)
	}
	if s.ConfigErr != nil {
		// Without the config, the rest would describe a folder that isn't
		// set up, which this one is.
		row("Size now", size, false)
		return b.String()
	}

	switch {
	case s.Destination == "":
		row("Destination", "none on this computer", false)
	case s.DestinationMissing:
		row("Destination", s.Destination+" (not set up on this computer)", true)
	default:
		row("Destination", fmt.Sprintf("%s (%s)", s.Destination, s.Backend), false)
	}
	row("Size now", size, false)

	switch {
	case s.Last != nil && s.Last.Failed():
		// This computer's failure stays in red even when another computer
		// has backed the folder up since.
		failed := when(s.Last.LastAttempt) + " · failed: " + s.Last.Error
		if !s.Latest.IsZero() {
			failed += " · newest backup " + when(s.Latest) + onHost(s)
		}
		row("Last backup", failed, true)
	case !s.Latest.IsZero():
		row("Last backup", when(s.Latest)+" · succeeded"+onHost(s), false)
	case s.Last != nil:
		// The destination couldn't be read; this computer's record is all there is.
		row("Last backup", when(s.Last.LastSuccess)+" · succeeded", false)
	default:
		row("Last backup", "never", false)
	}

	if s.Config != nil {
		sched, bad := string(s.Config.Schedule), true
		if !s.Scheduled || !s.SchedulerInstalled {
			// Not on this computer's list (a fresh clone), or the hourly
			// runner isn't installed: either way nothing will run it here.
			sched += " · not scheduled on this computer"
		}
		switch {
		case s.Overdue && (s.Last == nil || s.Last.LastSuccess.IsZero()):
			sched += " · overdue: never backed up"
		case s.Overdue:
			sched += " · overdue: no backup since " + when(s.Last.LastSuccess)
		case !s.Scheduled || !s.SchedulerInstalled:
		case !s.NextDue.After(s.At):
			sched, bad = sched+" · due now", false
		default:
			sched, bad = sched+" · next due "+when(s.NextDue), false
		}
		row("Schedule", sched, bad)
		if s.Scheduled && s.SchedulerInstalled && s.LoggedInOnly {
			// ctdev never enables lingering (CTD-87).
			fmt.Fprintf(&b, "  %s %s\n", label.Render(""), styles.Dimmed.Render("runs only while you're logged in"))
		}
		k := s.Config.Keep
		keeping := fmt.Sprintf("%d hourly, %d daily, %d weekly, %d monthly, %d yearly", k.Hourly, k.Daily, k.Weekly, k.Monthly, k.Yearly)
		if s.Last != nil && s.Last.RetentionError != "" {
			// The backup succeeded, so this is a warning, not a failure.
			warn("Keeping", keeping+" · retention failed: "+s.Last.RetentionError)
		} else {
			row("Keeping", keeping, false)
		}
	} else {
		row("Schedule", "none: this folder isn't configured", false)
		row("Keeping", "one-off backups, until you delete them", false)
	}

	if s.Backups == 0 {
		row("Backups", "none", false)
	} else {
		row("Backups", fmt.Sprintf("%d · oldest %s · %s stored", s.Backups, s.Oldest.Local().Format("Jan 2"), Bytes(s.Stored)), false)
	}
	if s.DataCheckRead {
		// The destination's weekly maintenance, whichever computer ran it.
		switch c := s.DataCheck; {
		case c == nil:
			row("Data check", "not run yet", false)
		case c.Failed():
			row("Data check", c.Finished.Local().Format("Jan 2")+" · "+c.Reason(), true)
		default:
			row("Data check", c.Finished.Local().Format("Jan 2")+" · OK", false)
		}
	}
	if s.ReadErr != nil {
		row("Problem", strings.ReplaceAll(s.ReadErr.Error(), "\n", "; "), true)
	}
	return b.String()
}

// onHost names the computer that made the newest backup when it isn't this one.
func onHost(s backup.Status) string {
	if s.LatestHost == "" || s.LatestHost == s.Hostname {
		return ""
	}
	return " on " + s.LatestHost
}

func when(t time.Time) string { return t.Local().Format("Jan 2, 3:04 PM") }

// Bytes renders a size in decimal units, the way file managers show it.
func Bytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGTPE"[exp])
}

// Files is a file count with a thousands separator: "1 file", "4,812 files".
func Files(n int) string {
	if n == 1 {
		return "1 file"
	}
	return thousands(n) + " files"
}

func thousands(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
