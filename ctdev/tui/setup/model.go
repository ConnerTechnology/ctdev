package setup

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	s "github.com/ConnerTechnology/dotfiles/ctdev/setup"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
)

type Mode int

const (
	ModeInteractive Mode = iota
	ModeReadonly
)

type viewState int

const (
	viewList viewState = iota
	viewModal
	viewConfirm
)

type Model struct {
	states   []s.SettingState
	mode     Mode
	cursor   int
	view     viewState
	modal    *ModalModel
	confirm  *ConfirmModel
	offset   int // scroll offset for viewport
	width    int
	height   int
	quitting bool
	applied  bool
}

func New(states []s.SettingState, mode Mode) Model {
	return Model{
		states: states,
		mode:   mode,
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
		switch inst.view {
		case viewModal:
			return inst.updateModal(msg)
		case viewConfirm:
			return inst.updateConfirm(msg)
		default:
			return inst.updateList(msg)
		}
	}
	return inst, nil
}

func (inst *Model) updateModal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Delegate to modal; for now just close on any key since modal is a stub
	switch msg.String() {
	case "esc", "q", "i", "enter":
		inst.view = viewList
		inst.modal = nil
	}
	return inst, nil
}

func (inst *Model) updateConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		inst.confirm.cancelled = true
		inst.view = viewList
		inst.confirm = nil
	case "enter", "y":
		inst.confirm.confirmed = true
		inst.applied = true
		return inst, tea.Quit
	}
	return inst, nil
}

func (inst *Model) updateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		inst.quitting = true
		return inst, tea.Quit
	case "up", "k":
		if inst.cursor > 0 {
			inst.cursor--
		}
	case "down", "j":
		if inst.cursor < len(inst.states)-1 {
			inst.cursor++
		}
	case "space":
		if inst.mode == ModeInteractive && inst.cursor >= 0 && inst.cursor < len(inst.states) {
			inst.states[inst.cursor].Enabled = !inst.states[inst.cursor].Enabled
		}
	case "i":
		if inst.cursor >= 0 && inst.cursor < len(inst.states) {
			modal := NewModal(&inst.states[inst.cursor], inst.mode)
			inst.modal = &modal
			inst.view = viewModal
		}
	case "enter":
		if inst.mode == ModeInteractive {
			confirm := NewConfirm(inst.states, false)
			inst.confirm = &confirm
			inst.view = viewConfirm
		}
	}
	return inst, nil
}

func (inst *Model) View() tea.View {
	if inst.view == viewConfirm && inst.confirm != nil {
		return inst.viewConfirm()
	}

	var b strings.Builder
	mainContent := inst.buildList()

	if inst.view == viewModal && inst.modal != nil {
		// Dim the main list and overlay modal centered
		dimmed := styles.Dimmed.Render(mainContent)
		b.WriteString(dimmed)
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("  [Info: %s] (press esc to close)\n", inst.modal.state.Setting.Name))
	} else {
		b.WriteString(mainContent)
	}

	return tea.NewView(b.String())
}

