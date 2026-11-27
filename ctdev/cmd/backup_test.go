package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
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

	// The whole-machine restic backup is gone: backup's only subcommand is
	// the hidden one the scheduler runs, and restore and the restic wizard no
	// longer exist.
	backup := childCommand(t, rootCmd, "backup")
	if len(backup.Commands()) != 1 || !childCommand(t, backup, "run-scheduled").Hidden {
		t.Errorf("backup should have only the hidden run-scheduled subcommand, got %d", len(backup.Commands()))
	}
	if hasSubcommand(rootCmd, "restore") {
		t.Error("ctdev restore should be removed")
	}

	configure := childCommand(t, rootCmd, "configure")
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

// backupFolderForTest makes ~/Projects/website under a temp HOME and runs the
// test from inside it.
func backupFolderForTest(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "Projects", "website")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<p>example</p>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

func runBackupBatch(t *testing.T) (string, error) {
	t.Helper()
	flagBatch = true
	t.Cleanup(func() { flagBatch = false })
	var out bytes.Buffer
	backupCmd.SetOut(&out)
	t.Cleanup(func() { backupCmd.SetOut(nil) })
	err := backupCmd.RunE(backupCmd, nil)
	// Output that isn't a terminal carries no styling at all.
	if strings.Contains(out.String(), "\x1b") {
		t.Errorf("status to a non-terminal has ANSI escapes: %q", out.String())
	}
	return out.String(), err
}

// Without a terminal, ctdev backup prints the status and fails in a folder
// that isn't backed up, so it works as a check.
func TestBackupWithoutATerminalFailsWhenNotBackedUp(t *testing.T) {
	backupFolderForTest(t)
	out, err := runBackupBatch(t)
	if err == nil {
		t.Error("want a non-zero exit in a folder that isn't backed up")
	}
	if !strings.HasPrefix(out, "Not backed up: ~/Projects/website\n") || !strings.Contains(out, "Last backup") {
		t.Errorf("status = %q", out)
	}
}

func writeBackupTestFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func TestBackupWithoutATerminalChecksLocalScheduleAndSnapshots(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("the scheduler supports Linux and macOS")
	}
	for _, tt := range []struct {
		name                        string
		listed, installed, snapshot bool
		wantErr                     string
	}{
		{name: "config only", wantErr: "not scheduled"},
		{name: "timer missing", listed: true, snapshot: true, wantErr: "not scheduled"},
		{name: "first backup pending", listed: true, installed: true, wantErr: "no backups"},
		{name: "backed up", listed: true, installed: true, snapshot: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := backupFolderForTest(t)
			home := os.Getenv("HOME")
			cfg := backup.NewConfig("example-backups")
			if err := backup.WriteConfig(dir, cfg); err != nil {
				t.Fatal(err)
			}

			cfgDir := filepath.Join(home, ".config", "ctdev", "backup")
			writeBackupTestFile(t, filepath.Join(cfgDir, "destinations", cfg.Destination+".env"), "RESTIC_REPOSITORY="+filepath.Join(home, "repo")+"\nCTDEV_COMPUTER_KEY=example\n", 0o600)
			if tt.listed {
				now := time.Now().UTC().Format(time.RFC3339)
				writeBackupTestFile(t, filepath.Join(cfgDir, "folders.json"), `{"folders":[{"path":"`+dir+`","id":"`+cfg.ID+`","schedule":"daily","added":"`+now+`"}]}`, 0o600)
			}
			if tt.installed {
				if runtime.GOOS == "linux" {
					systemd := filepath.Join(home, ".config", "systemd", "user")
					for _, name := range []string{"ctdev-backup.service", "ctdev-backup.timer", "timers.target.wants/ctdev-backup.timer"} {
						writeBackupTestFile(t, filepath.Join(systemd, name), "", 0o600)
					}
				} else {
					writeBackupTestFile(t, filepath.Join(home, "Library", "LaunchAgents", "io.connertechnology.ctdev.backup.plist"), "", 0o600)
				}
			}
			snapshots := "[]"
			if tt.snapshot {
				snapshots = `[{"id":"0123456789abcdef","time":"2026-10-21T12:00:00Z","tags":["ctdev-folder:` + cfg.ID + `"]}]`
			}
			writeBackupTestFile(t, filepath.Join(home, ".local", "bin", "restic"), `#!/bin/sh
case "$*" in
version) echo 'restic 0.19.1' ;;
snapshots*ctdev-maintenance*) echo '[]' ;;
snapshots*) echo '`+snapshots+`' ;;
stats*) echo '{"total_size": 1}' ;;
*) exit 1 ;;
esac
`, 0o755)
			out, err := runBackupBatch(t)
			if tt.wantErr == "" && err != nil || tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("want error containing %q, got %v; status:\n%s", tt.wantErr, err, out)
			}
		})
	}
}

