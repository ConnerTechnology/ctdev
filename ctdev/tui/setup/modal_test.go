package setup

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	s "github.com/ConnerTechnology/dotfiles/ctdev/setup"
)

func TestSliderLeft(t *testing.T) {
	state := &s.SettingState{
		Setting:      &s.Setting{Name: "Delay", Control: s.ControlSlider, Default: "200", Slider: &s.SliderRange{Min: 100, Max: 1000, Step: 25, Unit: "ms"}},
		DesiredValue: "200",
		Enabled:      true,
	}
	m := NewModal(state, ModeInteractive)
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if state.DesiredValue != "175" {
		t.Errorf("expected 175, got %s", state.DesiredValue)
	}
}

func TestSliderClampMin(t *testing.T) {
	state := &s.SettingState{
		Setting:      &s.Setting{Name: "Delay", Control: s.ControlSlider, Default: "200", Slider: &s.SliderRange{Min: 100, Max: 1000, Step: 25, Unit: "ms"}},
		DesiredValue: "100",
		Enabled:      true,
	}
	m := NewModal(state, ModeInteractive)
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if state.DesiredValue != "100" {
		t.Errorf("expected 100 (clamped), got %s", state.DesiredValue)
	}
}

func TestSliderRight(t *testing.T) {
	state := &s.SettingState{
		Setting:      &s.Setting{Name: "Delay", Control: s.ControlSlider, Default: "200", Slider: &s.SliderRange{Min: 100, Max: 1000, Step: 25, Unit: "ms"}},
		DesiredValue: "200",
		Enabled:      true,
	}
	m := NewModal(state, ModeInteractive)
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if state.DesiredValue != "225" {
		t.Errorf("expected 225, got %s", state.DesiredValue)
	}
}

func TestSliderClampMax(t *testing.T) {
	state := &s.SettingState{
		Setting:      &s.Setting{Name: "Delay", Control: s.ControlSlider, Default: "200", Slider: &s.SliderRange{Min: 100, Max: 1000, Step: 25, Unit: "ms"}},
		DesiredValue: "1000",
		Enabled:      true,
	}
	m := NewModal(state, ModeInteractive)
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if state.DesiredValue != "1000" {
		t.Errorf("expected 1000 (clamped), got %s", state.DesiredValue)
	}
}

func TestPickerDown(t *testing.T) {
	state := &s.SettingState{
		Setting: &s.Setting{
			Name: "Profile", Control: s.ControlPicker, Default: "performance",
			Choices: []s.PickerChoice{{Value: "performance"}, {Value: "balanced"}, {Value: "power-saver"}},
		},
		DesiredValue: "performance",
		Enabled:      true,
	}
	m := NewModal(state, ModeInteractive)
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if state.DesiredValue != "balanced" {
		t.Errorf("expected balanced, got %s", state.DesiredValue)
	}
}

func TestPickerUp(t *testing.T) {
	state := &s.SettingState{
		Setting: &s.Setting{
			Name: "Profile", Control: s.ControlPicker, Default: "performance",
			Choices: []s.PickerChoice{{Value: "performance"}, {Value: "balanced"}, {Value: "power-saver"}},
		},
		DesiredValue: "balanced",
		Enabled:      true,
	}
	m := NewModal(state, ModeInteractive)
	if m.pickerIdx != 1 {
		t.Fatalf("expected initial pickerIdx 1, got %d", m.pickerIdx)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if state.DesiredValue != "performance" {
		t.Errorf("expected performance, got %s", state.DesiredValue)
	}
}

func TestPickerClampTop(t *testing.T) {
	state := &s.SettingState{
		Setting: &s.Setting{
			Name: "Profile", Control: s.ControlPicker, Default: "performance",
			Choices: []s.PickerChoice{{Value: "performance"}, {Value: "balanced"}},
		},
		DesiredValue: "performance",
		Enabled:      true,
	}
	m := NewModal(state, ModeInteractive)
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if state.DesiredValue != "performance" {
		t.Errorf("expected performance (clamped), got %s", state.DesiredValue)
	}
}

func TestDefaultReset(t *testing.T) {
	state := &s.SettingState{
		Setting:      &s.Setting{Name: "Delay", Control: s.ControlSlider, Default: "200", Slider: &s.SliderRange{Min: 100, Max: 1000, Step: 25, Unit: "ms"}},
		DesiredValue: "500",
		Enabled:      true,
	}
	m := NewModal(state, ModeInteractive)
	m.Update(tea.KeyPressMsg{Code: 'd'})
	if state.DesiredValue != "200" {
		t.Errorf("expected 200 (default), got %s", state.DesiredValue)
	}
}

func TestReadonlyNoAdjust(t *testing.T) {
	state := &s.SettingState{
		Setting:      &s.Setting{Name: "Delay", Control: s.ControlSlider, Default: "200", Slider: &s.SliderRange{Min: 100, Max: 1000, Step: 25, Unit: "ms"}},
		DesiredValue: "200",
		Enabled:      true,
	}
	m := NewModal(state, ModeReadonly)
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if state.DesiredValue != "200" {
		t.Errorf("readonly should not change value, got %s", state.DesiredValue)
	}
}

func TestModalEscCloses(t *testing.T) {
	state := &s.SettingState{
		Setting:      &s.Setting{Name: "Test", Control: s.ControlToggle, Default: "on"},
		DesiredValue: "on",
		Enabled:      true,
	}
	m := NewModal(state, ModeInteractive)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !m.Closed() {
		t.Error("expected modal closed on esc")
	}
}

func TestModalToggleSpace(t *testing.T) {
	state := &s.SettingState{
		Setting:      &s.Setting{Name: "Feature", Control: s.ControlToggle, Default: "on"},
		DesiredValue: "on",
		Enabled:      true,
	}
	m := NewModal(state, ModeInteractive)
	m.Update(tea.KeyPressMsg{Code: ' '})
	if state.Enabled {
		t.Error("expected Enabled toggled to false")
	}
}

func TestFormatSliderValueInteger(t *testing.T) {
	got := formatSliderValue(500, 25)
	if got != "500" {
		t.Errorf("expected 500, got %s", got)
	}
}

func TestFormatSliderValueDecimal(t *testing.T) {
	got := formatSliderValue(1.5, 0.5)
	if got != "1.5" {
		t.Errorf("expected 1.5, got %s", got)
	}
}
