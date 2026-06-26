// Package multiselect is a reusable grouped multi-select list for the ctdev
// TUIs (the update checklist and the install/uninstall picker). It owns the
// shared interaction logic — cursor navigation, a scrolling window with sticky
// header/footer, incremental filtering, bulk select/none/invert, per-group
// toggles, a discoverable help bar (bubbles help+key), and a full-width cursor
// highlight — while callers supply only the grouped data to render.
package multiselect

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/charmbracelet/x/ansi"
)

// nameColWidth is the fixed width of the primary-label column; longer labels
// are truncated with an ellipsis so the detail column stays aligned.
const nameColWidth = 22

// Badge is a small labeled tag rendered after an item (e.g. "KERNEL").
type Badge struct {
	Text  string
	Style lipgloss.Style
}

// Item is one selectable row.
type Item struct {
	ID          string // stable selection key, unique across the whole list
	Primary     string // main label (component / package name)
	Secondary   string // dimmed detail (version transition or description)
	Note        string // dimmed trailing note (e.g. "(linux only)")
	Search      string // extra text folded into filter matching (e.g. tags)
	Badges      []Badge
	Group       string // owning group key (set by New)
	Selectable  bool   // cursor may land on it and Space toggles it
	Bulk        bool   // included by select-all / none / invert / group toggle
	Marked      bool   // pre-existing state (e.g. already installed) → ● glyph
	NoPreselect bool   // excluded from Options.PreselectAll (e.g. a risky default)
}

// Group is an ordered section of items with a display title.
type Group struct {
	Key   string
	Title string
	Items []Item
}

// Options configures a Model.
type Options struct {
	Title        string // screen title
	StatusSuffix string // static text appended after "N selected"
	PreselectAll bool   // start with every Bulk item selected
}

// Result is the outcome of a run.
type Result struct {
	Selected []string // item IDs in display order
	Quit     bool
}

// row is the flattened display unit: a group header or an item.
type row struct {
	header bool
	group  string
	item   Item
}

// Model is the shared multi-select widget. Embed it (or wrap it) in a
// domain-specific tea.Model; see the checklist and picker packages.
type Model struct {
	title        string
	statusSuffix string
	rows         []row
	titles       map[string]string
	selected     map[string]bool
	collapsed    map[string]bool
	cursor       int
	scrollOffset int
	filtering    bool
	filter       string
	width        int
	height       int
	help         help.Model
	keys         keyMap
	quitting     bool
	confirmed    bool
}

// New flattens groups into a Model, optionally pre-selecting every Bulk item.
func New(groups []Group, opts Options) *Model {
	var rows []row
	titles := map[string]string{}
	selected := map[string]bool{}
	for _, g := range groups {
		titles[g.Key] = g.Title
		rows = append(rows, row{header: true, group: g.Key})
		for _, it := range g.Items {
			it.Group = g.Key
			rows = append(rows, row{group: g.Key, item: it})
			if opts.PreselectAll && it.Bulk && !it.NoPreselect {
				selected[it.ID] = true
			}
		}
	}
	m := &Model{
		title:        opts.Title,
		statusSuffix: opts.StatusSuffix,
		rows:         rows,
		titles:       titles,
		selected:     selected,
		collapsed:    map[string]bool{},
		help:         help.New(),
		keys:         defaultKeys(),
	}
	m.cursor = m.firstLandable()
	return m
}

// Init asks the terminal for its background color so the palette can adapt to a
// light theme. The reply arrives asynchronously as a tea.BackgroundColorMsg, so
// this never blocks startup (unlike a synchronous query over e.g. Mosh).
func (m *Model) Init() tea.Cmd { return tea.RequestBackgroundColor }

// Update mutates the model in place and returns any command. Wrappers call this
// from their own tea.Model.Update and return their outer pointer.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
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
			return nil
		}
		switch {
		case key.Matches(msg, m.keys.Quit):
			m.quitting = true
			return tea.Quit
		case key.Matches(msg, m.keys.Confirm):
			m.confirmed = true
			return tea.Quit
		case key.Matches(msg, m.keys.Up):
			m.moveCursor(-1)
		case key.Matches(msg, m.keys.Down):
			m.moveCursor(1)
		case key.Matches(msg, m.keys.Home):
			m.cursor = m.firstLandable()
		case key.Matches(msg, m.keys.End):
			m.cursor = m.lastLandable()
		case key.Matches(msg, m.keys.Toggle):
			m.toggle()
		case key.Matches(msg, m.keys.Group):
			m.collapse()
		case key.Matches(msg, m.keys.All):
			m.bulk(func(bool) bool { return true })
		case key.Matches(msg, m.keys.None):
			m.bulk(func(bool) bool { return false })
		case key.Matches(msg, m.keys.Invert):
			m.bulk(func(cur bool) bool { return !cur })
		case key.Matches(msg, m.keys.Filter):
			m.filtering = true
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
		}
	}
	return nil
}

