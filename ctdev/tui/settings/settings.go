// Package settings is the full-screen system-settings browser behind a bare
// `ctdev configure`: every applicable setting in one grouped, filterable list
// with its current value, a marker when it differs from the recommended value,
// and modeless editing — Enter cycles pickers/toggles, ←/→ steps sliders, r
// jumps to recommended. Changes queue up; `a` hands them back to the caller,
// which shows the usual summary + confirm before applying.
package settings

import (
	"fmt"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/setup"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/charmbracelet/x/ansi"
)

const nameColWidth = 28

// row is a category header or an index into the states slice.
type row struct {
	header bool
	title  string // header title
	idx    int    // state index for setting rows
}

type Model struct {
	states []setup.SettingState
	rows   []row

	cursor       int
	scrollOffset int
	width        int
	height       int
	filtering    bool
	filter       string

	help    help.Model
	keys    keyMap
	applied bool // user chose apply (vs quit/discard)
}

// New builds the browser over the given states. The caller owns the slice;
// edits mutate DesiredValue/Enabled in place so the existing summary+apply
// flow can consume them directly after Run.
func New(states []setup.SettingState) *Model {
	m := &Model{
		states: states,
		help:   help.New(),
		keys:   defaultKeys(),
	}
	seen := ""
	for i := range states {
		if cat := states[i].Setting.Category; cat != seen {
			seen = cat
			m.rows = append(m.rows, row{header: true, title: cat})
		}
		m.rows = append(m.rows, row{idx: i})
	}
	m.cursor = m.firstSetting()
	return m
}

// Applied reports whether the user chose to apply (vs discard) on exit.
func (m *Model) Applied() bool { return m.applied }

// PendingCount returns how many settings have a queued change.
func (m *Model) PendingCount() int {
	n := 0
	for i := range m.states {
		if m.states[i].Enabled && m.states[i].DesiredValue != m.states[i].CurrentValue {
			n++
		}
	}
	return n
}

func (m *Model) Init() tea.Cmd { return tea.RequestBackgroundColor }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.SetWidth(msg.Width)
	case tea.BackgroundColorMsg:
		styles.SetDarkBackground(msg.IsDark())
	case tea.KeyPressMsg:
		if m.filtering {
			m.updateFilter(msg)
			return m, nil
		}
		switch {
		case key.Matches(msg, m.keys.Quit):
			m.applied = false
			return m, tea.Quit
		case key.Matches(msg, m.keys.Apply):
			m.applied = true
			return m, tea.Quit
		case key.Matches(msg, m.keys.Up):
			m.moveCursor(-1)
		case key.Matches(msg, m.keys.Down):
			m.moveCursor(1)
		case key.Matches(msg, m.keys.Home):
			m.cursor = m.firstSetting()
		case key.Matches(msg, m.keys.End):
			m.cursor = m.lastSetting()
		case key.Matches(msg, m.keys.Cycle):
			m.cycle()
		case key.Matches(msg, m.keys.Dec):
			m.slider(-1)
		case key.Matches(msg, m.keys.Inc):
			m.slider(1)
		case key.Matches(msg, m.keys.Recommend):
			m.setRecommended()
		case key.Matches(msg, m.keys.Revert):
			m.revert()
		case key.Matches(msg, m.keys.Filter):
			m.filtering = true
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
		}
	}
	return m, nil
}

// --- editing ---

func (m *Model) current() *setup.SettingState {
	r := m.rows[m.cursor]
	if r.header {
		return nil
	}
	return &m.states[r.idx]
}

// value returns what a state's row should display: the queued value when a
// change is pending, else the detected current value.
func value(st *setup.SettingState) string {
	if st.Enabled {
		return st.DesiredValue
	}
	return st.CurrentValue
}

// queue records v as the pending value, clearing the pending flag when the
// user cycles back to the detected current value.
func queue(st *setup.SettingState, v string) {
	st.DesiredValue = v
	st.Enabled = v != st.CurrentValue
}

