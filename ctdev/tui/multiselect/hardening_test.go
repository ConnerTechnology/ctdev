package multiselect

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestEmptyModel_NoPanicOnKeys(t *testing.T) {
	m := New(nil, Options{Title: "Empty"})
	for _, msg := range []tea.KeyPressMsg{
		{Code: tea.KeySpace},
		{Code: tea.KeyTab},
		{Code: tea.KeyDown},
		{Code: tea.KeyUp},
		{Code: tea.KeyEnter},
	} {
		m.Update(msg)
	}
	if len(m.Result().Selected) != 0 {
		t.Errorf("expected empty selection, got %v", m.Result().Selected)
	}
}

func TestZeroMatchFilter_SpaceDoesNotToggleHiddenGroup(t *testing.T) {
	groups := []Group{{
		Key:   "g1",
		Title: "Group 1",
		Items: []Item{
			{ID: "a", Primary: "alpha", Selectable: true, Bulk: true},
			{ID: "b", Primary: "beta", Selectable: true, Bulk: true},
		},
	}}
	m := New(groups, Options{Title: "Test"})

	// Enter filter mode, type a query matching nothing, press Enter to exit filter.
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	// Pressing Space when nothing matches must NOT silently toggle hidden group 0.
	m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := m.Result().Selected; len(got) != 0 {
		t.Errorf("Space on 0-match filter toggled hidden items: %v", got)
	}
}

func TestFilterMode_CtrlCQuits(t *testing.T) {
	groups := []Group{{
		Key:   "g1",
		Title: "Group 1",
		Items: []Item{{ID: "a", Primary: "alpha", Selectable: true}},
	}}
	m := New(groups, Options{Title: "Test"})

	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd on ctrl+c in filter mode")
	}
	if !m.Result().Quit {
		t.Error("expected Result().Quit == true after ctrl+c in filter mode")
	}
}