func (inst *Model) viewConfirm() tea.View {
	var b strings.Builder
	b.WriteString(styles.Title.Render("Confirm Changes"))
	b.WriteString("\n\n")

	if inst.confirm != nil && len(inst.confirm.changes) > 0 {
		for _, c := range inst.confirm.changes {
			b.WriteString(fmt.Sprintf("  %s: %s -> %s\n",
				lipgloss.NewStyle().Foreground(styles.Bright).Render(c.name),
				styles.Warning.Render(c.from),
				styles.Success.Render(c.to),
			))
		}
	} else {
		b.WriteString("  No changes to apply.\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Help.Render("Enter/y apply  esc/q cancel"))
	return tea.NewView(b.String())
}

func (inst *Model) buildList() string {
	var b strings.Builder

	if inst.mode == ModeInteractive {
		b.WriteString(styles.Title.Render("System Setup"))
	} else {
		b.WriteString(styles.Title.Render("System Configuration"))
	}
	b.WriteString("\n")

	if inst.mode == ModeInteractive {
		b.WriteString(styles.Help.Render("j/k navigate  space toggle  i info  enter confirm  q quit"))
	} else {
		b.WriteString(styles.Help.Render("j/k navigate  i info  q quit"))
	}
	b.WriteString("\n\n")

	// Build all lines grouped by category
	var lines []string
	categories := inst.categories()
	stateIdx := 0

	for _, cat := range categories {
		lines = append(lines, styles.CategoryHeader.Render(cat))

		for i := range inst.states {
			if inst.states[i].Setting.Category != cat {
				continue
			}

			line := inst.renderRow(i, stateIdx)
			lines = append(lines, line)
			stateIdx++
		}
		lines = append(lines, "") // blank line between categories
	}

	// Viewport scrolling
	viewportHeight := inst.height - 6 // account for title, help, status
	if viewportHeight < 5 {
		viewportHeight = 20
	}

	// Find cursor's line position
	cursorLine := inst.findCursorLine(lines)

	// Adjust offset to keep cursor visible with 3 lines context
	if cursorLine < inst.offset+3 {
		inst.offset = cursorLine - 3
	}
	if cursorLine >= inst.offset+viewportHeight-3 {
		inst.offset = cursorLine - viewportHeight + 4
	}
	if inst.offset < 0 {
		inst.offset = 0
	}
	if inst.offset > len(lines)-viewportHeight {
		maxOffset := len(lines) - viewportHeight
		if maxOffset < 0 {
			maxOffset = 0
		}
		inst.offset = maxOffset
	}

	end := inst.offset + viewportHeight
	if end > len(lines) {
		end = len(lines)
	}

	for _, line := range lines[inst.offset:end] {
		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

func (inst *Model) renderRow(stateIdx int, _ int) string {
	st := inst.states[stateIdx]
	isCursor := stateIdx == inst.cursor

	var line string
	name := lipgloss.NewStyle().Foreground(styles.Bright).Width(32).Render(st.Setting.Name)

	// Color value green if matches default, yellow otherwise
	valueStyle := styles.Success
	if st.DesiredValue != st.Setting.Default {
		valueStyle = styles.Warning
	}
	value := valueStyle.Render(st.DesiredValue)

	if inst.mode == ModeInteractive {
		indicator := styles.Unselected.String() // "○"
		if st.Enabled {
			indicator = styles.Selected.String() // "◉"
		}
		line = fmt.Sprintf("  %s %s %s", indicator, name, value)
	} else {
		line = fmt.Sprintf("  %s %s", name, value)
	}

	if isCursor {
		line = styles.Cursor.Render(line)
	}

	return line
}

func (inst *Model) categories() []string {
	seen := make(map[string]bool)
	var cats []string
	for _, st := range inst.states {
		if !seen[st.Setting.Category] {
			seen[st.Setting.Category] = true
			cats = append(cats, st.Setting.Category)
		}
	}
	return cats
}

func (inst *Model) findCursorLine(lines []string) int {
	// Map cursor (state index) to line index
	lineIdx := 0
	stateIdx := 0
	categories := inst.categories()
	catIdx := 0
	currentCat := ""
	if len(categories) > 0 {
		currentCat = categories[0]
	}

	for _, st := range inst.states {
		if st.Setting.Category != currentCat {
			// We moved to a new category; find its header line
			for catIdx < len(categories) && categories[catIdx] != st.Setting.Category {
				catIdx++
			}
			currentCat = st.Setting.Category
		}

		if stateIdx == inst.cursor {
			// Search for this state's line in the lines slice
			for li, l := range lines {
				if strings.Contains(l, st.Setting.Name) {
					return li
				}
			}
			return lineIdx
		}
		stateIdx++
		lineIdx++
	}
	return 0
}

// Applied returns whether the user confirmed and applied changes.
func (inst *Model) Applied() bool { return inst.applied }

// States returns the current setting states.
func (inst *Model) States() []s.SettingState { return inst.states }
