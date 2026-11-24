package backupapp

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
	"github.com/charmbracelet/x/ansi"
)

// dashboard is a Dashboard's step: its message and the one-key action bar.
// The cursor runs over the actions, then the esc item when there is one.
type dashboard struct {
	d      Dashboard
	cursor int // -1 when nothing can be focused
}

func newDashboard(d Dashboard) *dashboard {
	b := &dashboard{d: d, cursor: -1}
	for i, a := range d.Actions {
		if a.Dimmed {
			continue
		}
		if b.cursor < 0 || a.Key == d.Focus {
			b.cursor = i
		}
		if a.Key == d.Focus {
			break
		}
	}
	if b.cursor < 0 && d.Back != "" {
		b.cursor = len(d.Actions)
	}
	return b
}

func (b *dashboard) init() tea.Cmd { return nil }

// move steps the cursor to the next action in dir that isn't dimmed,
// staying put at either end.
func (b *dashboard) move(dir int) {
	last := len(b.d.Actions)
	if b.d.Back != "" {
		last++
	}
	for i := b.cursor + dir; i >= 0 && i < last; i += dir {
		if i == len(b.d.Actions) || !b.d.Actions[i].Dimmed {
			b.cursor = i
			return
		}
	}
}

func (b *dashboard) update(msg tea.Msg) (*answer, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil, nil
	}
	switch k.String() {
	case "left", "shift+tab":
		b.move(-1)
	case "right", "tab":
		b.move(1)
	case "enter", "space":
		switch {
		case b.cursor == len(b.d.Actions):
			return &answer{err: ErrBack}, nil
		case b.cursor >= 0:
			return &answer{value: b.d.Actions[b.cursor]}, nil
		}
	case "esc":
		return &answer{err: ErrBack}, nil
	default:
		for _, a := range b.d.Actions {
			if a.Key == k.Text && !a.Dimmed {
				return &answer{value: a}, nil
			}
		}
	}
	return nil, nil
}

func (b *dashboard) view(width, _ int) stepView {
	help := b.d.Help
	if help == "" {
		help = "←/→ choose · enter run · or press the letter"
	}
	items := make([]string, len(b.d.Actions))
	for i, a := range b.d.Actions {
		switch {
		case i == b.cursor:
			items[i] = chip(a.Key + "  " + a.Label)
		case a.Dimmed:
			items[i] = " " + styles.Dimmed.Render(a.Key+" "+a.Label) + " "
		default:
			items[i] = " " + keyHint(a.Key, a.Label) + " "
		}
	}
	switch {
	case b.d.Back == "":
	case b.cursor == len(b.d.Actions):
		items = append(items, chip("esc  "+b.d.Back))
	default:
		items = append(items, " "+keyHint("esc", b.d.Back)+" ")
	}
	return stepView{notes: b.d.Message, notesGiveWay: true, keys: strings.Join(bar(items, width), "\n"), help: help}
}

// bar lays out items one space apart after a two-space indent, wrapping
// before an item that would cross the frame.
func bar(items []string, width int) []string {
	var lines []string
	line := " "
	for _, it := range items {
		if ansi.StringWidth(line)+1+ansi.StringWidth(it) > width && line != " " {
			lines = append(lines, line)
			line = " "
		}
		line += " " + it
	}
	return append(lines, line)
}

// chip is a focused button.
func chip(text string) string { return styles.Chip.Render(" " + text + " ") }

// dangerChip is a destructive button.
func dangerChip(text string) string { return styles.BadgeDanger.Render(text) }

// keyHint is a key and what it does: "esc Back".
func keyHint(key, label string) string {
	return styles.Key.Render(key) + " " + styles.Dimmed.Render(label)
}
