package picker

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
)

type item struct {
	component   component.Component
	isCategory  bool
	category    component.Category
	collapsed   bool
	installed   bool
	unsupported bool
}

type Mode int

const (
	ModeInstall Mode = iota
	ModeUninstall
)

type Model struct {
	items        []item
	cursor       int
	selected     map[string]bool
	filtering    bool
	filter       string
	quitting     bool
	confirmed    bool
	width        int
	height       int
	scrollOffset int
	platform     component.OS
	mode         Mode
}

type Result struct {
	Selected []string
	Quit     bool
}

func New(components []component.Component, installed map[string]bool, os component.OS, mode Mode) Model {
	groups := component.GroupByCategory(components)

	categoryOrder := []component.Category{
		component.CategoryCLI,
		component.CategoryDesktop,
		component.CategoryRuntime,
		component.CategorySecurity,
		component.CategoryInfra,
		component.CategorySystem,
	}

	var items []item
	for _, cat := range categoryOrder {
		comps, ok := groups[cat]
		if !ok || len(comps) == 0 {
			continue
		}
		items = append(items, item{isCategory: true, category: cat})
		for _, c := range comps {
			items = append(items, item{
				component:   c,
				installed:   installed[c.Name],
				unsupported: !c.SupportsOS(os),
			})
		}
	}

	m := Model{
		items:    items,
		selected: make(map[string]bool),
		platform: os,
		mode:     mode,
	}
	// Advance cursor past initial category header to first selectable item
	if len(items) > 0 && items[0].isCategory {
		m.moveCursor(1)
	}
	return m
}

func (inst *Model) Init() tea.Cmd {
	return nil
}

func (inst *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		inst.width = msg.Width
		inst.height = msg.Height
		return inst, nil

	case tea.KeyPressMsg:
		if inst.filtering {
			return inst.updateFilter(msg)
		}

		switch msg.String() {
		case "q", "ctrl+c":
			inst.quitting = true
			return inst, tea.Quit
		case "enter":
			inst.confirmed = true
			return inst, tea.Quit
		case "up", "k":
			inst.moveCursor(-1)
		case "down", "j":
			inst.moveCursor(1)
		case "space":
			inst.toggleSelected()
		case "tab":
			inst.toggleCategory()
		case "/":
			inst.filtering = true
			inst.filter = ""
		case "a":
			inst.selectAll()
		case "n":
			inst.selectNone()
		}
	}
	return inst, nil
}

// visible reports whether a row is currently rendered: not collapsed under a
// category and (for components) matching the active filter.
func (inst *Model) visible(idx int) bool {
	if inst.isHidden(idx) {
		return false
	}
	it := inst.items[idx]
	if it.isCategory {
		// While filtering, drop category headers whose children all filter out.
		return inst.filter == "" || inst.categoryHasMatch(idx)
	}
	return inst.matchesFilter(it.component)
}

// categoryHasMatch reports whether any component under the category at catIdx
// matches the active filter.
func (inst *Model) categoryHasMatch(catIdx int) bool {
	for i := catIdx + 1; i < len(inst.items) && !inst.items[i].isCategory; i++ {
		if inst.matchesFilter(inst.items[i].component) {
			return true
		}
	}
	return false
}

// landable reports whether the cursor may rest on a row — it must be visible
// and selectable (categories are landable so Tab can collapse them).
func (inst *Model) landable(idx int) bool {
	return idx >= 0 && idx < len(inst.items) && inst.visible(idx) && !inst.items[idx].unsupported
}

func (inst *Model) moveCursor(dir int) {
	for {
		inst.cursor += dir
		if inst.cursor < 0 {
			inst.cursor = 0
			break
		}
		if inst.cursor >= len(inst.items) {
			inst.cursor = len(inst.items) - 1
			break
		}
		if inst.landable(inst.cursor) {
			return
		}
	}
	// Hit a boundary on a non-landable row (e.g. a filtered-out edge); snap
	// back to the nearest row the cursor is allowed to rest on.
	inst.snapCursor()
}

// snapCursor moves the cursor to the nearest landable row, searching forward
// then backward. Used after the filter or collapse state changes the set of
// visible rows out from under the cursor.
func (inst *Model) snapCursor() {
	if inst.landable(inst.cursor) {
		return
	}
	for i := inst.cursor + 1; i < len(inst.items); i++ {
		if inst.landable(i) {
			inst.cursor = i
			return
		}
	}
	for i := inst.cursor - 1; i >= 0; i-- {
		if inst.landable(i) {
			inst.cursor = i
			return
		}
	}
}

