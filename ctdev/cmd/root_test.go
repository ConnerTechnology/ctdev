package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestTopLevelName(t *testing.T) {
	root := &cobra.Command{Use: "ctdev"}
	doctor := &cobra.Command{Use: "doctor"}
	backup := &cobra.Command{Use: "backup"}
	backupTest := &cobra.Command{Use: "test"}
	backup.AddCommand(backupTest)
	root.AddCommand(doctor, backup)

	tests := []struct {
		cmd  *cobra.Command
		want string
	}{
		{root, "ctdev"},
		{doctor, "doctor"},
		{backup, "backup"},
		// A nested command reports the top-level name the guard gates on, not
		// its own — otherwise `ctdev backup test` would be read as "test".
		{backupTest, "backup"},
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