// A config file that can't be read fails the check with the reason, and
// the status is the folder's, not "Not backed up".
func TestBackupWithoutATerminalFailsWhenTheConfigCantBeRead(t *testing.T) {
	dir := backupFolderForTest(t)
	body := `{"id": "website", "destination": "example-backups", "schedule": "daily", "keep": {}}`
	if err := os.WriteFile(filepath.Join(dir, backup.ConfigFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runBackupBatch(t)
	if err == nil || !strings.Contains(err.Error(), "isn't a UUID") {
		t.Errorf("want a failure with the reason, got %v", err)
	}
	if !strings.HasPrefix(out, "Backed up folder: ~/Projects/website · config can't be read: ") {
		t.Errorf("status = %q", out)
	}
}

// A check that passes when backups can't run is useless: a destination this
// computer isn't set up on fails it.
func TestBackupWithoutATerminalFailsWhenTheDestinationIsMissing(t *testing.T) {
	dir := backupFolderForTest(t)
	if err := backup.WriteConfig(dir, backup.NewConfig("example-backups")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(dir, "docs"))
	out, err := runBackupBatch(t)
	if err == nil || !strings.Contains(err.Error(), "example-backups isn't set up on this computer") {
		t.Errorf("want a failure naming the missing destination, got %v", err)
	}
	if !strings.HasPrefix(out, "Backed up folder: ~/Projects/website  (you're in ~/Projects/website/docs)\n") ||
		!strings.Contains(out, "not scheduled on this computer") {
		t.Errorf("status = %q", out)
	}
}

// A destination that can't be read, here because restic isn't installed,
// fails the check too.
func TestBackupWithoutATerminalFailsWhenTheDestinationCantBeRead(t *testing.T) {
	dir := backupFolderForTest(t)
	t.Setenv("PATH", t.TempDir()) // no restic anywhere
	cfg := backup.NewConfig("example-backups")
	if err := backup.WriteConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	env := filepath.Join(os.Getenv("HOME"), ".config", "ctdev", "backup", "destinations", "example-backups.env")
	if err := os.MkdirAll(filepath.Dir(env), 0o700); err != nil {
		t.Fatal(err)
	}
	body := "RESTIC_REPOSITORY=" + filepath.Join(t.TempDir(), "repo") + "\nCTDEV_COMPUTER_KEY=example\n"
	if err := os.WriteFile(env, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runBackupBatch(t)
	if err == nil || !strings.Contains(err.Error(), "can't read") {
		t.Errorf("want a failure saying the destination can't be read, got %v", err)
	}
	if !strings.Contains(out, "Problem") {
		t.Errorf("status = %q", out)
	}
}

// The non-terminal check fails for an overdue folder.
func TestBackupWithoutATerminalFailsWhenOverdue(t *testing.T) {
	dir := backupFolderForTest(t)
	t.Setenv("PATH", t.TempDir()) // the destination isn't read: it isn't set up here
	cfg := backup.NewConfig("example-backups")
	if err := backup.WriteConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	cfgDir := filepath.Join(os.Getenv("HOME"), ".config", "ctdev", "backup")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	long := "2020-01-01T00:00:00Z"
	writeFile := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(cfgDir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("folders.json", `{"folders":[{"path":"`+dir+`","id":"`+cfg.ID+`","schedule":"daily","added":"`+long+`"}]}`)
	writeFile("state.json", `{"folders":{"`+dir+`":{"last_attempt":"`+long+`","last_success":"`+long+`"}}}`)
	// What this computer knows, failed or overdue, is the error, ahead of
	// the destination not being set up here.
	out, err := runBackupBatch(t)
	if err == nil || !strings.Contains(err.Error(), "overdue") {
		t.Fatalf("want a non-zero exit saying the folder is overdue, got %v; status:\n%s", err, out)
	}
	if !strings.Contains(out, "overdue: no backup since") {
		t.Errorf("status = %q", out)
	}
}

// run-scheduled prints each listed folder's outcome and fails when one did.
func TestRunScheduledReportsEachFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	notes := &recordingNotifier{}
	scheduledNotifier = notes
	t.Cleanup(func() { scheduledNotifier = nil })
	var out bytes.Buffer
	backupRunScheduledCmd.SetOut(&out)
	t.Cleanup(func() { backupRunScheduledCmd.SetOut(nil) })
	if err := backupRunScheduledCmd.RunE(backupRunScheduledCmd, nil); err != nil || out.Len() != 0 {
		t.Errorf("with nothing scheduled: err %v, output %q", err, out.String())
	}

	dir := filepath.Join(home, ".config", "ctdev", "backup")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(home, "gone")
	body := `{"folders":[{"path":"` + gone + `","id":"00000000-0000-4000-8000-000000000000","schedule":"daily"}]}`
	if err := os.WriteFile(filepath.Join(dir, "folders.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	err := backupRunScheduledCmd.RunE(backupRunScheduledCmd, nil)
	if err == nil || !strings.HasPrefix(out.String(), "~/gone: missing: the folder isn't there any more") {
		t.Errorf("err %v, output %q", err, out.String())
	}
	if len(notes.titles) != 1 || notes.titles[0] != "Backup failing" {
		t.Errorf("notifications = %q, want one Backup failing", notes.titles)
	}
}

type recordingNotifier struct{ titles []string }

func (n *recordingNotifier) Notify(_ context.Context, title, _ string) error {
	n.titles = append(n.titles, title)
	return nil
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

// Per the --batch check contract, backupStatusOnly must exit non-zero on every
// backup health problem: failed data checks, overdue data checks, and retention
// failures in addition to failed/overdue backups.
func TestBackupStatusOnly_FailsOnDataCheckAndRetentionProblems(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX stand-in restic")
	}

	setupHealthy := func(t *testing.T, stateJSON, maintJSON string) (*backup.Machine, string) {
		t.Helper()
		home := t.TempDir()
		dir := filepath.Join(home, "Projects", "website")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("hi"), 0o644); err != nil {
			t.Fatal(err)
		}
		now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
		cfg := backup.Config{
			ID:          "11111111-1111-4111-8111-111111111111",
			Destination: "example-backups",
			Schedule:    backup.Daily,
			Keep:        backup.DefaultKeep,
		}
		if err := backup.WriteConfig(dir, cfg); err != nil {
			t.Fatal(err)
		}
		confDir := filepath.Join(home, ".config", "ctdev", "backup")
		destDir := filepath.Join(confDir, "destinations")
		if err := os.MkdirAll(destDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(destDir, "example-backups.env"),
			[]byte("RESTIC_REPOSITORY="+filepath.Join(home, "repo")+"\nCTDEV_COMPUTER_KEY=pw\nCTDEV_COMPUTER_KEY_ID=k1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(confDir, "folders.json"),
			[]byte(`{"folders":[{"path":"`+dir+`","id":"11111111-1111-4111-8111-111111111111","schedule":"daily","destination":"example-backups","added":"2026-10-01T00:00:00Z"}]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(confDir, "state.json"), []byte(stateJSON), 0o600); err != nil {
			t.Fatal(err)
		}

		// Stand-in restic: returns 1 snapshot, stats, and optional maintenance marker.
		maintSnap := "[]"
		if maintJSON != "" {
			enc := base64.RawURLEncoding.EncodeToString([]byte(maintJSON))
			maintSnap = `[{"id":"cccccccccccccccc","short_id":"cccccccc","time":"2026-11-25T00:00:00Z","hostname":"example-laptop","tags":["ctdev-maintenance","ctdev-maintenance-data:` + enc + `"]}]`
		}
		bin := filepath.Join(home, "restic")
		script := `#!/bin/sh
case "$1" in
version) echo "restic 0.19.1 compiled with go1.26.4 on linux/amd64" ;;
snapshots)
  case "$*" in
    *ctdev-maintenance*)
      printf '%s\n' '` + maintSnap + `' ;;
    *)
      echo '[{"id":"aaaaaaaaaaaaaaaa","short_id":"aaaaaaaa","time":"2026-11-26T10:00:00Z","hostname":"example-laptop","tags":["ctdev-folder:11111111-1111-4111-8111-111111111111","ctdev-path:~/Projects/website","ctdev-host:example-laptop"],"summary":{"total_files_processed":1,"total_bytes_processed":2,"data_added":2}}]' ;;
  esac ;;
stats) echo '{"total_size": 100}' ;;
esac
`
		if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}

		// Fake systemd user service, timer, and wants link so SchedulerInstalled == true.
		unitDir := filepath.Join(home, ".config", "systemd", "user")
		wantsDir := filepath.Join(unitDir, "timers.target.wants")
		if err := os.MkdirAll(wantsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(unitDir, "ctdev-backup.service"), []byte("[Service]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(unitDir, "ctdev-backup.timer"), []byte("[Timer]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(unitDir, "ctdev-backup.timer"), filepath.Join(wantsDir, "ctdev-backup.timer")); err != nil {
			t.Fatal(err)
		}
		t.Setenv("HOME", home)

		m := &backup.Machine{
			Dir:      confDir,
			Home:     home,
			Hostname: "example-laptop",
			Username: "example",
			Restic:   bin,
			GOOS:     "linux",
			Now:      func() time.Time { return now },
		}
		return m, dir
	}

	okRun := func(dir, retErr string) string {
		return `{"folders":{"` + dir + `":{"destination":"example-backups","last_attempt":"2026-11-26T10:00:00Z","last_success":"2026-11-26T10:00:00Z","retention_error":"` + retErr + `"}}}`
	}

	t.Run("healthy exits zero", func(t *testing.T) {
		m, dir := setupHealthy(t, "", `{"started":"2026-11-24T23:55:00Z","finished":"2026-11-25T00:00:00Z","host":"example-laptop","slice":1,"ok":true}`)
		_ = os.WriteFile(filepath.Join(m.Dir, "state.json"), []byte(okRun(dir, "")), 0o600)
		var out bytes.Buffer
		if err := backupStatusOnly(context.Background(), m, dir, &out); err != nil {
			t.Errorf("backupStatusOnly(healthy) = %v, want nil\nout:\n%s", err, out.String())
		}
	})

	t.Run("failed data check exits non-zero", func(t *testing.T) {
		m, dir := setupHealthy(t, "", `{"started":"2026-11-24T23:55:00Z","finished":"2026-11-25T00:00:00Z","host":"example-laptop","slice":1,"ok":false,"step":"check","error":"pack corrupt"}`)
		_ = os.WriteFile(filepath.Join(m.Dir, "state.json"), []byte(okRun(dir, "")), 0o600)
		var out bytes.Buffer
		err := backupStatusOnly(context.Background(), m, dir, &out)
		if err == nil || !strings.Contains(err.Error(), "data check") {
			t.Errorf("backupStatusOnly(failed data check) = %v, want data check error\nout:\n%s", err, out.String())
		}
	})

	t.Run("overdue data check exits non-zero", func(t *testing.T) {
		// No maintenance snapshot ever run, and folder added 2026-10-01 (>2 weeks before 2026-11-26).
		m, dir := setupHealthy(t, "", "")
		_ = os.WriteFile(filepath.Join(m.Dir, "state.json"), []byte(okRun(dir, "")), 0o600)
		var out bytes.Buffer
		err := backupStatusOnly(context.Background(), m, dir, &out)
		if err == nil || !strings.Contains(err.Error(), "data check") || !strings.Contains(err.Error(), "overdue") {
			t.Errorf("backupStatusOnly(overdue data check) = %v, want overdue data check error\nout:\n%s", err, out.String())
		}
	})

	t.Run("retention error exits non-zero", func(t *testing.T) {
		m, dir := setupHealthy(t, "", `{"started":"2026-11-24T23:55:00Z","finished":"2026-11-25T00:00:00Z","host":"example-laptop","slice":1,"ok":true}`)
		_ = os.WriteFile(filepath.Join(m.Dir, "state.json"), []byte(okRun(dir, "forget failed")), 0o600)
		var out bytes.Buffer
		err := backupStatusOnly(context.Background(), m, dir, &out)
		if err == nil || !strings.Contains(err.Error(), "retention") {
			t.Errorf("backupStatusOnly(retention error) = %v, want retention error\nout:\n%s", err, out.String())
		}
	})
}
