package checklist

import (
	"testing"

	tea "charm.land/bubbletea/v2"
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

func TestSourceLabel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"apt", "System Packages (apt)"},
		{"brew", "System Packages (brew)"},
		{"flatpak", "Flatpak"},
		{"git", "Git Repositories"},
		{"runtime", "Runtimes"},
		{"npm", "NPM Global Packages"},
		{"ctdev", "ctdev"},
		{"cli", "CLI Tools"},
		{"unknown", "unknown"},
		{"brew-cask", "brew-cask"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sourceLabel(tt.input)
			if got != tt.want {
				t.Errorf("sourceLabel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestChecklistKeyboardNavigation(t *testing.T) {
	m := New(testItems())

	// Down arrow moves cursor to 1
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("after down: cursor = %d, want 1", m.cursor)
	}

	// Up arrow moves cursor back to 0
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("after up: cursor = %d, want 0", m.cursor)
	}

	// Space deselects item 0 (was selected by default)
	m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if m.selected[0] {
		t.Error("after space: item 0 should be deselected")
	}

	// 'a' selects all items
	m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if len(m.selected) != 3 {
		t.Errorf("after 'a': selected = %d, want 3", len(m.selected))
	}

	// 'n' deselects all items
	m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if len(m.selected) != 0 {
		t.Errorf("after 'n': selected = %d, want 0", len(m.selected))
	}
}

func TestChecklistConfirmResult(t *testing.T) {
	m := New(testItems())

	// Deselect item 0
	delete(m.selected, 0)

	// Confirm
	m.confirmed = true

	result := m.GetResult()
	if len(result.Selected) != 2 {
		t.Errorf("expected 2 selected items, got %d", len(result.Selected))
	}
	if result.Quit {
		t.Error("expected non-quit result")
	}
}
