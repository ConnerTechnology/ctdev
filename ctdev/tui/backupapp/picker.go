package backupapp

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
	"github.com/charmbracelet/x/ansi"
)

// picker is a Picker's step.
type picker struct {
	p      Picker
	cursor int
}

func newPicker(p Picker) *picker {
	pk := &picker{p: p, cursor: -1}
	if p.Cursor >= 0 && p.Cursor < len(p.Items) && !p.Items[p.Cursor].Dimmed {
		pk.cursor = p.Cursor
	} else {
		pk.move(1)
	}
	return pk
}

func (pk *picker) init() tea.Cmd { return nil }

func (pk *picker) move(dir int) {
	for i := pk.cursor + dir; i >= 0 && i < len(pk.p.Items); i += dir {
		if !pk.p.Items[i].Dimmed {
			pk.cursor = i
			return
		}
	}
}

func (pk *picker) update(msg tea.Msg) (*answer, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil, nil
	}
	switch k.String() {
	case "up", "k", "shift+tab":
		pk.move(-1)
	case "down", "j", "tab":
		pk.move(1)
	case "enter", "space":
		if pk.cursor >= 0 {
			return &answer{value: pk.cursor}, nil
		}
	case "esc":
		return &answer{err: ErrBack}, nil
	case "q":
		if pk.p.Quit {
			return &answer{err: ErrBack}, nil
		}
	}
	return nil, nil
}

func (pk *picker) view(width, height int) stepView {
	p := pk.p
	var lines []string
	if p.Heading != "" {
		h := "  " + styles.Header.Render(p.Heading)
		if p.Hint != "" {
			h += "  " + styles.Dimmed.Render(p.Hint)
		}
		lines = append(lines, h, "")
	}
	if p.Intro != "" {
		lines = append(lines, strings.Split(p.Intro, "\n")...)
		lines = append(lines, "")
	}

	widths := pk.widths()
	indent := "  "
	if pk.grouped() {
		indent += "  "
	}
	if len(p.Columns) > 0 {
		var titles []string
		for _, c := range p.Columns {
			titles = append(titles, c.Title)
		}
		lines = append(lines, indent+styles.Dimmed.Render(strings.TrimRight(pk.cells(titles, widths), " ")))
	}

	first, last := pk.window(height)
	if first > 0 {
		lines = append(lines, "  "+styles.Dimmed.Render(fmt.Sprintf("↑ %d more", first)))
	}
	for i := first; i < last; i++ {
		it := p.Items[i]
		if it.Group != "" && (i == first || p.Items[i-1].Group != it.Group) {
			if i > first {
				lines = append(lines, "")
			}
			lines = append(lines, "  "+styles.Value.Render(it.Group))
		}
		row := strings.TrimRight(pk.cells(it.Cells, widths), " ")
		switch {
		case i == pk.cursor:
			plain := indent + ansi.Strip(row)
			if p.Radio {
				plain = indent + "◉ " + ansi.Strip(row)
			}
			lines = append(lines, styles.Cursor.Render(plain+strings.Repeat(" ", max(width-ansi.StringWidth(plain), 0))))
		case p.Radio:
			lines = append(lines, indent+styles.Dimmed.Render("○ "+ansi.Strip(row)))
		case it.Dimmed:
			lines = append(lines, indent+styles.Dimmed.Render(ansi.Strip(row)))
		default:
			lines = append(lines, indent+row)
		}
		if it.Detail != "" {
			detail := indent + "  "
			if p.Radio {
				detail += "  "
			}
			lines = append(lines, detail+styles.Dimmed.Render(it.Detail))
		}
	}
	if last < len(p.Items) {
		lines = append(lines, "  "+styles.Dimmed.Render(fmt.Sprintf("↓ %d more", len(p.Items)-last)))
	}
	if p.Note != "" {
		lines = append(lines, "", "  "+styles.Dimmed.Render(p.Note))
	}

	enter, back := orDefault(p.Enter, "Open"), orDefault(p.Back, "Back")
	keys := "  " + chip("enter  "+enter) + "   " + keyHint("esc", back)
	help := "↑/↓ move · enter " + strings.ToLower(enter) + " · esc " + strings.ToLower(back)
	if p.Quit {
		keys = "  " + chip("enter  "+enter) + "   " + keyHint("q", "Quit")
		help = "↑/↓ move · enter " + strings.ToLower(enter) + " · q quit"
	}
	return stepView{content: lines, keys: keys, help: orDefault(p.Help, help)}
}

func (pk *picker) grouped() bool {
	for _, it := range pk.p.Items {
		if it.Group != "" {
			return true
		}
	}
	return false
}

// widths are each column's: the widest of its title and cells.
func (pk *picker) widths() []int {
	var w []int
	grow := func(cells []string) {
		for i, c := range cells {
			if i >= len(w) {
				w = append(w, 0)
			}
			w[i] = max(w[i], ansi.StringWidth(c))
		}
	}
	var titles []string
	for _, c := range pk.p.Columns {
		titles = append(titles, c.Title)
	}
	grow(titles)
	for _, it := range pk.p.Items {
		grow(it.Cells)
	}
	return w
}

// cells lays a row out in columns three apart, right-aligning the columns
// that ask for it.
func (pk *picker) cells(cells []string, widths []int) string {
	var b strings.Builder
	for i, c := range cells {
		pad := strings.Repeat(" ", widths[i]-ansi.StringWidth(c))
		if i < len(pk.p.Columns) && pk.p.Columns[i].Right {
			b.WriteString(pad + c)
		} else {
			b.WriteString(c + pad)
		}
		if i < len(cells)-1 {
			b.WriteString("   ")
		}
	}
	return b.String()
}

// window is the items that fit on screen around the cursor; every item when
// the height isn't known.
func (pk *picker) window(height int) (first, last int) {
	n := len(pk.p.Items)
	if n == 0 {
		return 0, 0
	}
	room := height - 16
	if height <= 0 || n <= room {
		return 0, n
	}
	room = max(room, 3)
	first = max(0, min(pk.cursor-room/2, n-room))
	return first, min(first+room, n)
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
