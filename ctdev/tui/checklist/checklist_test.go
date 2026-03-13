package checklist

import (
	"testing"
)

func testItems() []UpdateItem {
	return []UpdateItem{
		{Name: "curl", Source: "apt", CurrentVer: "7.88", NewVer: "8.0", IsMajor: true},
		{Name: "git", Source: "apt", CurrentVer: "2.39", NewVer: "2.40"},
		{Name: "zsh-plugins", Source: "git", CurrentVer: "", NewVer: "3 commits behind"},
	}
}

func TestChecklistAllSelectedByDefault(t *testing.T) {
	m := New(testItems())
	if len(m.selected) != 3 {
		t.Errorf("expected 3 selected by default, got %d", len(m.selected))
	}
}

func TestChecklistToggle(t *testing.T) {
	m := New(testItems())
	// Deselect first item
	delete(m.selected, 0)
	if len(m.selected) != 2 {
		t.Errorf("expected 2 selected after deselect, got %d", len(m.selected))
	}
}

func TestChecklistSelectNone(t *testing.T) {
	m := New(testItems())
	m.selected = make(map[int]bool)
	result := m.GetResult()
	// Not confirmed, so it's a quit
	m.confirmed = true
	result = m.GetResult()
	if len(result.Selected) != 0 {
		t.Errorf("expected 0 selected, got %d", len(result.Selected))
	}
}

func TestChecklistQuit(t *testing.T) {
	m := New(testItems())
	m.quitting = true
	result := m.GetResult()
	if !result.Quit {
		t.Error("expected quit result")
	}
}