// Result returns the selected item IDs in display order (or Quit).
func (m *Model) Result() Result {
	if m.quitting && !m.confirmed {
		return Result{Quit: true}
	}
	var ids []string
	for _, r := range m.rows {
		if r.header {
			continue
		}
		if m.selected[r.item.ID] {
			ids = append(ids, r.item.ID)
		}
	}
	return Result{Selected: ids}
}

// --- selection / navigation ---

func (m *Model) toggle() {
	r := m.rows[m.cursor]
	if r.header {
		m.toggleGroup(r.group)
		return
	}
	if !r.item.Selectable {
		return
	}
	if m.selected[r.item.ID] {
		delete(m.selected, r.item.ID)
	} else {
		m.selected[r.item.ID] = true
	}
}

// toggleGroup selects every selectable item in a group, or clears them all when
// they are already fully selected (tri-state parent behavior).
func (m *Model) toggleGroup(key string) {
	total, sel := m.groupCounts(key)
	target := sel < total
	for _, r := range m.rows {
		if r.header || r.group != key || !r.item.Selectable {
			continue
		}
		if target {
			m.selected[r.item.ID] = true
		} else {
			delete(m.selected, r.item.ID)
		}
	}
}

// bulk applies fn to every Bulk item currently matching the filter, leaving
// items outside the filter untouched (so filter→all→clear→repeat accumulates).
func (m *Model) bulk(fn func(cur bool) bool) {
	for _, r := range m.rows {
		if r.header || !r.item.Selectable || !r.item.Bulk || !m.matchesItem(r.item) {
			continue
		}
		if fn(m.selected[r.item.ID]) {
			m.selected[r.item.ID] = true
		} else {
			delete(m.selected, r.item.ID)
		}
	}
}

// collapse toggles the current row's group and parks the cursor on its header.
func (m *Model) collapse() {
	g := m.rows[m.cursor].group
	m.collapsed[g] = !m.collapsed[g]
	for i, r := range m.rows {
		if r.header && r.group == g {
			m.cursor = i
			break
		}
	}
	m.snapCursor()
}

func (m *Model) moveCursor(dir int) {
	for {
		m.cursor += dir
		if m.cursor < 0 {
			m.cursor = 0
			break
		}
		if m.cursor >= len(m.rows) {
			m.cursor = len(m.rows) - 1
			break
		}
		if m.landable(m.cursor) {
			return
		}
	}
	m.snapCursor()
}

// snapCursor moves the cursor to the nearest landable row (forward then back).
func (m *Model) snapCursor() {
	if m.landable(m.cursor) {
		return
	}
	for i := m.cursor + 1; i < len(m.rows); i++ {
		if m.landable(i) {
			m.cursor = i
			return
		}
	}
	for i := m.cursor - 1; i >= 0; i-- {
		if m.landable(i) {
			m.cursor = i
			return
		}
	}
}

func (m *Model) firstLandable() int {
	for i := range m.rows {
		if m.landable(i) {
			return i
		}
	}
	return 0
}

func (m *Model) lastLandable() int {
	for i := len(m.rows) - 1; i >= 0; i-- {
		if m.landable(i) {
			return i
		}
	}
	return 0
}

// landable reports whether the cursor may rest on a row: it must be visible and
// either a header (collapsible / group-toggle) or a selectable item.
func (m *Model) landable(i int) bool {
	if i < 0 || i >= len(m.rows) || !m.visibleRow(i) {
		return false
	}
	r := m.rows[i]
	return r.header || r.item.Selectable
}

// visibleRow reports whether a row is currently rendered.
func (m *Model) visibleRow(i int) bool {
	r := m.rows[i]
	if r.header {
		return m.filter == "" || m.groupHasMatch(r.group)
	}
	if m.collapsed[r.group] {
		return false
	}
	return m.matchesItem(r.item)
}

func (m *Model) groupHasMatch(key string) bool {
	for _, r := range m.rows {
		if !r.header && r.group == key && m.matchesItem(r.item) {
			return true
		}
	}
	return false
}

// matchesItem reports whether an item passes the active filter (collapse state
// is ignored — collapse is a view convenience, not a filter).
func (m *Model) matchesItem(it Item) bool {
	if m.filter == "" {
		return true
	}
	f := strings.ToLower(m.filter)
	hay := strings.ToLower(strings.Join([]string{
		it.Primary, it.Secondary, it.Note, it.Search, m.titles[it.Group],
	}, " "))
	for _, b := range it.Badges {
		hay += " " + strings.ToLower(b.Text)
	}
	return strings.Contains(hay, f)
}

// groupCounts returns the number of selectable items in a group and how many of
// them are selected.
func (m *Model) groupCounts(key string) (total, selected int) {
	for _, r := range m.rows {
		if r.header || r.group != key || !r.item.Selectable {
			continue
		}
		total++
		if m.selected[r.item.ID] {
			selected++
		}
	}
	return total, selected
}

// --- filter input ---

