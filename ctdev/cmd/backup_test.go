package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestResolveBackupServices(t *testing.T) {
	all, err := resolveBackupServices(nil)
	if err != nil {
		t.Fatalf("no args: unexpected error: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("no args should resolve to all providers")
	}

	one, err := resolveBackupServices([]string{"pihole"})
	if err != nil {
		t.Fatalf("pihole: unexpected error: %v", err)
	}
	if len(one) != 1 || one[0].name != "pihole" {
		t.Fatalf("expected [pihole], got %v", one)
	}

	if _, err := resolveBackupServices([]string{"bogus"}); err == nil {
		t.Error("unknown service should error")
	}
}

func hasSubcommand(parent *cobra.Command, name string) bool {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return true
		}
	}
	return false
}

func childCommand(t *testing.T, parent *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	t.Fatalf("%s has no %q subcommand", parent.Name(), name)
	return nil
}

func TestCommandTreeShape(t *testing.T) {
	// backup/restore replace the old top-level pihole command.
	if !hasSubcommand(rootCmd, "backup") || !hasSubcommand(rootCmd, "restore") {
		t.Error("expected top-level backup and restore commands")
	}
	if hasSubcommand(rootCmd, "pihole") {
		t.Error("ctdev pihole should be removed (folded into backup/restore)")
	}
	// gpu moved under configure.
	if hasSubcommand(rootCmd, "gpu") {
		t.Error("ctdev gpu should be removed (folded into configure gpu)")
	}

	backup := childCommand(t, rootCmd, "backup")
	if !hasSubcommand(backup, "now") || !hasSubcommand(backup, "snapshots") {
		t.Error("backup should front restic via now/snapshots")
	}

	configure := childCommand(t, rootCmd, "configure")
	if !hasSubcommand(configure, "gpu") {
		t.Error("configure should have a gpu subcommand")
	}
	// And exactly one gpu subcommand (no duplicate from the slugOrder auto-loop).
	count := 0
	for _, c := range configure.Commands() {
		if c.Name() == "gpu" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one configure gpu command, got %d", count)
	}
}