func (inst *Model) isHidden(idx int) bool {
	if inst.items[idx].isCategory {
		return false
	}
	for i := idx - 1; i >= 0; i-- {
		if inst.items[i].isCategory {
			return inst.items[i].collapsed
		}
	}
	return false
}

func (inst *Model) toggleSelected() {
	if inst.cursor >= 0 && inst.cursor < len(inst.items) {
		it := inst.items[inst.cursor]
		if !it.isCategory && !it.unsupported {
			if inst.selected[it.component.Name] {
				delete(inst.selected, it.component.Name)
			} else {
				inst.selected[it.component.Name] = true
			}
		}
	}
}

func (inst *Model) toggleCategory() {
	if inst.cursor >= 0 && inst.cursor < len(inst.items) && inst.items[inst.cursor].isCategory {
		inst.items[inst.cursor].collapsed = !inst.items[inst.cursor].collapsed
	}
}

// onFilterChanged lands the cursor on the first matching component after the
// filter string changes, so the highlight is always on a real result rather
// than a category header or a filtered-out row.
func (inst *Model) onFilterChanged() {
	for i := range inst.items {
		if !inst.items[i].isCategory && inst.landable(i) {
			inst.cursor = i
			return
		}
	}
	// No component matches; fall back to the nearest landable row.
	inst.cursor = 0
	inst.snapCursor()
}

func (inst *Model) selectAll() {
	for _, it := range inst.items {
		if it.isCategory || it.unsupported {
			continue
		}
		if inst.mode == ModeUninstall {
			if it.installed {
				inst.selected[it.component.Name] = true
			}
		} else {
			if !it.installed {
				inst.selected[it.component.Name] = true
			}
		}
	}
}

func (inst *Model) selectNone() {
	inst.selected = make(map[string]bool)
}

func (inst *Model) updateFilter(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Keep the filter applied and return to navigation over the matches.
		inst.filtering = false
		inst.onFilterChanged()
		return inst, nil
	case "esc":
		// Cancel: clear the filter and restore the full list.
		inst.filtering = false
		inst.filter = ""
		inst.snapCursor()
		return inst, nil
	case "backspace":
		if len(inst.filter) > 0 {
			inst.filter = inst.filter[:len(inst.filter)-1]
			inst.onFilterChanged()
		}
	default:
		if len(msg.String()) == 1 {
			inst.filter += msg.String()
			inst.onFilterChanged()
		}
	}
	return inst, nil
}

func (inst *Model) matchesFilter(c component.Component) bool {
	if inst.filter == "" {
		return true
	}
	f := strings.ToLower(inst.filter)
	return strings.Contains(strings.ToLower(c.Name), f) ||
		strings.Contains(strings.ToLower(c.Description), f) ||
		matchTags(c.Tags, f)
}

func matchTags(tags []string, filter string) bool {
	for _, t := range tags {
		if strings.Contains(strings.ToLower(t), filter) {
			return true
		}
	}
	return false
}

// visibleIndices returns the indices of all currently-rendered rows in order.
func (inst *Model) visibleIndices() []int {
	var out []int
	for i := range inst.items {
		if inst.visible(i) {
			out = append(out, i)
		}
	}
	return out
}

// windowFor slices visible into the rows that fit on screen around the cursor,
// updating scrollOffset so the cursor stays in view. It returns the window plus
// the count of rows hidden above and below it (for the "N more" indicators).
func (inst *Model) windowFor(visible []int) (window []int, above, below int) {
	// Chrome = title + help + blank + (filter line + blank) + status bar.
	chrome := 4
	if inst.filtering {
		chrome = 6
	}
	avail := inst.height - chrome
	if inst.height <= 0 || avail >= len(visible) {
		// Size unknown or everything fits: render all, no scrolling.
		inst.scrollOffset = 0
		return visible, 0, 0
	}
	if avail < 1 {
		avail = 1
	}
	// Reserve a line top and bottom for the "N more" indicators.
	capacity := avail - 2
	if capacity < 1 {
		capacity = 1
	}

	cursorPos := 0
	for p, idx := range visible {
		if idx == inst.cursor {
			cursorPos = p
			break
		}
	}
	if cursorPos < inst.scrollOffset {
		inst.scrollOffset = cursorPos
	}
	if cursorPos >= inst.scrollOffset+capacity {
		inst.scrollOffset = cursorPos - capacity + 1
	}
	if max := len(visible) - capacity; inst.scrollOffset > max {
		inst.scrollOffset = max
	}
	if inst.scrollOffset < 0 {
		inst.scrollOffset = 0
	}

	end := inst.scrollOffset + capacity
	if end > len(visible) {
		end = len(visible)
	}
	return visible[inst.scrollOffset:end], inst.scrollOffset, len(visible) - end
}

