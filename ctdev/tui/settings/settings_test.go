package settings

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/setup"
)

func testStates() []setup.SettingState {
	settings := []setup.Setting{
		{Name: "Toggle A", Category: "Cat1", Control: setup.ControlToggle, Default: "enabled"},
		{Name: "Picker B", Category: "Cat1", Control: setup.ControlPicker, Default: "x",
			Choices: []setup.PickerChoice{{Value: "x"}, {Value: "y"}, {Value: "z"}}},
		{Name: "Slider C", Category: "Cat2", Control: setup.ControlSlider, Default: "20",
			Slider: &setup.SliderRange{Min: 0, Max: 30, Step: 10}},
		{Name: "OneWay D", Category: "Cat2", Control: setup.ControlToggle, Default: "installed", OneWay: true},
	}
	states := make([]setup.SettingState, len(settings))
	current := []string{"disabled", "y", "10", "not installed"}
	for i := range settings {
		states[i] = setup.SettingState{Setting: &settings[i], CurrentValue: current[i], DesiredValue: current[i]}
	}
	return states
}

func press(m *Model, key string) {
	m.Update(tea.KeyPressMsg{Code: keyCode(key), Text: keyText(key)})
}

// The bubbletea v2 KeyPressMsg for plain runes carries the rune; for named
// keys the Code. Build the minimal message whose String() matches.
func keyCode(k string) rune {
	switch k {
	case "enter":
		return tea.KeyEnter
	case "down":
		return tea.KeyDown
	case "right":
		return tea.KeyRight
	default:
		return []rune(k)[0]
	}
}

func keyText(k string) string {
	if len([]rune(k)) == 1 {
		return k
	}
	return ""
}

func TestCycleQueuesAndUnqueues(t *testing.T) {
	m := New(testStates())

	// Cursor starts on Toggle A ("disabled", recommended "enabled").
	press(m, "enter")
	if m.PendingCount() != 1 {
		t.Fatalf("after cycle: pending = %d, want 1", m.PendingCount())
	}
	if v := value(m.current()); v != "enabled" {
		t.Errorf("toggle cycled to %q, want enabled", v)
	}
	// Cycling again returns to the detected value → nothing pending.
	press(m, "enter")
	if m.PendingCount() != 0 {
		t.Errorf("after cycle back: pending = %d, want 0", m.PendingCount())
	}
}

func TestSliderAndRecommended(t *testing.T) {
	m := New(testStates())
	press(m, "down") // Picker B
	press(m, "down") // Slider C (10)
	press(m, "right")
	if v := value(m.current()); v != "20" {
		t.Errorf("slider stepped to %q, want 20", v)
	}
	press(m, "u") // revert
	if m.PendingCount() != 0 {
		t.Errorf("after revert: pending = %d, want 0", m.PendingCount())
	}
	press(m, "r") // recommended = 20
	if v := value(m.current()); v != "20" || m.PendingCount() != 1 {
		t.Errorf("recommended: value %q pending %d, want 20/1", v, m.PendingCount())
	}
}

func TestOneWayQueuesRecommendedOnly(t *testing.T) {
	m := New(testStates())
	m.cursor = m.lastSetting() // OneWay D ("not installed")
	press(m, "enter")
	st := m.current()
	if !st.Enabled || st.DesiredValue != "installed" {
		t.Errorf("one-way cycle should queue the recommended apply, got %+v", st)
	}
	press(m, "enter")
	if st.Enabled {
		t.Error("second cycle should unqueue the one-way apply")
	}
}

func TestApplyAndQuitFlags(t *testing.T) {
	m := New(testStates())
	press(m, "a")
	if !m.Applied() {
		t.Error("a should mark applied")
	}
	m2 := New(testStates())
	press(m2, "q")
	if m2.Applied() {
		t.Error("q should not mark applied")
	}
}
