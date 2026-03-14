package setup

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	s "github.com/ConnerTechnology/dotfiles/ctdev/setup"
)

func TestConfirmShowsDiff(t *testing.T) {
	states := []s.SettingState{
		{Setting: &s.Setting{Name: "GRUB timeout"}, CurrentValue: "5", DesiredValue: "10", Enabled: true},
		{Setting: &s.Setting{Name: "Key repeat"}, CurrentValue: "500", DesiredValue: "200", Enabled: true},
		{Setting: &s.Setting{Name: "Already good"}, CurrentValue: "on", DesiredValue: "on", Enabled: true},
	}
	m := NewConfirm(states, false)
	if len(m.changes) != 2 {
		t.Errorf("expected 2 changes, got %d", len(m.changes))
	}
}

func TestConfirmEnter(t *testing.T) {
	states := []s.SettingState{
		{Setting: &s.Setting{Name: "Test"}, CurrentValue: "off", DesiredValue: "on", Enabled: true},
	}
	m := NewConfirm(states, false)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.confirmed {
		t.Error("expected confirmed on enter")
	}
}

func TestConfirmEsc(t *testing.T) {
	states := []s.SettingState{
		{Setting: &s.Setting{Name: "Test"}, CurrentValue: "off", DesiredValue: "on", Enabled: true},
	}
	m := NewConfirm(states, false)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !m.cancelled {
		t.Error("expected cancelled on esc")
	}
}

func TestConfirmViewShowsChanges(t *testing.T) {
	states := []s.SettingState{
		{Setting: &s.Setting{Name: "GRUB timeout"}, CurrentValue: "5", DesiredValue: "10", Enabled: true},
	}
	m := NewConfirm(states, false)
	view := m.View(80, 24)
	if !strings.Contains(view, "Apply Changes?") {
		t.Error("expected Apply Changes? header")
	}
	if !strings.Contains(view, "GRUB timeout") {
		t.Error("expected change name in view")
	}
	if !strings.Contains(view, "1 setting will be applied") {
		t.Errorf("expected count text, got: %s", view)
	}
}

func TestConfirmDryRunView(t *testing.T) {
	states := []s.SettingState{
		{Setting: &s.Setting{Name: "Test"}, CurrentValue: "off", DesiredValue: "on", Enabled: true},
	}
	m := NewConfirm(states, true)
	view := m.View(80, 24)
	if !strings.Contains(view, "[dry-run]") {
		t.Error("expected dry-run header")
	}
	if strings.Contains(view, "Enter to confirm") {
		t.Error("dry-run should not show Enter prompt")
	}
}

func TestConfirmNoChanges(t *testing.T) {
	states := []s.SettingState{
		{Setting: &s.Setting{Name: "Same"}, CurrentValue: "on", DesiredValue: "on", Enabled: true},
	}
	m := NewConfirm(states, false)
	if len(m.changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(m.changes))
	}
	view := m.View(80, 24)
	if !strings.Contains(view, "No changes to apply") {
		t.Error("expected no changes message")
	}
}

func TestConfirmDisabledNotIncluded(t *testing.T) {
	states := []s.SettingState{
		{Setting: &s.Setting{Name: "Disabled"}, CurrentValue: "off", DesiredValue: "on", Enabled: false},
	}
	m := NewConfirm(states, false)
	if len(m.changes) != 0 {
		t.Errorf("expected 0 changes for disabled setting, got %d", len(m.changes))
	}
}