func (inst *Model) View() tea.View {
	var b strings.Builder

	if inst.mode == ModeUninstall {
		b.WriteString(styles.Title.Render("Select components to uninstall"))
	} else {
		b.WriteString(styles.Title.Render("Select components to install"))
	}
	b.WriteString("\n")
	b.WriteString(styles.Help.Render("↑/↓ (j/k) move · Space toggle · a all · n none · Tab expand · / filter (Esc clear) · Enter confirm · q quit"))
	b.WriteString("\n\n")

	if inst.filtering {
		b.WriteString(fmt.Sprintf("Filter: %s█\n\n", inst.filter))
	}

	// Compute the scrolling window: which visible rows fit in the terminal.
	visible := inst.visibleIndices()
	window, above, below := inst.windowFor(visible)
	if above > 0 {
		b.WriteString(styles.Dimmed.Render(fmt.Sprintf("  ↑ %d more", above)) + "\n")
	}

	for _, i := range window {
		it := inst.items[i]

		isCursor := i == inst.cursor
		line := ""

		if it.isCategory {
			arrow := "▼"
			if it.collapsed {
				arrow = "▶"
			}
			count := inst.countInCategory(it.category)
			line = styles.CategoryHeader.Render(fmt.Sprintf("%s %s", arrow, string(it.category)))
			if it.collapsed {
				line += styles.Dimmed.Render(fmt.Sprintf(" (%d components)", count))
			}
		} else if it.unsupported {
			osLabel := ""
			for _, s := range it.component.SupportedOS {
				if s != component.OSAny {
					osLabel = string(s)
				}
			}
			if osLabel == "" {
				osLabel = "other"
			}
			indicator := styles.Dimmed.Render("⊘")
			name := lipgloss.NewStyle().Foreground(styles.Subtle).Width(16).Render(it.component.Name)
			desc := styles.Dimmed.Render(it.component.Description + " (" + osLabel + " only)")
			line = fmt.Sprintf("  %s  %s %s", indicator, name, desc)
		} else {
			indicator := styles.Unselected.String()
			if inst.selected[it.component.Name] {
				indicator = styles.Selected.String()
			} else if it.installed && inst.mode == ModeInstall {
				indicator = styles.Success.Render("●")
			}
			name := lipgloss.NewStyle().Foreground(styles.Bright).Width(16).Render(it.component.Name)
			desc := styles.Dimmed.Render(it.component.Description)
			line = fmt.Sprintf("  %s %s %s", indicator, name, desc)
		}

		if isCursor {
			line = styles.Cursor.Render(line)
		}
		b.WriteString(line + "\n")
	}

	if below > 0 {
		b.WriteString(styles.Dimmed.Render(fmt.Sprintf("  ↓ %d more", below)) + "\n")
	}

	selectedCount := len(inst.selected)
	installedCount := inst.countInstalled()
	status := fmt.Sprintf("%s · %d already installed",
		styles.Success.Render(fmt.Sprintf("%d selected", selectedCount)),
		installedCount,
	)
	b.WriteString(styles.StatusBar.Render(status))

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func (inst *Model) countInCategory(cat component.Category) int {
	count := 0
	for _, it := range inst.items {
		if !it.isCategory && it.component.Category == cat {
			count++
		}
	}
	return count
}

func (inst *Model) countInstalled() int {
	count := 0
	for _, it := range inst.items {
		if !it.isCategory && it.installed {
			count++
		}
	}
	return count
}

func (inst *Model) GetResult() Result {
	if inst.quitting && !inst.confirmed {
		return Result{Quit: true}
	}
	// Walk items in display order so the returned names are deterministic
	// and match the ordering the user sees in the UI.
	var names []string
	for _, it := range inst.items {
		if it.isCategory {
			continue
		}
		if inst.selected[it.component.Name] {
			names = append(names, it.component.Name)
		}
	}
	return Result{Selected: names}
}
