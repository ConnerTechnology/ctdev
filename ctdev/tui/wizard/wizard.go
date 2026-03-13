package wizard

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
)

type Option struct {
	Label       string
	Description string
	Enabled     bool
	AlreadyDone bool
	BashScript  string
}

type Step struct {
	Title       string
	Description string
	Options     []Option
}

type Model struct {
	steps       []Step
	currentStep int
	cursor      int
	completed   map[int]bool
	quitting    bool
	confirmed   bool
	width       int
	height      int
}

type Result struct {
	Steps []Step
	Quit  bool
}

func New(steps []Step) Model {
	return Model{
		steps:     steps,
		completed: make(map[int]bool),
	}
}

func (inst Model) Init() tea.Cmd {
	return nil
}

func (inst Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			inst.completed[inst.currentStep] = true
			if inst.currentStep < len(inst.steps)-1 {
				inst.currentStep++
				inst.cursor = 0
			} else {
				inst.confirmed = true
				return inst, tea.Quit
			}
		case "esc":
			if inst.currentStep > 0 {
				inst.currentStep--
				inst.cursor = 0
			}
		case "s":
			if inst.currentStep < len(inst.steps)-1 {
				inst.currentStep++
				inst.cursor = 0
			}
		case "up", "k":
			if inst.cursor > 0 {
				inst.cursor--
			}
		case "down", "j":
			step := inst.steps[inst.currentStep]
			if inst.cursor < len(step.Options)-1 {
				inst.cursor++
			}
		case " ":
			step := &inst.steps[inst.currentStep]
			if inst.cursor < len(step.Options) && !step.Options[inst.cursor].AlreadyDone {
				step.Options[inst.cursor].Enabled = !step.Options[inst.cursor].Enabled
			}
		}
	}
	return inst, nil
}

func (inst Model) View() tea.View {
	var b strings.Builder

	sidebarWidth := 28

	// Sidebar
	var sidebar strings.Builder
	sidebar.WriteString(styles.Title.Render("Setup Steps"))
	sidebar.WriteString("\n\n")
	for i, step := range inst.steps {
		icon := "○"
		style := styles.Dimmed
		if inst.completed[i] {
			icon = "✓"
			style = lipgloss.NewStyle().Foreground(styles.Green)
		} else if i == inst.currentStep {
			icon = "●"
			style = lipgloss.NewStyle().Foreground(styles.Blue).Bold(true)
		}
		sidebar.WriteString(fmt.Sprintf("  %s %s\n", style.Render(icon), style.Render(step.Title)))
	}
	sidebar.WriteString(fmt.Sprintf("\n  %s", styles.Dimmed.Render(fmt.Sprintf("Step %d of %d", inst.currentStep+1, len(inst.steps)))))

	// Main panel
	var main strings.Builder
	step := inst.steps[inst.currentStep]
	main.WriteString(lipgloss.NewStyle().Bold(true).Foreground(styles.Bright).Render(step.Title))
	main.WriteString("\n")
	main.WriteString(styles.Dimmed.Render(step.Description))
	main.WriteString("\n\n")

	for i, opt := range step.Options {
		indicator := styles.Unselected.String()
		if opt.Enabled {
			indicator = styles.Selected.String()
		}
		label := opt.Label
		if opt.AlreadyDone {
			indicator = styles.Dimmed.Render("○")
			label = styles.Dimmed.Render(opt.Label) + " " + styles.Success.Render("already active")
		}

		line := fmt.Sprintf("  %s %s", indicator, label)
		if i == inst.cursor {
			line = styles.Cursor.Render(line)
		}
		main.WriteString(line + "\n")
	}

	main.WriteString("\n")
	main.WriteString(styles.Help.Render("Space toggle · Enter next · Esc back · s skip · q quit"))

	// Layout side by side
	sidebarBox := lipgloss.NewStyle().
		Width(sidebarWidth).
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#30363d")).
		PaddingRight(2).
		Render(sidebar.String())

	mainBox := lipgloss.NewStyle().
		PaddingLeft(2).
		Render(main.String())

	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, sidebarBox, mainBox))

	return tea.NewView(b.String())
}

func (inst Model) GetResult() Result {
	if inst.quitting && !inst.confirmed {
		return Result{Quit: true}
	}
	return Result{Steps: inst.steps}
}
