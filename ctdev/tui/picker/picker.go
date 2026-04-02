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
	ModeInstall   Mode = iota
	ModeUninstall
)

type Model struct {
	items     []item
	cursor    int
	selected  map[string]bool
	filtering bool
	filter    string
	filtered  []int
	quitting  bool
	confirmed bool
	width     int
	height    int
	platform  component.OS
	mode      Mode
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

func (inst *Model) moveCursor(dir int) {
	for {
		inst.cursor += dir
		if inst.cursor < 0 {
			inst.cursor = 0
			return
		}
		if inst.cursor >= len(inst.items) {
			inst.cursor = len(inst.items) - 1
			return
		}
		it := inst.items[inst.cursor]
		if !inst.isHidden(inst.cursor) && !it.unsupported {
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
	case "enter", "esc":
		inst.filtering = false
		return inst, nil
	case "backspace":
		if len(inst.filter) > 0 {
			inst.filter = inst.filter[:len(inst.filter)-1]
		}
	default:
		if len(msg.String()) == 1 {
			inst.filter += msg.String()
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

func (inst *Model) View() tea.View {
	var b strings.Builder

	if inst.mode == ModeUninstall {
		b.WriteString(styles.Title.Render("Select components to uninstall"))
	} else {
		b.WriteString(styles.Title.Render("Select components to install"))
	}
	b.WriteString("\n")
	b.WriteString(styles.Help.Render("Space toggle · a all · n none · Tab expand/collapse · / filter · Enter confirm · q quit"))
	b.WriteString("\n\n")

	if inst.filtering {
		b.WriteString(fmt.Sprintf("Filter: %s█\n\n", inst.filter))
	}

	for i, it := range inst.items {
		if inst.isHidden(i) {
			continue
		}
		if !it.isCategory && !inst.matchesFilter(it.component) {
			continue
		}

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
			name := lipgloss.NewStyle().Foreground(styles.Subtle).Width(14).Render(it.component.Name)
			desc := styles.Dimmed.Render(it.component.Description + " (" + osLabel + " only)")
			line = fmt.Sprintf("  %s  %s %s", indicator, name, desc)
		} else {
			indicator := styles.Unselected.String()
			if inst.selected[it.component.Name] {
				indicator = styles.Selected.String()
			} else if it.installed && inst.mode == ModeInstall {
				indicator = styles.Success.Render("●")
			}
			name := lipgloss.NewStyle().Foreground(styles.Bright).Width(14).Render(it.component.Name)
			desc := styles.Dimmed.Render(it.component.Description)
			line = fmt.Sprintf("  %s %s %s", indicator, name, desc)
		}

		if isCursor {
			line = styles.Cursor.Render(line)
		}
		b.WriteString(line + "\n")
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
	var names []string
	for name := range inst.selected {
		names = append(names, name)
	}
	return Result{Selected: names}
}
