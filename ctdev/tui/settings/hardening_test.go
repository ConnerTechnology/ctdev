package settings

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/setup"
)

func TestEmptyModel_NoPanicOnKeys(t *testing.T) {
	m := New(nil)
	for _, msg := range []tea.KeyPressMsg{
		{Code: tea.KeySpace},
		{Code: tea.KeyLeft},
		{Code: tea.KeyRight},
		{Code: 'r', Text: "r"},
		{Code: 'u', Text: "u"},
	} {
		m.Update(msg)
	}
	if v := m.View(); v.Content == "" {
		t.Error("expected non-empty view for empty model")
	}
}

func TestFilterMode_CtrlCQuitsAndFilterRenders(t *testing.T) {
	states := []setup.SettingState{
		{Setting: &setup.Setting{Name: "Power profile", Category: "Power", Control: setup.ControlToggle, Default: "balanced"}, CurrentValue: "balanced"},
		{Setting: &setup.Setting{Name: "Mouse speed", Category: "Mouse", Control: setup.ControlToggle, Default: "fast"}, CurrentValue: "slow"},
	}
	m := New(states)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})

	// Filter to "mouse" and verify View renders only the matching setting.
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, r := range "mouse" {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	view := m.View().Content
	if !strings.Contains(view, "Mouse speed") || strings.Contains(view, "Power profile") {
		t.Errorf("filtered view unexpected:\n%s", view)
	}

	// ctrl+c during filter mode quits without applying.
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd on ctrl+c in filter mode")
	}
	if m.Applied() {
		t.Error("ctrl+c in filter mode must not mark Applied=true")
	}
}
