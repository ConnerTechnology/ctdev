//go:build unix

package cmd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// backupNowStandin is a configured folder whose destination's restic is a
// stand-in at ~/.local/bin/restic, where ctdev looks first. backupCode is
// how backup exits (0 makes a backup); every forget fails.
func backupNowStandin(t *testing.T, backupCode string) (*backup.Machine, string) {
	t.Helper()
	dir := backupFolderForTest(t)
	m, err := backup.ThisMachine(sysutil.Opts{Stdout: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(m.Home, ".local", "bin", "restic")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
case "$1" in
version) echo "restic 0.19.1 compiled with go1.26.4 on linux/amd64" ;;
backup)
  if [ "` + backupCode + `" != 0 ]; then echo "Fatal: repository is already locked" >&2; exit ` + backupCode + `; fi
  echo '{"message_type":"summary","snapshot_id":"0123456789abcdef","total_files_processed":1,"total_bytes_processed":1,"data_added":1,"backup_end":"2026-10-22T12:00:00Z"}' ;;
snapshots) echo '[]' ;;
stats) echo '{"total_size": 0}' ;;
forget) echo "Fatal: example failure" >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	env := filepath.Join(m.Dir, "destinations", "example-backups.env")
	if err := os.MkdirAll(filepath.Dir(env), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env, []byte("RESTIC_REPOSITORY="+filepath.Join(t.TempDir(), "repo")+"\nCTDEV_COMPUTER_KEY=example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := backup.WriteConfig(dir, backup.NewConfig("example-backups")); err != nil {
		t.Fatal(err)
	}
	saved := stdinScanner
	stdinScanner = bufio.NewScanner(strings.NewReader("y\n"))
	t.Cleanup(func() { stdinScanner = saved })
	return m, dir
}

// Back up now applies the keep rules through Retain, so a failure is
// recorded on the folder, as a scheduled run records it.
func TestBackUpNowRecordsARetentionFailure(t *testing.T) {
	m, dir := backupNowStandin(t, "0")
	ctx := context.Background()
	var err error
	out := captureStdout(t, func() { err = backupNow(ctx, m, m.Status(ctx, dir)) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Applying the keep rules failed") {
		t.Errorf("output:\n%s\nwant the keep-rules warning", out)
	}
	s := m.Status(ctx, dir)
	if s.Last == nil || s.Last.Failed() || !strings.Contains(s.Last.RetentionError, "example failure") {
		t.Errorf("last run = %+v, want a success with the keep-rules failure recorded", s.Last)
	}
}

// When the destination stays locked, Back up now says it's busy and to try
// again, rather than that the backup failed.
func TestBackUpNowSaysWhenTheDestinationIsBusy(t *testing.T) {
	m, dir := backupNowStandin(t, "11")
	ctx := context.Background()
	var err error
	captureStdout(t, func() { err = backupNow(ctx, m, m.Status(ctx, dir)) })
	if !errors.Is(err, errDestinationBusy) {
		t.Fatalf("err = %v, want the busy message", err)
	}
	if want := "The destination is busy (another computer's weekly maintenance or backup). Try again in a few minutes."; err.Error() != want {
		t.Errorf("message = %q, want %q", err, want)
	}
	if s := m.Status(ctx, dir); s.Last == nil || s.Last.Failed() || s.Last.Deferred == "" {
		t.Errorf("last run = %+v, want deferred, not failed", s.Last)
	}
}
