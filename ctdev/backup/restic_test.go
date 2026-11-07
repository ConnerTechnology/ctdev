package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// requireRestic skips a test that needs the restic binary when it isn't
// installed, except in CI, where CI installs it and a missing binary is a
// broken job rather than a reason to skip.
func requireRestic(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("restic"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("restic is not on PATH; CI must install it before the Go tests")
		}
		t.Skip("restic is not installed; install it (ctdev install restic) to run the backup tests")
	}
	// Keep restic's cache out of the developer's home.
	t.Setenv("RESTIC_CACHE_DIR", filepath.Join(t.TempDir(), "cache"))
}

// testMachine is a machine whose config dir and home are both in a temp dir.
func testMachine(t *testing.T, hostname string) *Machine {
	t.Helper()
	home := t.TempDir()
	return &Machine{
		Dir:      filepath.Join(home, ".config", "ctdev", "backup"),
		Home:     home,
		Hostname: hostname,
		Username: "example",
		Opts:     sysutil.Opts{Stdout: &bytes.Buffer{}},
	}
}

// exampleFolder makes ~/Projects/website under m's home with two files.
func exampleFolder(t *testing.T, m *Machine) string {
	t.Helper()
	dir := filepath.Join(m.Home, "Projects", "website")
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"index.html": "<p>example</p>\n", "docs/readme.txt": "example notes\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const exampleMaster = "correct horse example staple"

// template is one destination created for the whole package run. Creating
// one costs restic about 20 seconds of deliberately slow key derivation, so
// each test gets a copy of it instead of its own.
var template struct {
	once  sync.Once
	dir   string
	setup Setup
	err   error
}

func TestMain(m *testing.M) {
	code := m.Run()
	if template.dir != "" {
		os.RemoveAll(template.dir)
	}
	os.Exit(code)
}

// createExample gives m its own copy of a destination created by an
// example-laptop machine, saved on m as if m had created it.
func createExample(t *testing.T, m *Machine) (*Destination, Setup) {
	t.Helper()
	template.once.Do(func() {
		if template.dir, template.err = os.MkdirTemp("", "ctdev-backup-test-"); template.err != nil {
			return
		}
		creator := &Machine{
			Dir: filepath.Join(template.dir, "config"), Home: template.dir,
			Hostname: "example-laptop", Username: "example", Opts: sysutil.Opts{Stdout: &bytes.Buffer{}},
		}
		template.setup, template.err = creator.CreateDestination(context.Background(),
			NewDestination{Name: "example-backups", Address: filepath.Join(template.dir, "repo")}, exampleMaster)
	})
	if template.err != nil {
		t.Fatal(template.err)
	}
	repo := filepath.Join(t.TempDir(), "example-backups")
	if err := os.CopyFS(repo, os.DirFS(template.setup.Destination.Address)); err != nil {
		t.Fatal(err)
	}
	d := *template.setup.Destination
	d.Address, d.m = repo, m
	if err := m.save(&d); err != nil {
		t.Fatal(err)
	}
	s := template.setup
	s.Destination = &d
	s.Kit = strings.Replace(s.Kit, template.setup.Destination.Address, repo, 1)
	return &d, s
}

func TestCreateDestinationBackUpAndFindByFolderID(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, s := createExample(t, m)

	// The kit names the destination and carries the Secret Key, and the
	// Recovery Code is shown apart from it.
	if !strings.Contains(s.Kit, d.Address) || !strings.Contains(s.Kit, "CT1-") {
		t.Errorf("kit is missing the address or the Secret Key:\n%s", s.Kit)
	}
	if s.RecoveryCode == "" || strings.Contains(s.Kit, s.RecoveryCode) {
		t.Error("the Recovery Code must exist and stay out of the kit")
	}
	if strings.Contains(s.Kit, exampleMaster) {
		t.Error("the kit must not hold the master password")
	}

	// The destination is saved owner-only on this machine.
	names, err := m.Destinations()
	if err != nil || len(names) != 1 || names[0] != "example-backups" {
		t.Fatalf("Destinations() = %v, %v", names, err)
	}
	fi, err := os.Stat(filepath.Join(m.Dir, "destinations", "example-backups.env"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("destination file mode %o, want 600", fi.Mode().Perm())
	}

	// Three keys: the account key, the Recovery Code, and this computer.
	computers, err := d.Computers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(computers) != 1 || computers[0].Hostname != "example-laptop" || !computers[0].ThisComputer {
		t.Errorf("Computers() = %+v, want just example-laptop, marked as this computer", computers)
	}

	// Exactly three keys, and the Recovery Code's opens the repository.
	keys, err := d.keys(ctx, s.RecoveryCode)
	if err != nil {
		t.Fatalf("the Recovery Code doesn't open the repository: %v", err)
	}
	var labels []string
	for _, k := range keys {
		labels = append(labels, k.UserName+"@"+k.HostName)
	}
	sort.Strings(labels)
	if want := "ctdev-account@ctdev,ctdev-recovery@ctdev,example@example-laptop"; strings.Join(labels, ",") != want {
		t.Errorf("keys = %v, want %s", labels, want)
	}

	root := exampleFolder(t, m)
	cfg := NewConfig(d.Name)
	if err := m.Configure(ctx, d, root, cfg); err != nil {
		t.Fatal(err)
	}
	snap, err := m.Backup(ctx, d, root, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Files != 3 {
		t.Errorf("backed up %d files, want 3 (the config file is backed up too)", snap.Files)
	}

	snaps, err := d.Snapshots(ctx, m.Ref(root, &cfg))
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 {
		t.Fatalf("found %d backups by folder ID, want 1", len(snaps))
	}
	got := snaps[0]
	if got.FolderID != cfg.ID || got.Path != "~/Projects/website" || got.Host != "example-laptop" {
		t.Errorf("backup tags = folder %q path %q host %q", got.FolderID, got.Path, got.Host)
	}

	// The backup holds the folder's contents with relative paths.
	files, err := d.files(ctx, got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(files, ",") != "/.ctdev-backup.json,/docs,/docs/readme.txt,/index.html" {
		t.Errorf("backup contents = %v, want paths relative to the folder", files)
	}
}

func TestSecondComputerConnectsWithKitAndMasterPassword(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	first := testMachine(t, "example-laptop")
	_, s := createExample(t, first)

	second := testMachine(t, "example-desktop")

	// A wrong master password, or the right one with a different Secret Key,
	// doesn't open the repository, and nothing is saved.
	otherKey := strings.Replace(s.Kit, secretKeyFrom(t, s.Kit), newSecretKey().String(), 1)
	for name, try := range map[string]struct{ kit, master string }{
		"wrong master password": {s.Kit, "not the example password"},
		"wrong Secret Key":      {otherKey, exampleMaster},
	} {
		_, err := second.ConnectDestination(ctx, try.kit, try.master)
		if !errors.Is(err, ErrWrongPassword) {
			t.Errorf("%s: want ErrWrongPassword, got %v", name, err)
		}
		if names, _ := second.Destinations(); len(names) != 0 {
			t.Errorf("%s: saved %v, want nothing", name, names)
		}
	}

	d, err := second.ConnectDestination(ctx, s.Kit, exampleMaster)
	if err != nil {
		t.Fatal(err)
	}
	computers, err := d.Computers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var hosts []string
	for _, c := range computers {
		hosts = append(hosts, c.Hostname)
		if c.ThisComputer != (c.Hostname == "example-desktop") {
			t.Errorf("%s: ThisComputer = %v", c.Hostname, c.ThisComputer)
		}
	}
	if strings.Join(hosts, ",") != "example-desktop,example-laptop" {
		t.Errorf("computers = %v, want both, each with its own key", hosts)
	}

	// The second computer's own key opens the repository on its own.
	root := exampleFolder(t, second)
	cfg := NewConfig(d.Name)
	if _, err := second.Backup(ctx, d, root, &cfg); err != nil {
		t.Fatal(err)
	}

	// The first computer's status shows that backup as the folder's newest,
	// made on the second computer.
	firstRoot := exampleFolder(t, first)
	writeConfig(t, firstRoot, cfg)
	st := first.Status(ctx, firstRoot)
	if st.ReadErr != nil {
		t.Fatal(st.ReadErr)
	}
	if st.Latest.IsZero() || st.LatestHost != "example-desktop" || st.Hostname != "example-laptop" {
		t.Errorf("latest %v on %q, this computer %q; want the desktop's backup", st.Latest, st.LatestHost, st.Hostname)
	}
}

func TestOneOffBackupsJoinTheFolderWhenConfigured(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	root := exampleFolder(t, m)

	// A one-off backup has no folder ID; it's found by path and hostname.
	if _, err := m.Backup(ctx, d, root, nil); err != nil {
		t.Fatal(err)
	}
	oneOffs, err := d.Snapshots(ctx, m.Ref(root, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(oneOffs) != 1 || oneOffs[0].FolderID != "" || oneOffs[0].Path != "~/Projects/website" {
		t.Fatalf("one-off backups = %+v", oneOffs)
	}
	if _, err := os.Stat(filepath.Join(root, ConfigFileName)); !os.IsNotExist(err) {
		t.Error("a one-off backup must not write a config file")
	}

	cfg := NewConfig(d.Name)
	if err := m.Configure(ctx, d, root, cfg); err != nil {
		t.Fatal(err)
	}
	byID, err := d.Snapshots(ctx, m.Ref(root, &cfg))
	if err != nil {
		t.Fatal(err)
	}
	if len(byID) != 1 || byID[0].FolderID != cfg.ID {
		t.Errorf("after Configure, backups by folder ID = %+v, want the one-off", byID)
	}
}

func TestBackupDryRunPrintsTheResticCommandWithoutSecrets(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	root := exampleFolder(t, m)
	cfg := NewConfig(d.Name)

	var out bytes.Buffer
	m.Opts = sysutil.Opts{Stdout: &out, DryRun: true}
	if _, err := m.Backup(ctx, d, root, &cfg); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"[dry-run] restic backup .", "--host example-laptop", "ctdev-folder:" + cfg.ID, "ctdev-path:~/Projects/website"} {
		if !strings.Contains(got, want) {
			t.Errorf("dry-run output %q is missing %q", got, want)
		}
	}
	if strings.Contains(got, d.computerKey) {
		t.Error("dry-run output must not show the computer key")
	}
	if snaps, _ := d.Snapshots(ctx, m.Ref(root, &cfg)); len(snaps) != 0 {
		t.Errorf("a dry run made %d backups", len(snaps))
	}
}

// When saving fails after the computer key was added, the key is removed, so
// no key is left that nobody holds the password to.
func TestConnectRemovesItsKeyWhenSavingFails(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	first := testMachine(t, "example-laptop")
	d, s := createExample(t, first)

	second := testMachine(t, "example-desktop")
	// A file where the config dir should be makes the save fail.
	if err := os.MkdirAll(filepath.Dir(second.Dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second.Dir, []byte("example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := second.ConnectDestination(ctx, s.Kit, exampleMaster); err == nil {
		t.Fatal("want the save to fail")
	}
	computers, err := d.Computers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(computers) != 1 || computers[0].Hostname != "example-laptop" {
		t.Errorf("computers = %+v, want only example-laptop", computers)
	}
}

func TestCreatingOverAnExistingRepositoryPointsAtConnect(t *testing.T) {
	requireRestic(t)
	first := testMachine(t, "example-laptop")
	d, _ := createExample(t, first)
	second := testMachine(t, "example-desktop")
	_, err := second.CreateDestination(context.Background(), NewDestination{Name: "example-backups", Address: d.Address}, exampleMaster)
	if err == nil || !strings.Contains(err.Error(), "connect to it with its Emergency Kit") {
		t.Errorf("want a pointer to connecting instead, got %v", err)
	}
	if names, _ := second.Destinations(); len(names) != 0 {
		t.Errorf("saved %v, want nothing", names)
	}
}

func secretKeyFrom(t *testing.T, kit string) string {
	t.Helper()
	k, err := ParseKit(kit)
	if err != nil {
		t.Fatal(err)
	}
	return k.SecretKey
}
