package setup

import (
	"strings"
	"testing"
)

// TestBootSettingsPlainNamesAndWhy pins the boot category to CTD-90's
// "Every setting" rule: each setting has a plain name (what you get, not
// GRUB's variable) and a one-line "why" in the wizard, and the recommended
// defaults are the ones on Thomas's Linux desktop (menu, 10 s, OS prober on).
func TestBootSettingsPlainNamesAndWhy(t *testing.T) {
	want := []struct {
		name    string
		def     string
		grubVar string
	}{
		{"Boot menu at startup", "menu", "GRUB_TIMEOUT_STYLE"},
		{"Boot menu wait time", "10", "GRUB_TIMEOUT"},
		{"Other operating systems in the boot menu", "enabled", "GRUB_DISABLE_OS_PROBER"},
	}

	boot := FilterBySlug(Registry, "boot")
	if len(boot) != len(want) {
		t.Fatalf("boot has %d settings, want %d", len(boot), len(want))
	}
	for i, w := range want {
		s := boot[i]
		if s.Name != w.name {
			t.Errorf("boot[%d].Name = %q, want %q", i, s.Name, w.name)
		}
		if strings.Contains(s.Name, "GRUB") {
			t.Errorf("%q: plain name must not name GRUB", s.Name)
		}
		if s.Default != w.def {
			t.Errorf("%q: Default = %q, want %q", s.Name, s.Default, w.def)
		}
		why := strings.TrimSpace(s.Description)
		if why == "" {
			t.Errorf("%q: no why", s.Name)
		}
		if strings.Contains(why, "\n") {
			t.Errorf("%q: why must be one line, got %q", s.Name, why)
		}
		if strings.Contains(why, w.grubVar) {
			t.Errorf("%q: why names the mechanism %s", s.Name, w.grubVar)
		}
	}
}
