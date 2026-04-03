package picker

import (
	"testing"

	"github.com/ConnerTechnology/dotfiles/ctdev/component"
)

func testComponents() []component.Component {
	return []component.Component{
		{Name: "docker", Description: "Container runtime", Category: component.CategoryCLI, SupportedOS: []component.OS{component.OSAny}},
		{Name: "btop", Description: "Resource monitor", Category: component.CategoryCLI, SupportedOS: []component.OS{component.OSAny}},
		{Name: "chrome", Description: "Browser", Category: component.CategoryDesktop, SupportedOS: []component.OS{component.OSAny}},
	}
}

func TestPickerSelectToggle(t *testing.T) {
	m := New(testComponents(), map[string]bool{}, component.OSLinux, ModeInstall)
	m.moveCursor(1) // skip category header to first component
	m.toggleSelected()

	if len(m.selected) != 1 {
		t.Errorf("expected 1 selected, got %d", len(m.selected))
	}
}

func TestPickerQuitReturnsNoSelection(t *testing.T) {
	m := New(testComponents(), map[string]bool{}, component.OSLinux, ModeInstall)
	m.quitting = true

	result := m.GetResult()
	if !result.Quit {
		t.Error("expected quit result")
	}
}

func TestMatchTags(t *testing.T) {
	tests := []struct {
		name   string
		tags   []string
		filter string
		want   bool
	}{
		{"match found", []string{"json", "parser"}, "json", true},
		{"no match", []string{"json", "parser"}, "xml", false},
		{"nil tags", nil, "anything", false},
		{"case insensitive", []string{"JSON"}, "json", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchTags(tt.tags, tt.filter)
			if got != tt.want {
				t.Errorf("matchTags(%v, %q) = %v, want %v", tt.tags, tt.filter, got, tt.want)
			}
		})
	}
}

func TestMatchesFilter(t *testing.T) {
	m := New(testComponents(), map[string]bool{}, component.OSLinux, ModeInstall)
	docker := component.Component{Name: "docker", Description: "Container runtime"}

	// Empty filter matches everything
	if !m.matchesFilter(docker) {
		t.Error("empty filter should match any component")
	}

	// Partial name match
	m.filter = "dock"
	if !m.matchesFilter(docker) {
		t.Error("filter 'dock' should match 'docker'")
	}

	// No match
	m.filter = "xyz"
	if m.matchesFilter(docker) {
		t.Error("filter 'xyz' should not match 'docker'")
	}
}

func TestSelectAllInstallMode(t *testing.T) {
	installed := map[string]bool{"docker": true}
	m := New(testComponents(), installed, component.OSLinux, ModeInstall)

	m.selectAll()

	if m.selected["docker"] {
		t.Error("docker is already installed and should not be selected in install mode")
	}
	if !m.selected["btop"] {
		t.Error("btop should be selected")
	}
	if !m.selected["chrome"] {
		t.Error("chrome should be selected")
	}
}

func TestSelectNone(t *testing.T) {
	m := New(testComponents(), map[string]bool{}, component.OSLinux, ModeInstall)

	// Select some items first
	m.selected["docker"] = true
	m.selected["btop"] = true

	m.selectNone()

	if len(m.selected) != 0 {
		t.Errorf("expected 0 selected after selectNone, got %d", len(m.selected))
	}
}

func TestToggleCategory(t *testing.T) {
	m := New(testComponents(), map[string]bool{}, component.OSLinux, ModeInstall)

	// Move cursor to the first category header (index 0)
	m.cursor = 0
	if !m.items[0].isCategory {
		t.Fatal("expected item 0 to be a category header")
	}

	m.toggleCategory()

	if !m.items[0].collapsed {
		t.Error("category should be collapsed after toggle")
	}

	m.toggleCategory()

	if m.items[0].collapsed {
		t.Error("category should be expanded after second toggle")
	}
}

func TestCountInCategory(t *testing.T) {
	m := New(testComponents(), map[string]bool{}, component.OSLinux, ModeInstall)

	count := m.countInCategory(component.CategoryCLI)
	if count != 2 {
		t.Errorf("expected 2 components in CategoryCLI, got %d", count)
	}
}

func TestCountInstalled(t *testing.T) {
	installed := map[string]bool{"docker": true}
	m := New(testComponents(), installed, component.OSLinux, ModeInstall)

	count := m.countInstalled()
	if count != 1 {
		t.Errorf("expected 1 installed, got %d", count)
	}
}
