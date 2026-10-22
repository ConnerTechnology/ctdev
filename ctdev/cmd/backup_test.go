package cmd

import (
	"bytes"
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
	// There is no top-level pihole command; the Pi-hole component is reached
	// through install/configure.
	if hasSubcommand(rootCmd, "pihole") {
		t.Error("ctdev pihole should be removed")
	}
	// gpu moved under configure.
	if hasSubcommand(rootCmd, "gpu") {
		t.Error("ctdev gpu should be removed (folded into configure gpu)")
	}

	// The whole-machine restic backup is gone: backup is a bare placeholder
	// for per-folder backup, and restore and the restic wizard no longer exist.
	backup := childCommand(t, rootCmd, "backup")
	if len(backup.Commands()) != 0 {
		t.Errorf("backup should have no subcommands, got %d", len(backup.Commands()))
	}
	if hasSubcommand(rootCmd, "restore") {
		t.Error("ctdev restore should be removed")
	}

	configure := childCommand(t, rootCmd, "configure")
	if !hasSubcommand(configure, "gpu") {
		t.Error("configure should have a gpu subcommand")
	}
	if hasSubcommand(configure, "restic") {
		t.Error("configure should no longer have a restic subcommand")
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

func TestBackupSaysPerFolderBackupIsComing(t *testing.T) {
	var out bytes.Buffer
	backupCmd.SetOut(&out)
	t.Cleanup(func() { backupCmd.SetOut(nil) })
	if err := backupCmd.RunE(backupCmd, nil); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(out.String())
	if strings.Contains(got, "\n") || !strings.Contains(got, "CTD-87") {
		t.Errorf("want one line pointing at CTD-87, got %q", got)
	}
}

// The removed commands each fail as unknown commands.
func TestRemovedBackupCommandsAreUnknown(t *testing.T) {
	for _, args := range [][]string{
		{"backup", "now"},
		{"restore"},
		{"configure", "restic"},
	} {
		var out bytes.Buffer
		rootCmd.SetArgs(args)
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		err := rootCmd.Execute()
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		name := strings.Join(args, " ")
		if err == nil {
			t.Errorf("ctdev %s: want an error, got none", name)
			continue
		}
		if !strings.Contains(err.Error(), "unknown") {
			t.Errorf("ctdev %s: want an unknown-command error, got %v", name, err)
		}
	}
}
