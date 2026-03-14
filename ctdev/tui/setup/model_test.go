package setup

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	s "github.com/ConnerTechnology/dotfiles/ctdev/setup"
)

func testStates() []s.SettingState {
	return []s.SettingState{
		{Setting: &s.Setting{Name: "Test Toggle", Category: "Cat A", Control: s.ControlToggle, Default: "on"}, CurrentValue: "off", DesiredValue: "on", Enabled: true},
		{Setting: &s.Setting{Name: "Test Slider", Category: "Cat A", Control: s.ControlSlider, Default: "200", Slider: &s.SliderRange{Min: 100, Max: 1000, Step: 25, Unit: "ms"}}, CurrentValue: "500", DesiredValue: "200", Enabled: true},
		{Setting: &s.Setting{Name: "Other Setting", Category: "Cat B", Control: s.ControlToggle, Default: "on"}, CurrentValue: "on", DesiredValue: "on", Enabled: true},
	}
}

func TestNavigateDown(t *testing.T) {
	m := New(testStates(), ModeInteractive)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model := updated.(*Model)
	if model.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", model.cursor)
	}
}

func TestNavigateUpAtTop(t *testing.T) {
	m := New(testStates(), ModeInteractive)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	model := updated.(*Model)
	if model.cursor != 0 {
		t.Errorf("expected cursor 0, got %d", model.cursor)
	}
}

func TestToggleSpace(t *testing.T) {
	m := New(testStates(), ModeInteractive)
	updated, _ := m.Update(tea.KeyPressMsg{Code: ' '})
	model := updated.(*Model)
	if model.states[0].Enabled {
		t.Error("expected first item disabled after space")
	}
}

func TestToggleDisabledInReadonly(t *testing.T) {
	m := New(testStates(), ModeReadonly)
	updated, _ := m.Update(tea.KeyPressMsg{Code: ' '})
	model := updated.(*Model)
	if !model.states[0].Enabled {
		t.Error("space should not toggle in readonly mode")
	}
}

func TestQuitOnQ(t *testing.T) {
	m := New(testStates(), ModeInteractive)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q'})
	if cmd == nil {
		t.Error("expected quit command")
	}
}
