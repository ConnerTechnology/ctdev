package picker

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/component"
)

func testComponents() []component.Component {
	return []component.Component{
		{Name: "docker", Description: "Container runtime", Category: component.CategoryCLI, SupportedOS: []component.OS{component.OSAny}},
		{Name: "btop", Description: "Resource monitor", Category: component.CategoryCLI, SupportedOS: []component.OS{component.OSAny}, Tags: []string{"monitor"}},
		{Name: "chrome", Description: "Browser", Category: component.CategoryDesktop, SupportedOS: []component.OS{component.OSAny}},
	}
}

func send(m *Model, msgs ...tea.Msg) {
	for _, msg := range msgs {
		m.Update(msg)
	}
}

func key(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

func selectAllConfirm(m *Model) Result {
	send(m, key('a'), tea.KeyPressMsg{Code: tea.KeyEnter})
	return m.GetResult()
}

func TestSelectAllInstallExcludesInstalled(t *testing.T) {
	m := New(testComponents(), map[string]bool{"docker": true}, component.OSLinux, ModeInstall)
	got := selectAllConfirm(&m).Selected
	if contains(got, "docker") {
		t.Error("installed docker must not be selected by select-all in install mode")
	}
	if !contains(got, "btop") || !contains(got, "chrome") {
		t.Errorf("btop and chrome should be selected, got %v", got)
	}
}

func TestGetResultDeterministicOrder(t *testing.T) {
	m := New(testComponents(), map[string]bool{}, component.OSLinux, ModeInstall)
	// Display order: CLI (docker, btop) then Desktop (chrome).
	want := []string{"docker", "btop", "chrome"}
	got := selectAllConfirm(&m).Selected
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestUninstallSelectsInstalled(t *testing.T) {
	installed := map[string]bool{"docker": true, "btop": true, "chrome": true}
	m := New(testComponents(), installed, component.OSLinux, ModeUninstall)
	got := selectAllConfirm(&m).Selected
	if len(got) != 3 {
		t.Errorf("uninstall select-all should pick all installed, got %v", got)
	}
}

func TestQuitReturnsNoSelection(t *testing.T) {
	m := New(testComponents(), map[string]bool{}, component.OSLinux, ModeInstall)
	send(&m, key('q'))
	if !m.GetResult().Quit {
		t.Error("expected quit result")
	}
}

// Filtering then select-all must scope to the visible match only.
func TestFilterScopesSelectAll(t *testing.T) {
	m := New(testComponents(), map[string]bool{}, component.OSLinux, ModeInstall)
	send(&m, key('/'), key('c'), key('h'), key('r'), key('o'), key('m'), key('e'),
		tea.KeyPressMsg{Code: tea.KeyEnter}) // apply filter "chrome"
	got := selectAllConfirm(&m).Selected
	if len(got) != 1 || got[0] != "chrome" {
		t.Errorf("filtered select-all should pick only chrome, got %v", got)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
