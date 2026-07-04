package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

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

	// backup fronts restic snapshots only (no per-service export anymore).
	backup := childCommand(t, rootCmd, "backup")
	if !hasSubcommand(backup, "now") || !hasSubcommand(backup, "snapshots") {
		t.Error("backup should front restic via now/snapshots")
	}

	// restore wraps the restic restore helper.
	restore := childCommand(t, rootCmd, "restore")
	for _, sub := range []string{"ls", "files", "in-place", "check"} {
		if !hasSubcommand(restore, sub) {
			t.Errorf("restore should have a %q subcommand", sub)
		}
	}

	configure := childCommand(t, rootCmd, "configure")
	if !hasSubcommand(configure, "gpu") {
		t.Error("configure should have a gpu subcommand")
	}
	if !hasSubcommand(configure, "restic") {
		t.Error("configure should have a restic subcommand")
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

func TestBackupRejectsServiceArg(t *testing.T) {
	// The old `ctdev backup pihole` form is gone; a stray arg must error, not
	// silently run a snapshot.
	if err := backupCmd.RunE(backupCmd, []string{"pihole"}); err == nil {
		t.Error("expected error when a service name is passed to backup")
	}
}

func TestBackupPathsContent(t *testing.T) {
	out := backupPathsContent([]string{"/home/x", "/etc"})
	if !strings.Contains(out, "/home/x\n") || !strings.Contains(out, "/etc\n") {
		t.Errorf("paths missing from rendered file:\n%s", out)
	}
	if !strings.HasPrefix(out, "#") {
		t.Error("expected a leading comment header")
	}
}

func TestGeneratePassword(t *testing.T) {
	a, err := generatePassword(32)
	if err != nil {
		t.Fatal(err)
	}
	b, err := generatePassword(32)
	if err != nil {
		t.Fatal(err)
	}
	if a == "" || a == b {
		t.Errorf("expected two distinct non-empty passwords, got %q and %q", a, b)
	}
}
