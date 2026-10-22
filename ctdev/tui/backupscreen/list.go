package backupscreen

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// List is List backups: a header with the folder's backup count and the
// space they take, then one line per backup, newest first, and Back.
type List struct {
	header string
	snaps  []backup.Snapshot // newest first
	cursor int               // len(snaps) is Back
	chosen int               // index into snaps, or -1
	done   bool
}

// NewList builds the list for the folder shown as folder (~/…), from its
// backups in the order backup.Destination.Snapshots returns them, oldest
// first, and the space they take in the destination.
func NewList(folder string, snaps []backup.Snapshot, stored int64) *List {
	newest := slices.Clone(snaps)
	slices.SortStableFunc(newest, func(a, b backup.Snapshot) int { return b.Time.Compare(a.Time) })
	count := fmt.Sprintf("%d backups", len(snaps))
	if len(snaps) == 1 {
		count = "1 backup"
	}
	return &List{
		header: fmt.Sprintf("Backups of %s · %s · %s stored", folder, count, Bytes(stored)),
		snaps:  newest,
		chosen: -1,
	}
}

// Chosen is the backup picked, if one was.
func (l *List) Chosen() (backup.Snapshot, bool) {
	if l.chosen < 0 {
		return backup.Snapshot{}, false
	}
	return l.snaps[l.chosen], true
}

// Done reports whether the screen is finished: a backup was picked, or Back.
func (l *List) Done() bool { return l.done }

func (l *List) Init() tea.Cmd { return nil }

func (l *List) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return l, nil
	}
	switch k.String() {
	case "up", "k", "shift+tab":
		l.cursor = max(l.cursor-1, 0)
	case "down", "j", "tab":
		l.cursor = min(l.cursor+1, len(l.snaps))
	case "q", "esc", "ctrl+c":
		l.done = true
		return l, tea.Quit
	case "enter", "space":
		if l.cursor < len(l.snaps) {
			l.chosen = l.cursor
		}
		l.done = true
		return l, tea.Quit
	}
	return l, nil
}

func (l *List) View() tea.View {
	var b strings.Builder
	b.WriteString(styles.Title.Render(l.header) + "\n\n")
	if l.done {
		return tea.NewView(b.String())
	}
	for i, s := range l.snaps {
		line := fmt.Sprintf("%-16s  %8s  %12s  %12s", when(s.Time), Bytes(s.Size), Bytes(s.Added)+" new", Files(s.Files))
		if i == 0 {
			line += "  latest"
		}
		b.WriteString(menuLine(line, i == l.cursor))
	}
	b.WriteString(menuLine("Back", l.cursor == len(l.snaps)))
	b.WriteString("\n  " + styles.Help.Render("↑/↓ move · enter open · esc back") + "\n")
	return tea.NewView(b.String())
}

func menuLine(label string, selected bool) string {
	if selected {
		return "  " + styles.Cursor.Render("> "+label) + "\n"
	}
	return "    " + label + "\n"
}

// DetailAction is a choice on a backup's detail screen.
type DetailAction int

const (
	NoDetailAction DetailAction = iota
	Download
	DeleteOne
	Back
)

var detailItems = []struct {
	action DetailAction
	label  string
}{{Download, "Download as zip"}, {DeleteOne, "Delete this backup"}, {Back, "Back"}}

// Detail is one backup's details, then Download as zip · Delete this backup · Back.
type Detail struct {
	text   string
	cursor int
	chosen DetailAction
}

// NewDetail builds the detail screen for a backup of the folder shown as
// folder (~/…). latest marks the folder's newest backup.
func NewDetail(folder string, s backup.Snapshot, latest bool) *Detail {
	label := styles.Label(14)
	var b strings.Builder
	title := fmt.Sprintf("Backup of %s · %s", folder, when(s.Time))
	if latest {
		title += " · latest"
	}
	b.WriteString(styles.Title.Render(title) + "\n")
	row := func(name, value string) {
		fmt.Fprintf(&b, "  %s %s\n", label.Render(name), styles.Value.Render(value))
	}
	row("Size", Bytes(s.Size))
	row("Files", Files(s.Files))
	row("New data", Bytes(s.Added))
	row("Changes", fmt.Sprintf("%s changed · %s added", thousands(s.FilesChanged), thousands(s.FilesNew)))
	row("Duration", duration(s.Duration))
	row("Computer", s.Host)
	row("ID", s.ShortID)
	if s.Warning != "" {
		row("Note", s.Warning)
	}
	return &Detail{text: b.String()}
}

// Chosen is the action picked, or NoDetailAction while the screen is up.
func (d *Detail) Chosen() DetailAction { return d.chosen }

func (d *Detail) Init() tea.Cmd { return nil }

func (d *Detail) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	switch k.String() {
	case "up", "k", "shift+tab":
		d.cursor = max(d.cursor-1, 0)
	case "down", "j", "tab":
		d.cursor = min(d.cursor+1, len(detailItems)-1)
	case "q", "esc", "ctrl+c":
		d.chosen = Back
		return d, tea.Quit
	case "enter", "space":
		d.chosen = detailItems[d.cursor].action
		return d, tea.Quit
	}
	return d, nil
}

