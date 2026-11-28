package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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

// Without a terminal, --all prints each destination's header and its
// folders, grouped by computer.
func TestBackupAllWithoutATerminalListsTheFolders(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("backup runs on Linux and macOS")
	}
	backupFolderForTest(t)
	home := os.Getenv("HOME")
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	// The command labels the local host. Pass its JSON string to the
	// stand-in as data so the shell never interprets the hostname.
	hostJSON, err := json.Marshal(host)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CTDEV_TEST_HOST_JSON", string(hostJSON))
	writeBackupTestFile(t, filepath.Join(home, ".config", "ctdev", "backup", "destinations", "example-backups.env"),
		"RESTIC_REPOSITORY="+filepath.Join(home, "repo")+"\nCTDEV_COMPUTER_KEY=example\n", 0o600)
	writeBackupTestFile(t, filepath.Join(home, ".local", "bin", "restic"), `#!/bin/sh
case "$1" in
version) echo 'restic 0.19.1' ;;
key) printf '[{"id":"abc","hostname":%s,"current":true}]\n' "$CTDEV_TEST_HOST_JSON" ;;
snapshots) printf '[{"id":"01234567","hostname":%s,"tags":["ctdev-folder:website","ctdev-path:~/Projects/website"]}]\n' "$CTDEV_TEST_HOST_JSON" ;;
stats) echo '{"total_size":123}' ;;
*) exit 1 ;;
esac
`, 0o755)

	out, err := runBackupAllBatch(t)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	header, _, _ := strings.Cut(out, "\n")
	if header != "example-backups (local path) · 1 computer · 1 folder · 123 B stored" {
		t.Errorf("header = %q, want it to count the space stored", header)
	}
	for _, want := range []string{host + " (this computer)", "~/Projects/website", "1 backup"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
}
