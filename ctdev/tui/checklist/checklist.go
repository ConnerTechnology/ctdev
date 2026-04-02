package checklist

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
)

type UpdateItem struct {
	Name       string
	Source     string // "apt", "flatpak", "git", "runtime", "brew"
	CurrentVer string
	NewVer     string
	IsMajor    bool
	IsKernel   bool
}

type Model struct {
	items     []UpdateItem
	cursor    int
	selected  map[int]bool
	quitting  bool
	confirmed bool
	width     int
	height    int
}

type Result struct {
	Selected []UpdateItem
	Quit     bool
}

func New(items []UpdateItem) Model {
	sel := make(map[int]bool)
	for i := range items {
		sel[i] = true // all selected by default
	}
	return Model{
		items:    items,
		selected: sel,
	}
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
		switch msg.String() {
		case "q", "ctrl+c":
			inst.quitting = true
			return inst, tea.Quit
		case "enter":
			inst.confirmed = true
			return inst, tea.Quit
		case "up", "k":
			if inst.cursor > 0 {
				inst.cursor--
			}
		case "down", "j":
			if inst.cursor < len(inst.items)-1 {
				inst.cursor++
			}
		case "space":
			if inst.selected[inst.cursor] {
				delete(inst.selected, inst.cursor)
			} else {
				inst.selected[inst.cursor] = true
			}
		case "a":
			for i := range inst.items {
				inst.selected[i] = true
			}
		case "n":
			inst.selected = make(map[int]bool)
		}
	}
	return inst, nil
}

func (inst *Model) View() tea.View {
	var b strings.Builder

	b.WriteString(styles.Title.Render("Available Updates"))
	b.WriteString("\n")
	b.WriteString(styles.Help.Render("Space toggle · a all · n none · Enter install · q quit"))
	b.WriteString("\n\n")

	currentSource := ""
	for i, item := range inst.items {
		if item.Source != currentSource {
			currentSource = item.Source
			b.WriteString(styles.CategoryHeader.Render(sourceLabel(currentSource)))
			b.WriteString("\n")
		}

		indicator := styles.Unselected.String()
		if inst.selected[i] {
			indicator = styles.Selected.String()
		}

		version := styles.Dimmed.Render(fmt.Sprintf("%s → %s", item.CurrentVer, item.NewVer))
		line := fmt.Sprintf("  %s %-30s %s", indicator, item.Name, version)

		if item.IsMajor {
			line += " " + styles.Warning.Render("MAJOR")
		}
		if item.IsKernel {
			line += " " + styles.Warning.Render("KERNEL")
		}

		if i == inst.cursor {
			line = styles.Cursor.Render(line)
		}
		b.WriteString(line + "\n")
	}

	selectedCount := len(inst.selected)
	status := fmt.Sprintf("%d of %d selected", selectedCount, len(inst.items))
	b.WriteString(styles.StatusBar.Render(status))

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func (inst *Model) GetResult() Result {
	if inst.quitting && !inst.confirmed {
		return Result{Quit: true}
	}
	var selected []UpdateItem
	for i, item := range inst.items {
		if inst.selected[i] {
			selected = append(selected, item)
		}
	}
	return Result{Selected: selected}
}

func sourceLabel(source string) string {
	switch source {
	case "apt":
		return "System Packages (apt)"
	case "brew":
		return "System Packages (brew)"
	case "flatpak":
		return "Flatpak"
	case "git":
		return "Git Repositories"
	case "runtime":
		return "Runtimes"
	case "npm":
		return "NPM Global Packages"
	case "ctdev":
		return "ctdev"
	case "cli":
		return "CLI Tools"
	default:
		return source
	}
}