func (d *Detail) View() tea.View {
	var b strings.Builder
	b.WriteString(d.text + "\n")
	if d.chosen != NoDetailAction {
		return tea.NewView(b.String())
	}
	for i, it := range detailItems {
		b.WriteString(menuLine(it.label, i == d.cursor))
	}
	b.WriteString("\n  " + styles.Help.Render("↑/↓ move · enter choose · esc back") + "\n")
	return tea.NewView(b.String())
}

func duration(d time.Duration) string {
	if d < time.Second {
		return "under a second"
	}
	return d.Round(time.Second).String()
}

// Confirm asks a [y/N] question: only y confirms.
type Confirm struct {
	question string
	answer   string
	done     bool
	yes      bool
}

// NewConfirm asks question, which ends with its question mark.
func NewConfirm(question string) *Confirm { return &Confirm{question: question} }

// Done reports whether the question was answered.
func (c *Confirm) Done() bool { return c.done }

// Confirmed reports whether the answer was yes.
func (c *Confirm) Confirmed() bool { return c.yes }

func (c *Confirm) Init() tea.Cmd { return nil }

func (c *Confirm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}
	yes, answered := yesNo(k)
	if !answered {
		return c, nil
	}
	c.done, c.yes = true, yes
	c.answer = "n"
	if yes {
		c.answer = "y"
	}
	return c, tea.Quit
}

func (c *Confirm) View() tea.View {
	return tea.NewView("  " + c.question + " " + styles.Dimmed.Render("[y/N]") + " " + c.answer + "\n")
}

// yesNo reads a key as the answer to a [y/N] question: y is yes, and n,
// enter, esc, q or Ctrl-C is no. Any other key isn't an answer.
func yesNo(k tea.KeyPressMsg) (yes, answered bool) {
	switch strings.ToLower(k.Text) {
	case "y":
		return true, true
	case "n", "q":
		return false, true
	}
	switch k.String() {
	case "enter", "esc", "ctrl+c":
		return false, true
	}
	return false, false
}

// ConfirmDeleteAll asks [y/N] and then has you type the folder's name, since
// deleting every backup of a folder can't be undone.
type ConfirmDeleteAll struct {
	question string
	name     string
	typing   bool
	typed    string
	flash    string
	done     bool
	yes      bool
}

// NewDeleteAll confirms deleting all count backups of the folder shown as
// folder (~/…), whose base name name is what you type.
func NewDeleteAll(folder, name string, count int) *ConfirmDeleteAll {
	what := fmt.Sprintf("all %d backups", count)
	if count == 1 {
		what = "the only backup"
	}
	return &ConfirmDeleteAll{question: fmt.Sprintf("Delete %s of %s?", what, folder), name: name}
}

// NewTypeName only has you type the folder's name, for a delete whose
// [y/N] was already asked.
func NewTypeName(name string) *ConfirmDeleteAll {
	return &ConfirmDeleteAll{name: name, typing: true}
}

// Done reports whether the confirmation is finished, either way.
func (c *ConfirmDeleteAll) Done() bool { return c.done }

// Confirmed reports whether you said yes and typed the folder's name.
func (c *ConfirmDeleteAll) Confirmed() bool { return c.yes }

func (c *ConfirmDeleteAll) Init() tea.Cmd { return nil }

func (c *ConfirmDeleteAll) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}
	if !c.typing {
		yes, answered := yesNo(k)
		switch {
		case !answered:
			return c, nil
		case yes:
			c.typing = true
			return c, nil
		}
		c.done = true
		return c, tea.Quit
	}
	switch k.String() {
	case "esc", "ctrl+c":
		c.done = true
		return c, tea.Quit
	case "backspace":
		if r := []rune(c.typed); len(r) > 0 {
			c.typed = string(r[:len(r)-1])
		}
		return c, nil
	case "enter":
		if c.typed == c.name {
			c.done, c.yes = true, true
			return c, tea.Quit
		}
		c.flash = "That isn't " + c.name + "; type it exactly, or press esc to cancel."
		c.typed = ""
		return c, nil
	}
	if k.Text != "" && k.Mod&tea.ModCtrl == 0 {
		c.typed += k.Text
		c.flash = ""
	}
	return c, nil
}

func (c *ConfirmDeleteAll) View() tea.View {
	var b strings.Builder
	answer := ""
	if c.typing {
		answer = "y"
	}
	if c.question != "" {
		b.WriteString("  " + c.question + " " + styles.Dimmed.Render("[y/N]") + " " + answer + "\n")
	}
	if c.typing {
		b.WriteString("  Type " + styles.Value.Render(c.name) + " to confirm: " + c.typed + "\n")
		if c.flash != "" && !c.done {
			b.WriteString("  " + styles.Warning.Render(c.flash) + "\n")
		}
	}
	return tea.NewView(b.String())
}