func (m *Model) updateFilter(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "enter":
		m.filtering = false
		m.onFilterChanged()
	case "esc":
		m.filtering = false
		m.filter = ""
		m.snapCursor()
	case "backspace":
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.onFilterChanged()
		}
	default:
		if s := msg.String(); len(s) == 1 {
			m.filter += s
			m.onFilterChanged()
		}
	}
}

// onFilterChanged lands the cursor on the first visible item after the filter
// string changes, so the highlight is never stranded on a filtered-out row.
func (m *Model) onFilterChanged() {
	for i := range m.rows {
		if !m.rows[i].header && m.landable(i) {
			m.cursor = i
			return
		}
	}
	m.cursor = 0
	m.snapCursor()
}

// --- view ---

func (m *Model) View() tea.View {
	header := m.headerView()
	footer := m.footerView()
	chrome := lipgloss.Height(header) + lipgloss.Height(footer)

	visible := m.visibleIndices()
	window, above, below := m.windowFor(visible, chrome)

	lines := []string{header}
	if above > 0 {
		lines = append(lines, styles.Dimmed.Render(fmt.Sprintf("  ↑ %d more", above)))
	}
	for _, i := range window {
		r := m.rows[i]
		line := m.renderItem(r)
		if i == m.cursor {
			line = m.highlight(line)
		} else if m.width > 0 {
			// Clip to the terminal width so a long detail/badge row can't wrap and
			// throw off the one-line-per-row scroll math.
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
	left := styles.Title.Render(m.title)
	right := m.statusText()
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

func (m *Model) statusText() string {
	status := styles.Success.Render(fmt.Sprintf("%d selected", len(m.selected)))
	if m.statusSuffix != "" {
		status += styles.Dimmed.Render(" " + m.statusSuffix)
	}
	return status
}

func (m *Model) footerView() string {
	return styles.Help.Render(m.help.View(m.keys))
}

func (m *Model) renderItem(r row) string {
	if r.header {
		return m.renderHeader(r.group)
	}
	it := r.item
	var ind string
	switch {
	case !it.Selectable:
		ind = styles.Dimmed.Render("⊘")
	case m.selected[it.ID]:
		ind = styles.Selected.String()
	case it.Marked:
		ind = styles.Success.Render("●")
	default:
		ind = styles.Unselected.String()
	}

	nameStyle := styles.Value
	if !it.Selectable {
		nameStyle = styles.Dimmed
	}
	name := nameStyle.Width(nameColWidth).Render(truncate(it.Primary, nameColWidth))

	parts := []string{"  " + ind + " " + name}
	if it.Secondary != "" {
		parts = append(parts, styles.Dimmed.Render(it.Secondary))
	}
	for _, b := range it.Badges {
		parts = append(parts, b.Style.Render(b.Text))
	}
	if it.Note != "" {
		parts = append(parts, styles.Dimmed.Render(it.Note))
	}
	return strings.Join(parts, " ")
}

func (m *Model) renderHeader(key string) string {
	arrow := "▾"
	if m.collapsed[key] {
		arrow = "▸"
	}
	total, sel := m.groupCounts(key)
	count := fmt.Sprintf(" · %d", total)
	switch {
	case total > 0 && sel == total:
		count = fmt.Sprintf(" · %d ✓", total)
	case sel > 0:
		count = fmt.Sprintf(" · %d/%d", sel, total)
	}
	return styles.Header.Render(arrow+" "+m.titles[key]) + styles.Dimmed.Render(count)
}

// highlight renders a row as a solid full-width bar. The content is stripped to
// plain text first so the bar is unbroken (embedded foreground colors would
// otherwise punch holes in the background); the glyphs still convey state.
func (m *Model) highlight(content string) string {
	w := m.width
	if w <= 0 {
		w = lipgloss.Width(content)
	}
	return styles.Cursor.
		Width(w).Inline(true).MaxWidth(w).
		Render(ansi.Strip(content))
}

func (m *Model) visibleIndices() []int {
	var out []int
	for i := range m.rows {
		if m.visibleRow(i) {
			out = append(out, i)
		}
	}
	return out
}

// windowFor slices visible into the rows that fit below the chrome, scrolling to
// keep the cursor in view. It returns the window plus the counts hidden above
// and below (for the "N more" indicators).
func (m *Model) windowFor(visible []int, chrome int) (window []int, above, below int) {
	if m.height <= 0 {
		m.scrollOffset = 0
		return visible, 0, 0
	}
	avail := m.height - chrome
	if avail >= len(visible) {
		m.scrollOffset = 0
		return visible, 0, 0
	}
	if avail < 3 {
		avail = 3
	}
	capacity := avail - 2 // reserve a line each for the ↑/↓ indicators
	if capacity < 1 {
		capacity = 1
	}

	cursorPos := 0
	for p, idx := range visible {
		if idx == m.cursor {
			cursorPos = p
			break
		}
	}
	if cursorPos < m.scrollOffset {
		m.scrollOffset = cursorPos
	}
	if cursorPos >= m.scrollOffset+capacity {
		m.scrollOffset = cursorPos - capacity + 1
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

// truncate shortens s to max display cells, appending an ellipsis when cut.
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
