package setup

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	s "github.com/ConnerTechnology/dotfiles/ctdev/setup"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
)

// ModalModel handles the info modal overlay.
type ModalModel struct {
	state     *s.SettingState
	mode      Mode
	closed    bool
	pickerIdx int
}

func NewModal(state *s.SettingState, mode Mode) ModalModel {
	m := ModalModel{state: state, mode: mode}
	if state.Setting.Control == s.ControlPicker {
		for i, c := range state.Setting.Choices {
			if c.Value == state.DesiredValue {
				m.pickerIdx = i
				break
			}
		}
	}
	return m
}

func (inst *ModalModel) Closed() bool { return inst.closed }

func (inst *ModalModel) Update(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "esc":
		inst.closed = true
		return
	}

	if inst.mode != ModeInteractive {
		return
	}

	switch msg.String() {
	case "d":
		inst.state.DesiredValue = inst.state.Setting.Default
		// Sync picker index if applicable
		if inst.state.Setting.Control == s.ControlPicker {
			for i, c := range inst.state.Setting.Choices {
				if c.Value == inst.state.DesiredValue {
					inst.pickerIdx = i
					break
				}
			}
		}
		return
	}

	switch inst.state.Setting.Control {
	case s.ControlSlider:
		inst.updateSlider(msg)
	case s.ControlPicker:
		inst.updatePicker(msg)
	case s.ControlToggle:
		inst.updateToggle(msg)
	}
}

func (inst *ModalModel) updateSlider(msg tea.KeyPressMsg) {
	if inst.state.Setting.Slider == nil {
		return
	}
	sr := inst.state.Setting.Slider
	val, err := strconv.ParseFloat(inst.state.DesiredValue, 64)
	if err != nil {
		return
	}

	switch msg.String() {
	case "left":
		val -= sr.Step
		if val < sr.Min {
			val = sr.Min
		}
	case "right":
		val += sr.Step
		if val > sr.Max {
			val = sr.Max
		}
	default:
		return
	}

	inst.state.DesiredValue = formatSliderValue(val, sr.Step)
}

func (inst *ModalModel) updatePicker(msg tea.KeyPressMsg) {
	choices := inst.state.Setting.Choices
	if len(choices) == 0 {
		return
	}

	switch msg.String() {
	case "up":
		if inst.pickerIdx > 0 {
			inst.pickerIdx--
		}
	case "down":
		if inst.pickerIdx < len(choices)-1 {
			inst.pickerIdx++
		}
	default:
		return
	}

	inst.state.DesiredValue = choices[inst.pickerIdx].Value
}

func (inst *ModalModel) updateToggle(msg tea.KeyPressMsg) {
	if msg.String() == "space" {
		// Toggle the desired value between common toggle pairs
		switch inst.state.DesiredValue {
		case "true":
			inst.state.DesiredValue = "false"
		case "false":
			inst.state.DesiredValue = "true"
		case "enabled":
			inst.state.DesiredValue = "disabled"
		case "disabled":
			inst.state.DesiredValue = "enabled"
		case "signed":
			inst.state.DesiredValue = "unsigned"
		case "unsigned":
			inst.state.DesiredValue = "signed"
		case "installed":
			inst.state.DesiredValue = "not installed"
		case "not installed":
			inst.state.DesiredValue = "installed"
		case "active":
			inst.state.DesiredValue = "inactive"
		case "inactive":
			inst.state.DesiredValue = "active"
		default:
			// Fall back to toggling the enabled flag
			inst.state.Enabled = !inst.state.Enabled
		}
	}
}

