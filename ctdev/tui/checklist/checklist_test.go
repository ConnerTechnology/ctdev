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
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // confirm
	result := m.GetResult()
	if result.Quit {
		t.Fatal("expected confirm, not quit")
	}
	if len(result.Selected) != 3 {
		t.Errorf("expected all 3 selected by default, got %d", len(result.Selected))
	}
}

func TestChecklistToggleThenConfirm(t *testing.T) {
	m := New(testItems())
	// Cursor starts on the apt header; down lands on the first item (curl).
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeySpace}) // deselect curl
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // confirm

	result := m.GetResult()
	if len(result.Selected) != 2 {
		t.Fatalf("expected 2 selected after deselecting curl, got %d (%v)", len(result.Selected), result.Selected)
	}
	for _, it := range result.Selected {
		if it.Name == "curl" {
			t.Error("curl should have been deselected")
		}
	}
}

func TestChecklistSelectNone(t *testing.T) {
	m := New(testItems())
	m.Update(tea.KeyPressMsg{Code: 'A', Text: "A"}) // none
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})   // confirm
	if got := len(m.GetResult().Selected); got != 0 {
		t.Errorf("expected 0 selected after 'A', got %d", got)
	}
}

func TestChecklistQuit(t *testing.T) {
	m := New(testItems())
	m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if !m.GetResult().Quit {
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
		{"brew-cask", "Desktop Apps (brew cask)"},
		{"mintupdate", "System Packages (Mint)"},
		{"flatpak", "Flatpak"},
		{"git", "Git Repositories"},
		{"runtime", "Runtimes"},
		{"npm", "NPM Global Packages"},
		{"ctdev", "ctdev"},
		{"cli", "CLI Tools"},
		{"docker", "Docker (containers)"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := sourceLabel(tt.input); got != tt.want {
				t.Errorf("sourceLabel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