func (m *Model) cycle() {
	st := m.current()
	if st == nil {
		return
	}
	s := st.Setting
	if s.OneWay {
		// One-way settings have exactly one meaningful change: queue the
		// recommended apply (or unqueue it). Already-applied ones are done.
		if st.CurrentValue == s.Default {
			return
		}
		if st.Enabled {
			queue(st, st.CurrentValue)
		} else {
			queue(st, s.Default)
		}
		return
	}
	if s.Control == setup.ControlSlider {
		return // sliders edit with ←/→
	}
	queue(st, setup.NextValue(s, value(st)))
}

func (m *Model) slider(dir int) {
	st := m.current()
	if st == nil || st.Setting.Control != setup.ControlSlider {
		return
	}
	queue(st, setup.AdjustSlider(st.Setting, value(st), dir))
}

func (m *Model) setRecommended() {
	st := m.current()
	if st == nil || st.Setting.Default == "" {
		return
	}
	queue(st, st.Setting.Default)
}

func (m *Model) revert() {
	if st := m.current(); st != nil {
		queue(st, st.CurrentValue)
	}
}

// --- navigation / filtering (same conventions as tui/multiselect) ---

func (m *Model) matches(st *setup.SettingState) bool {
	if m.filter == "" {
		return true
	}
	f := strings.ToLower(m.filter)
	hay := strings.ToLower(st.Setting.Name + " " + st.Setting.Description + " " + st.Setting.Category)
	return strings.Contains(hay, f)
}

func (m *Model) visibleRow(i int) bool {
	r := m.rows[i]
	if r.header {
		if m.filter == "" {
			return true
		}
		for j := i + 1; j < len(m.rows) && !m.rows[j].header; j++ {
			if m.matches(&m.states[m.rows[j].idx]) {
				return true
			}
		}
		return false
	}
	return m.matches(&m.states[r.idx])
}

func (m *Model) landable(i int) bool {
	return i >= 0 && i < len(m.rows) && !m.rows[i].header && m.visibleRow(i)
}

func (m *Model) moveCursor(dir int) {
	for i := m.cursor + dir; i >= 0 && i < len(m.rows); i += dir {
		if m.landable(i) {
			m.cursor = i
			return
		}
	}
}

func (m *Model) firstSetting() int {
	for i := range m.rows {
		if m.landable(i) {
			return i
		}
	}
	return 0
}

func (m *Model) lastSetting() int {
	for i := len(m.rows) - 1; i >= 0; i-- {
		if m.landable(i) {
			return i
		}
	}
	return 0
}

func (m *Model) snapCursor() {
	if m.landable(m.cursor) {
		return
	}
	m.cursor = m.firstSetting()
}

func (m *Model) updateFilter(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "enter":
		m.filtering = false
		m.snapCursor()
	case "esc":
		m.filtering = false
		m.filter = ""
		m.snapCursor()
	case "backspace":
		if r := []rune(m.filter); len(r) > 0 {
			m.filter = string(r[:len(r)-1])
			m.snapCursor()
		}
	case "space":
		m.filter += " "
		m.snapCursor()
	default:
		if r := []rune(msg.String()); len(r) == 1 && unicode.IsPrint(r[0]) {
			m.filter += string(r)
			m.snapCursor()
		}
	}
}

// --- view ---

func (m *Model) View() tea.View {
	header := m.headerView()
	footer := m.footerView()
	chrome := lipgloss.Height(header) + lipgloss.Height(footer)

	var visible []int
	for i := range m.rows {
		if m.visibleRow(i) {
			visible = append(visible, i)
		}
	}
	window, above, below := m.window(visible, chrome)

	lines := []string{header}
	if above > 0 {
		lines = append(lines, styles.Dimmed.Render(fmt.Sprintf("  ↑ %d more", above)))
	}
	for _, i := range window {
		line := m.renderRow(i)
		if i == m.cursor {
			w := m.width
			if w <= 0 {
				w = lipgloss.Width(line)
			}
			line = styles.Cursor.Width(w).Inline(true).MaxWidth(w).Render(ansi.Strip(line))
		} else if m.width > 0 {
			line = ansi.Truncate(line, m.width, "…")
		}
		lines = append(lines, line)
	}
	if below > 0 {
		lines = append(lines, styles.Dimmed.Render(fmt.Sprintf("  ↓ %d more", below)))
	}
	lines = append(lines, footer)

	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}

