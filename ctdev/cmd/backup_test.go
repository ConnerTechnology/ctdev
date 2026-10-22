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

// Back up now and Configure refuse a status with no folder to act on.
func TestActionsRefuseAnEmptyTarget(t *testing.T) {
	backupFolderForTest(t)
	m, err := backup.ThisMachine(sysutil.Opts{Stdout: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var s backup.Status // what a status that couldn't find the folder looks like
	if err := backupNow(ctx, m, s); err == nil || err.Error() != "no folder to act on" {
		t.Errorf("backupNow with no target: want \"no folder to act on\", got %v", err)
	}
	if err := configureFolder(ctx, m, s); err == nil || err.Error() != "no folder to act on" {
		t.Errorf("configureFolder with no target: want \"no folder to act on\", got %v", err)
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

// Configure in a folder on a computer with no destination walks through
// creating one, then writes the folder's config file with no secret in it.
func TestConfigureCreatesADestinationAndConfiguresTheFolder(t *testing.T) {
	if _, err := exec.LookPath("restic"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("restic is not on PATH; CI must install it before the Go tests")
		}
		t.Skip("restic is not installed; install it (ctdev install restic) to run this test")
	}
	dir := backupFolderForTest(t)
	t.Setenv("RESTIC_CACHE_DIR", filepath.Join(t.TempDir(), "cache"))
	repo := filepath.Join(t.TempDir(), "example-backups")
	const master = "correct horse example staple"

	saved := stdinScanner
	t.Cleanup(func() { stdinScanner = saved })
	stdinScanner = bufio.NewScanner(strings.NewReader(strings.Join([]string{
		"",                            // destination: 1) Create a new destination
		"example-backups",             // its name
		"4",                           // local path
		"~/Projects/website/.backups", // inside the folder: refused, asked again
		"4",
		repo,
		master, master,
		"",                 // Enter once the kit and Recovery Code are saved
		"1",                // Hourly
		"", "", "", "", "", // default retention
	}, "\n") + "\n"))

	ctx := context.Background()
	m, err := backup.ThisMachine(sysutil.Opts{Stdout: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	var cfgErr error
	out := captureStdout(t, func() { cfgErr = configureFolder(ctx, m, m.Status(ctx, dir)) })
	if cfgErr != nil {
		t.Fatalf("configureFolder: %v\n%s", cfgErr, out)
	}
	for _, want := range []string{"back the repository up into itself", "Emergency Kit", "Secret Key:", "Recovery Code", "can't be decrypted"} {
		if !strings.Contains(out, want) {
			t.Errorf("setup output is missing %q", want)
		}
	}

	cfg, err := backup.ReadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Destination != "example-backups" || cfg.Schedule != backup.Hourly || cfg.Keep != backup.DefaultKeep {
		t.Errorf("config = %+v", cfg)
	}
	data, _ := os.ReadFile(filepath.Join(dir, backup.ConfigFileName))
	if strings.Contains(string(data), "CT1-") || strings.Contains(string(data), master) || strings.Contains(string(data), repo) {
		t.Errorf("the folder's config file holds a secret or the address:\n%s", data)
	}
	if s := m.Status(ctx, dir); !s.Scheduled || s.DestinationMissing {
		t.Errorf("after Configure: scheduled %v, destination missing %v", s.Scheduled, s.DestinationMissing)
	}

	// Configured, readable and never failed: the non-terminal check passes.
	if out, err := runBackupBatch(t); err != nil {
		t.Errorf("status check after Configure: %v\n%s", err, out)
	}

	// From a subfolder, Configure reconfigures the backed-up folder above it,
	// starting from its current values.
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	stdinScanner = bufio.NewScanner(strings.NewReader(strings.Join([]string{
		"",                 // destination: 1) example-backups, the current one
		"3",                // Weekly
		"", "", "", "", "", // keep the retention
	}, "\n") + "\n"))
	out = captureStdout(t, func() { cfgErr = configureFolder(ctx, m, m.Status(ctx, docs)) })
	if cfgErr != nil {
		t.Fatalf("configureFolder from a subfolder: %v\n%s", cfgErr, out)
	}
	again, err := backup.ReadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if again.Schedule != backup.Weekly || again.ID != cfg.ID || again.Keep != cfg.Keep {
		t.Errorf("reconfigured from a subfolder: %+v, want weekly with the same ID and retention", again)
	}
	if _, err := os.Stat(filepath.Join(docs, backup.ConfigFileName)); !os.IsNotExist(err) {
		t.Error("no config file may be written in the subfolder")
	}
}

// Stop backing up offers to delete the folder's backups, so it refuses when
// it can't see them, rather than stop without that chance. It refuses before
// asking anything, and the folder stays backed up.
func TestStopBackingUpRefusesWhenTheBackupsCantBeRead(t *testing.T) {
	dir := backupFolderForTest(t)
	cfg := backup.NewConfig("example-backups")
	if err := backup.WriteConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	m, err := backup.ThisMachine(sysutil.Opts{Stdout: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]backup.Status{
		"unreadable": {Folder: backup.Folder{Cwd: dir, Root: dir, Config: &cfg}, Target: dir,
			Destination: "example-backups", ReadErr: errors.New("example failure")},
		"not set up here": {Folder: backup.Folder{Cwd: dir, Root: dir, Config: &cfg}, Target: dir,
			Destination: "example-backups", DestinationMissing: true},
	} {
		err := stopBackingUp(context.Background(), m, s)
		if err == nil {
			t.Errorf("%s: want a refusal", name)
		} else if name == "unreadable" && !strings.Contains(err.Error(), "can't read the backups of ~/Projects/website") {
			t.Errorf("%s: error %q doesn't say why", name, err)
		}
		if _, err := os.Stat(filepath.Join(dir, backup.ConfigFileName)); err != nil {
			t.Errorf("%s: the config file is gone: %v", name, err)
		}
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
