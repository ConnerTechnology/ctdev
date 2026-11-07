package settings

import (
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/setup"
)

// A multi-pick setting shows in the browser but is changed in its category
// wizard: Enter queues nothing and the detail line says where to go.
func TestMultiPickPointsToWizard(t *testing.T) {
	s := setup.Setting{Name: "Hide drives", Slug: "desktop", Category: "Desktop", Control: setup.ControlMultiPick}
	states := []setup.SettingState{{Setting: &s, CurrentValue: "AAAA-1111", DesiredValue: "AAAA-1111"}}
	m := New(states)
	press(m, "enter")
	if m.PendingCount() != 0 {
		t.Errorf("Enter queued a change on a multi-pick: pending = %d", m.PendingCount())
	}
	if out := m.footerView(); !strings.Contains(out, "ctdev configure desktop") {
		t.Errorf("footer does not point to the wizard: %q", out)
	}
}
