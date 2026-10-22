package cmd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

func runBackupAllBatch(t *testing.T) (string, error) {
	t.Helper()
	flagBackupAll = true
	t.Cleanup(func() { flagBackupAll = false })
	return runBackupBatch(t)
}

// Without a terminal and with no destination, --all says how to connect one
// and writes nothing, here or under ~/.config.
func TestBackupAllWithoutATerminalOrADestination(t *testing.T) {
	dir := backupFolderForTest(t)
	_, err := runBackupAllBatch(t)
	if err == nil || !strings.Contains(err.Error(), "no backup destination") {
		t.Errorf("want a failure saying there's no destination, got %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("--all wrote into the current folder: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".config", "ctdev")); !os.IsNotExist(err) {
		t.Errorf("--all wrote under ~/.config/ctdev (%v)", err)
	}
}

// Creating a destination under --all refuses a local path relative to the
// current directory, which --all can be run from anywhere, but takes one
// named in full, even under the current directory: ~/Backups from ~.
func TestCreatingADestinationUnderAllNeedsAPathNamedInFull(t *testing.T) {
	backupFolderForTest(t)
	home := os.Getenv("HOME")
	t.Chdir(home)
	saved := stdinScanner
	t.Cleanup(func() { stdinScanner = saved })
	stdinScanner = bufio.NewScanner(strings.NewReader(strings.Join([]string{
		"2",               // Create a new destination
		"example-backups", // its name
		"4", "backups",    // local path, relative: refused
		"4", "~/Backups", // named in full, under the current directory: taken
		// then the input ends at the master password, which cancels
	}, "\n") + "\n"))
	m, err := backup.ThisMachine(sysutil.Opts{Stdout: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	var setupErr error
	out := captureStdout(t, func() { _, setupErr = destinationForAll(context.Background(), m) })
	if !errors.Is(setupErr, errPromptCancelled) {
		t.Errorf("want the setup cancelled when the input ends, got %v", setupErr)
	}
	if strings.Count(out, "give an absolute path or one starting with ~/") != 1 {
		t.Errorf("want only the relative path refused:\n%s", out)
	}
	if !strings.Contains(out, "never stores the master password") {
		t.Errorf("~/Backups should be taken, moving on to the master password:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(home, "Backups")); !os.IsNotExist(err) {
		t.Errorf("a cancelled setup made ~/Backups (%v)", err)
	}
}

// Without a terminal, --all prints each destination's header and its
// folders, grouped by computer.
func TestBackupAllWithoutATerminalListsTheFolders(t *testing.T) {
	if _, err := exec.LookPath("restic"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("restic is not on PATH; CI must install it before the Go tests")
		}
		t.Skip("restic is not installed; install it (ctdev install restic) to run this test")
	}
	dir := backupFolderForTest(t)
	t.Setenv("RESTIC_CACHE_DIR", filepath.Join(t.TempDir(), "cache"))
	ctx := context.Background()
	m, err := backup.ThisMachine(sysutil.Opts{Stdout: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	setup, err := m.CreateDestination(ctx, backup.NewDestination{Name: "example-backups", Address: filepath.Join(t.TempDir(), "repo")}, "correct horse example staple")
	if err != nil {
		t.Fatal(err)
	}
	cfg := backup.NewConfig("example-backups")
	if err := m.Configure(ctx, setup.Destination, dir, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Backup(ctx, setup.Destination, dir, &cfg); err != nil {
		t.Fatal(err)
	}

	out, err := runBackupAllBatch(t)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	header, _, _ := strings.Cut(out, "\n")
	if !strings.HasPrefix(header, "example-backups (local path) · 1 computer · 1 folder · ") ||
		!strings.HasSuffix(header, " stored") || strings.HasSuffix(header, " 0 B stored") {
		t.Errorf("header = %q, want it to count the space stored", header)
	}
	for _, want := range []string{m.Hostname + " (this computer)", "~/Projects/website", "1 backup"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
}
