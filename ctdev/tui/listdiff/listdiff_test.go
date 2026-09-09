package listdiff

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/piholelists"
)

func testChanges() []piholelists.Change {
	live := piholelists.Entry{Kind: piholelists.Allow, Key: "a.example", Comment: "old", Groups: []string{"Default"}, Enabled: true}
	return []piholelists.Change{
		{Op: piholelists.Add, Entry: piholelists.Entry{Kind: piholelists.Adlist, Key: "https://example.com/ads.txt", Comment: "new list", Groups: []string{"Default"}, Enabled: true}},
		{Op: piholelists.Update, Entry: piholelists.Entry{Kind: piholelists.Allow, Key: "a.example", Comment: "new", Groups: []string{"Default"}, Enabled: true}, Live: &live},
		{Op: piholelists.Remove, Entry: piholelists.Entry{Kind: piholelists.DenyRegex, Key: `^gone\.`, Groups: []string{"Default"}, Enabled: true}},
	}
}

func TestPreselectsAddAndUpdateButNotRemove(t *testing.T) {
	m := New(testChanges())
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	result := m.GetResult()
	if result.Quit {
		t.Fatal("expected confirm, not quit")
	}
	if len(result.Selected) != 2 {
		t.Fatalf("expected the add and the update preselected, got %d", len(result.Selected))
	}
	for _, c := range result.Selected {
		if c.Op == piholelists.Remove {
			t.Error("a remove must be opt-in — unchecked means keep it on the Pi-hole")
		}
	}
}

func TestRemoveCanBeOptedInto(t *testing.T) {
	m := New(testChanges())
	// One group per kind, so the remove is the last row: End lands on it.
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	result := m.GetResult()
	if len(result.Selected) != 3 {
		t.Fatalf("expected all three after opting into the remove, got %d", len(result.Selected))
	}
	if result.Selected[2].Op != piholelists.Remove {
		t.Errorf("selections keep display order, got %v last", result.Selected[2].Op)
	}
}

func TestQuitSelectsNothing(t *testing.T) {
	m := New(testChanges())
	m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})

	result := m.GetResult()
	if !result.Quit {
		t.Fatal("expected quit")
	}
	if len(result.Selected) != 0 {
		t.Errorf("quitting must apply nothing, got %d", len(result.Selected))
	}
}

func TestViewGroupsByKindAndShowsTheChange(t *testing.T) {
	m := New(testChanges())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := m.View().Content

	for _, want := range []string{"Adlists", "Allow (exact)", "Deny regex", "ADD", "UPDATE", "REMOVE", "a.example", `comment "old"`} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
}

func TestNoChangesIsAnEmptyPicker(t *testing.T) {
	m := New(nil)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := m.GetResult(); len(got.Selected) != 0 {
		t.Errorf("expected nothing selected, got %d", len(got.Selected))
	}
}
