package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestTopLevelName(t *testing.T) {
	root := &cobra.Command{Use: "ctdev"}
	doctor := &cobra.Command{Use: "doctor"}
	configure := &cobra.Command{Use: "configure"}
	configureGPU := &cobra.Command{Use: "gpu"}
	configure.AddCommand(configureGPU)
	root.AddCommand(doctor, configure)

	tests := []struct {
		cmd  *cobra.Command
		want string
	}{
		{root, "ctdev"},
		{doctor, "doctor"},
		{configure, "configure"},
		// A nested command reports the top-level name the guard gates on, not
		// its own — otherwise `ctdev configure gpu` would be read as "gpu".
		{configureGPU, "configure"},
	}
	for _, tt := range tests {
		if got := topLevelName(tt.cmd); got != tt.want {
			t.Errorf("topLevelName(%q) = %q, want %q", tt.cmd.Use, got, tt.want)
		}
	}
}

// Every name in windowsCommands must exist in the real tree, or the guard is
// silently allowing a command that was renamed out from under it. The root
// itself and cobra's help/completion commands are added during Execute rather
// than init, so they're exempt.
func TestWindowsCommandsExist(t *testing.T) {
	generated := map[string]bool{
		"ctdev":      true,
		"help":       true,
		"completion": true,
		"__complete": true,
	}
	for name := range windowsCommands {
		if generated[name] {
			continue
		}
		if !hasSubcommand(rootCmd, name) {
			t.Errorf("windowsCommands allows %q but rootCmd has no such command", name)
		}
	}
}
