package picker

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/component"
)

func TestPicker_TagSearchAndUnsupportedOS(t *testing.T) {
	comps := []component.Component{
		{Name: "age", Description: "file encryption", Category: component.CategorySecurity, SupportedOS: []component.OS{component.OSAny}, Tags: []string{"crypto"}},
		{Name: "mac-only", Description: "Mac app", Category: component.CategoryDesktop, SupportedOS: []component.OS{component.OSMacOS}},
	}
	m := New(comps, map[string]bool{}, component.OSLinux, ModeInstall, false)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})

	// Filter by tag "crypto" should find "age".
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, r := range "crypto" {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	v := m.View().Content
	if !strings.Contains(v, "age") {
		t.Errorf("tag search 'crypto' did not match age:\n%s", v)
	}
	if strings.Contains(v, "mac-only") {
		t.Errorf("tag search 'crypto' unexpectedly showed mac-only:\n%s", v)
	}
}
