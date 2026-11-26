package checklist

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestChecklist_KernelNotPreselectedAndEmptyList(t *testing.T) {
	m := New([]UpdateItem{
		{Name: "linux-image-6.8", Source: "apt", IsKernel: true},
		{Name: "jq", Source: "apt"},
	})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	res := m.GetResult()
	if len(res.Selected) != 1 || res.Selected[0].Name != "jq" {
		t.Errorf("IsKernel item should not be preselected: got %+v", res.Selected)
	}

	empty := New(nil)
	empty.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	empty.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(empty.GetResult().Selected) != 0 {
		t.Errorf("empty checklist selected %+v", empty.GetResult().Selected)
	}
}