func (m *Model) headerView() string {
	left := styles.Title.Render("System settings")
	right := styles.Dimmed.Render("no changes")
	if n := m.PendingCount(); n > 0 {
		right = styles.Warning.Render(fmt.Sprintf("%d pending change(s)", n))
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	out := left + strings.Repeat(" ", gap) + right
	if m.filtering || m.filter != "" {
		caret := ""
		if m.filtering {
			caret = "█"
		}
		out += "\n\n" + styles.Help.Render("Filter: ") + styles.Value.Render(m.filter) + caret
	}
	return out + "\n"
}

func (m *Model) renderRow(i int) string {
	r := m.rows[i]
	if r.header {
		return styles.Header.Render("▾ " + r.title)
	}
	st := &m.states[r.idx]
	s := st.Setting

	name := styles.Value.Width(nameColWidth).Render(truncate(s.Name, nameColWidth))
	val := value(st)

	parts := []string{"  " + name}
	if st.Enabled {
		parts = append(parts, styles.Dimmed.Render(st.CurrentValue+" →"), styles.Warning.Render(val))
	} else {
		parts = append(parts, styles.Value.Render(val))
	}
	if s.Default != "" && val != s.Default {
		parts = append(parts, styles.BadgeWarn.Render("≠ REC"))
	} else if s.OneWay && st.CurrentValue == s.Default {
		parts = append(parts, styles.Success.Render("✓"))
	}
	return strings.Join(parts, " ")
}

// footerView is the detail pane for the cursor row plus the help bar.
func (m *Model) footerView() string {
	var b strings.Builder
	b.WriteString("\n")
	if st := m.current(); st != nil {
		s := st.Setting
		desc := s.Description
		if m.width > 4 {
			desc = ansi.Truncate(desc, m.width-4, "…")
		}
		b.WriteString("  " + styles.Dimmed.Render(desc) + "\n")
		detail := fmt.Sprintf("Current: %s", st.CurrentValue)
		if s.Default != "" {
			detail += fmt.Sprintf(" · Recommended: %s", s.Default)
		}
		if r := s.Slider; r != nil {
			detail += fmt.Sprintf(" · Range: %s–%s%s (←/→ step %s)",
				setup.FormatSliderVal(r.Min, r.Step), setup.FormatSliderVal(r.Max, r.Step), r.Unit,
				setup.FormatSliderVal(r.Step, r.Step))
		}
		b.WriteString("  " + styles.Help.Render(detail) + "\n")
	}
	b.WriteString(styles.Help.Render(m.help.View(m.keys)))
	return b.String()
}

func (m *Model) window(visible []int, chrome int) (window []int, above, below int) {
	if m.height <= 0 {
		m.scrollOffset = 0
		return visible, 0, 0
	}
	avail := m.height - chrome
	if avail >= len(visible) {
		m.scrollOffset = 0
		return visible, 0, 0
	}
	capacity := avail - 2
	if capacity < 3 {
		capacity = 3
	}
	pos := 0
	for p, idx := range visible {
		if idx == m.cursor {
			pos = p
			break
		}
	}
	if pos < m.scrollOffset {
		m.scrollOffset = pos
	}
	if pos >= m.scrollOffset+capacity {
		m.scrollOffset = pos - capacity + 1
	}
	if mx := len(visible) - capacity; m.scrollOffset > mx {
		m.scrollOffset = mx
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	end := m.scrollOffset + capacity
	if end > len(visible) {
		end = len(visible)
	}
	return visible[m.scrollOffset:end], m.scrollOffset, len(visible) - end
}

func truncate(s string, max int) string {
	if lipgloss.Width(s) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > max {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}
