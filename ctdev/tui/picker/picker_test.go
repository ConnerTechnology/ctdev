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
	m := New(testComponents(), map[string]bool{}, component.OSLinux)
	m.moveCursor(1) // skip category header to first component
	m.toggleSelected()

	if len(m.selected) != 1 {
		t.Errorf("expected 1 selected, got %d", len(m.selected))
	}
}

func TestPickerQuitReturnsNoSelection(t *testing.T) {
	m := New(testComponents(), map[string]bool{}, component.OSLinux)
	m.quitting = true

	result := m.GetResult()
	if !result.Quit {
		t.Error("expected quit result")
	}
}
