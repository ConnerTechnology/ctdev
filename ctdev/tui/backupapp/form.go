package backupapp

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// labelWidth is a form label's column; values start at column 21, or
// further right when a label is longer.
const labelWidth = 16

// form is a Form's step.
type form struct {
	f      Form
	values Values
	errs   map[string]string
	focus  int      // an index into f.Fields, always a shown field
	typing []string // a kit typed line by line so far, not yet ended
	line   string   // the kit line being typed
}

func newForm(f Form) *form {
	fm := &form{f: f, values: Values{}, errs: map[string]string{}}
	for _, fd := range f.Fields {
		fm.values[fd.Key] = fd.Value
		if fd.Error != "" {
			fm.errs[fd.Key] = fd.Error
		}
	}
	fm.focus = -1
	fm.move(1)
	for i, fd := range f.Fields {
		if fd.Error != "" && fm.shown(i) {
			fm.focus = i
			break
		}
	}
	return fm
}

func (fm *form) init() tea.Cmd { return nil }

func (fm *form) shown(i int) bool {
	s := fm.f.Fields[i].Show
	return s == nil || s(fm.values)
}

// move takes focus to the next shown field in dir, checking the one it
// leaves.
func (fm *form) move(dir int) {
	if fm.focus >= 0 {
		fm.check(fm.focus)
	}
	for i := fm.focus + dir; i >= 0 && i < len(fm.f.Fields); i += dir {
		if fm.shown(i) {
			fm.focus = i
			return
		}
	}
}

// check validates field i, keeping or clearing its error; true when valid.
func (fm *form) check(i int) bool {
	fd := fm.f.Fields[i]
	v := fm.values[fd.Key]
	msg := ""
	switch {
	case fd.Kind == KitPaste && v == "":
		msg = fm.errs[fd.Key] // a parse error stands until the next paste
		if msg == "" && fd.Required {
			msg = "Paste the Emergency Kit."
		}
	case fd.Required && strings.TrimSpace(v) == "":
		msg = "Required — please enter a value."
	case fd.Kind == Number && v != "":
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err != nil || n < 0 {
			msg = "Enter a whole number, 0 or more."
		}
	}
	if msg == "" && fd.Validate != nil {
		msg = fd.Validate(v)
	}
	if msg == "" {
		delete(fm.errs, fd.Key)
	} else {
		fm.errs[fd.Key] = msg
	}
	return msg == ""
}

// submit checks every shown field and answers with their values, or takes
// focus to the first that isn't valid.
func (fm *form) submit() *answer {
	first := -1
	for i := range fm.f.Fields {
		if fm.shown(i) && !fm.check(i) && first < 0 {
			first = i
		}
	}
	if first >= 0 {
		fm.focus = first
		return nil
	}
	vals := Values{}
	for i, fd := range fm.f.Fields {
		if fm.shown(i) {
			vals[fd.Key] = strings.TrimSpace(fm.values[fd.Key])
			if fd.Kind == KitPaste || fd.Kind == Secret {
				vals[fd.Key] = fm.values[fd.Key]
			}
		}
	}
	return &answer{value: vals}
}

func (fm *form) update(msg tea.Msg) (*answer, tea.Cmd) {
	if fm.focus < 0 {
		if k, ok := msg.(tea.KeyPressMsg); ok {
			switch k.String() {
			case "esc":
				return &answer{err: ErrBack}, nil
			case "enter":
				return fm.submit(), nil
			}
		}
		return nil, nil
	}
	fd := fm.f.Fields[fm.focus]
	if p, ok := msg.(tea.PasteMsg); ok {
		if fd.Kind == KitPaste {
			fm.takeKit(fd.Key, p.Content)
		} else if fd.Kind != Choice {
			fm.values[fd.Key] += strings.TrimSpace(strings.ReplaceAll(p.Content, "\n", ""))
		}
		return nil, nil
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil, nil
	}
	if fd.Kind == KitPaste && (fm.line != "" || len(fm.typing) > 0) {
		return fm.typeKit(fd.Key, k), nil
	}
	switch k.String() {
	case "esc":
		return &answer{err: ErrBack}, nil
	case "tab":
		fm.move(1)
	case "shift+tab":
		fm.move(-1)
	case "up", "down":
		dir := 1
		if k.String() == "up" {
			dir = -1
		}
		if fd.Kind == Choice {
			fm.choose(fd, dir)
		} else {
			fm.move(dir)
		}
	case "enter":
		return fm.submit(), nil
	case "backspace":
		if fd.Kind != Choice && fd.Kind != KitPaste {
			if r := []rune(fm.values[fd.Key]); len(r) > 0 {
				fm.values[fd.Key] = string(r[:len(r)-1])
			}
		}
	case "ctrl+u":
		if fd.Kind != Choice {
			fm.values[fd.Key] = ""
		}
	default:
		switch fd.Kind {
		case Choice:
		case KitPaste:
			if k.Text != "" {
				fm.line = k.Text
			}
		default:
			fm.values[fd.Key] += k.Text
		}
	}
	return nil, nil
}

