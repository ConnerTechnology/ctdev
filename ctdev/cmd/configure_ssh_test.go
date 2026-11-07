package cmd

import (
	"strings"
	"testing"
)

// linger, locale, tunnel and ufw merged into `configure ssh` (CTD-99), so
// their old category names are unknown, not silently a full sweep.
func TestConfigureRemoteAccessCategoriesMergedIntoSSH(t *testing.T) {
	configure := childCommand(t, rootCmd, "configure")
	if !hasSubcommand(configure, "ssh") {
		t.Fatal("configure should have an ssh subcommand")
	}
	for _, gone := range []string{"linger", "locale", "tunnel", "ufw"} {
		if hasSubcommand(configure, gone) {
			t.Errorf("configure %s should be gone (merged into configure ssh)", gone)
		}
		err := runConfigureAll(configure, []string{gone})
		if err == nil || !strings.Contains(err.Error(), "unknown configure category") {
			t.Errorf("configure %s: got %v, want an unknown configure category error", gone, err)
		}
	}
}