func (inst *ModalModel) View(width, height int) string {
	modalWidth := 50
	if width > 0 && width-4 < modalWidth {
		modalWidth = width - 4
	}
	if modalWidth < 30 {
		modalWidth = 30
	}
	contentWidth := modalWidth - 4 // padding inside border

	var b strings.Builder

	// Title
	b.WriteString(styles.Title.Render(inst.state.Setting.Name))
	b.WriteString("\n")

	// Current value
	unit := ""
	if inst.state.Setting.Slider != nil {
		unit = inst.state.Setting.Slider.Unit
	}
	b.WriteString(fmt.Sprintf("Current: %s%s", inst.state.CurrentValue, unit))
	b.WriteString("\n")

	if inst.mode == ModeReadonly {
		b.WriteString(fmt.Sprintf("Default: %s%s", inst.state.Setting.Default, unit))
		b.WriteString("\n")
	}

	// Description
	if inst.state.Setting.Description != "" {
		b.WriteString("\n")
		b.WriteString(styles.Dimmed.Render(inst.state.Setting.Description))
		b.WriteString("\n")
	}

	// Controls (interactive only)
	if inst.mode == ModeInteractive {
		b.WriteString("\n")
		switch inst.state.Setting.Control {
		case s.ControlSlider:
			b.WriteString(inst.renderSliderControl(contentWidth))
		case s.ControlPicker:
			b.WriteString(inst.renderPickerControl())
		case s.ControlToggle:
			b.WriteString(fmt.Sprintf("Value: %s\n", inst.state.DesiredValue))
			b.WriteString(styles.Help.Render("Space to toggle"))
			b.WriteString("\n")
		}
	}

	// Tech detail
	if inst.state.Setting.TechDetail != "" {
		b.WriteString("\n")
		b.WriteString(styles.Dimmed.Render(inst.state.Setting.TechDetail))
		b.WriteString("\n")
	}

	// Help line
	b.WriteString("\n")
	helpParts := []string{"Esc close"}
	if inst.mode == ModeInteractive {
		switch inst.state.Setting.Control {
		case s.ControlSlider:
			helpParts = append(helpParts, "← → adjust")
		case s.ControlPicker:
			helpParts = append(helpParts, "↑ ↓ select")
		case s.ControlToggle:
			helpParts = append(helpParts, "Space toggle")
		}
		helpParts = append(helpParts, "d default")
	}
	b.WriteString(styles.Help.Render(strings.Join(helpParts, " · ")))

	content := b.String()

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Blue).
		Padding(1, 2).
		Width(modalWidth)

	return box.Render(content)
}

func (inst *ModalModel) renderSliderControl(width int) string {
	sr := inst.state.Setting.Slider
	if sr == nil {
		return ""
	}
	val, _ := strconv.ParseFloat(inst.state.DesiredValue, 64)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Value: %s%s\n", inst.state.DesiredValue, sr.Unit))

	barWidth := width - 4
	if barWidth < 10 {
		barWidth = 10
	}
	b.WriteString(renderSliderBar(val, sr.Min, sr.Max, barWidth))
	b.WriteString("\n")

	minStr := formatSliderValue(sr.Min, sr.Step)
	maxStr := formatSliderValue(sr.Max, sr.Step)
	b.WriteString(styles.Dimmed.Render(fmt.Sprintf("%s%s — %s%s", minStr, sr.Unit, maxStr, sr.Unit)))
	b.WriteString("\n")

	return b.String()
}

func (inst *ModalModel) renderPickerControl() string {
	var b strings.Builder
	for i, c := range inst.state.Setting.Choices {
		indicator := "○"
		if i == inst.pickerIdx {
			indicator = "◉"
		}
		line := fmt.Sprintf("  %s %s", indicator, c.Value)
		if c.Description != "" {
			line += "  " + styles.Dimmed.Render(c.Description)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func renderSliderBar(value, min, max float64, width int) string {
	if width < 3 {
		width = 3
	}
	// Track width excludes the end markers
	trackWidth := width - 2
	if trackWidth < 1 {
		trackWidth = 1
	}

	ratio := (value - min) / (max - min)
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	pos := int(math.Round(ratio * float64(trackWidth-1)))

	var b strings.Builder
	b.WriteRune('◂')
	for i := 0; i < trackWidth; i++ {
		if i == pos {
			b.WriteRune('●')
		} else {
			b.WriteRune('━')
		}
	}
	b.WriteRune('▸')
	return b.String()
}

func formatSliderValue(val, step float64) string {
	if step >= 1 && val == math.Trunc(val) {
		return strconv.Itoa(int(val))
	}
	// Determine precision from step
	s := strconv.FormatFloat(step, 'f', -1, 64)
	precision := 0
	if idx := strings.IndexByte(s, '.'); idx >= 0 {
		precision = len(s) - idx - 1
	}
	return strconv.FormatFloat(val, 'f', precision, 64)
}