// choose moves a Choice field's value to the option in dir.
func (fm *form) choose(fd Field, dir int) {
	at := 0
	for i, o := range fd.Options {
		if o.Value == fm.values[fd.Key] {
			at = i
		}
	}
	if j := at + dir; j >= 0 && j < len(fd.Options) {
		fm.values[fd.Key] = fd.Options[j].Value
	}
}

// typeKit takes a kit typed line by line, without bracketed paste: enter
// ends a line, and an empty line ends the kit.
func (fm *form) typeKit(key string, k tea.KeyPressMsg) *answer {
	switch k.String() {
	case "esc":
		fm.typing, fm.line = nil, ""
	case "enter":
		if fm.line == "" {
			fm.takeKit(key, strings.Join(fm.typing, "\n"))
			fm.typing = nil
			break
		}
		fm.typing, fm.line = append(fm.typing, fm.line), ""
	case "backspace":
		if r := []rune(fm.line); len(r) > 0 {
			fm.line = string(r[:len(r)-1])
		}
	default:
		fm.line += k.Text
	}
	return nil
}

// takeKit parses a pasted kit: kept when it parses, emptied for the next
// paste when it doesn't.
func (fm *form) takeKit(key, text string) {
	// Terminals paste line ends as carriage returns, as if typed.
	text = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(text)
	if _, err := backup.ParseKit(text); err != nil {
		fm.values[key] = ""
		fm.errs[key] = capitalize(err.Error()) + ": paste the whole kit again."
		return
	}
	fm.values[key] = text
	delete(fm.errs, key)
}

func capitalize(s string) string {
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}

func (fm *form) view(width, _ int) stepView {
	var lines []string
	if fm.f.Intro != "" {
		lines = append(lines, strings.Split(fm.f.Intro, "\n")...)
		lines = append(lines, "")
	}
	help := "tab next field · shift+tab previous"
	width := labelWidth
	for _, fd := range fm.f.Fields {
		width = max(width, len(fd.Label))
	}
	for i, fd := range fm.f.Fields {
		if !fm.shown(i) {
			continue
		}
		focused := i == fm.focus
		mark := "  "
		if focused {
			mark = "▸ "
		}
		label := mark + styles.Dimmed.Render(fmt.Sprintf("%-*s", width, fd.Label)) + "   "
		v := fm.values[fd.Key]
		caret := ""
		if focused {
			caret = styles.Value.Render("█")
		}
		switch fd.Kind {
		case Choice:
			for j, o := range fd.Options {
				pre := label
				if j > 0 {
					pre = strings.Repeat(" ", 2+width+3)
				}
				if o.Value == v {
					lines = append(lines, pre+styles.Success.Render("◉")+" "+styles.Value.Render(o.Label))
				} else {
					lines = append(lines, pre+styles.Dimmed.Render("○ "+o.Label))
				}
			}
			if focused {
				help = "↑/↓ choose · tab next field · shift+tab previous"
			}
		case Secret:
			if v != "" {
				v = "••••••••••"
			}
			lines = append(lines, label+styles.Value.Render(v)+caret)
		case KitPaste:
			lines = append(lines, label+fm.kitState(v))
			if focused {
				help = "paste the kit, or type it and end with an empty line · esc back"
			}
		default:
			lines = append(lines, label+styles.Value.Render(v)+caret)
		}
		under := func(l Line) {
			for _, w := range wrap(l, width-2, 2) {
				lines = append(lines, "  "+w)
			}
		}
		if e := fm.errs[fd.Key]; e != "" {
			under(Line{Error, e})
		}
		if fd.Note != "" {
			under(Line{Info, fd.Note})
		}
	}
	var notes []Line
	if fm.f.Error != "" {
		notes = append(notes, Line{Error, fm.f.Error})
	}
	keys := "  " + chip("enter  "+orDefault(fm.f.Submit, "Next")) + "   " + keyHint("esc", orDefault(fm.f.Back, "Back"))
	return stepView{content: lines, notes: notes, keys: keys, help: orDefault(fm.f.Help, help)}
}

// kitState says where a kit paste field is, never showing the kit.
func (fm *form) kitState(v string) string {
	switch {
	case fm.line != "" || len(fm.typing) > 0:
		n := len(fm.typing)
		if fm.line != "" {
			n++
		}
		return styles.Dimmed.Render(fmt.Sprintf("reading the kit… %d lines; an empty line ends it", n))
	case v != "":
		k, _ := backup.ParseKit(v)
		return styles.Success.Render("✓") + " " + styles.Value.Render(fmt.Sprintf("Pasted an Emergency Kit for %s (%s)", k.Destination, backup.BackendLabel(k.Address)))
	}
	return styles.Dimmed.Render("waiting for a paste…")
}
