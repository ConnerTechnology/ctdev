package cmd

import (
	"strings"
	"testing"
)

// The system category is gone: TRIM moved to storage and the Logitech KVM fix
// to mouse (CTD-90 decision 20).
func TestConfigureSystemIsAnUnknownCategory(t *testing.T) {
	for _, c := range configureCmd.Commands() {
		if c.Name() == "system" {
			t.Fatal("configure still has a system subcommand")
		}
	}
	err := runConfigureAll(configureCmd, []string{"system"})
	if err == nil || !strings.Contains(err.Error(), `unknown configure category "system"`) {
		t.Errorf("configure system: err = %v, want unknown category", err)
	}
}
