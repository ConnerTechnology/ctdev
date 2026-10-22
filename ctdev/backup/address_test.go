package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalAddressesAreExpandedAndMadeAbsolute(t *testing.T) {
	m := testMachine(t, "example-laptop")
	cwd := t.TempDir()
	t.Chdir(cwd)
	for in, want := range map[string]string{
		"~":                         m.Home,
		"~/Backups":                 filepath.Join(m.Home, "Backups"),
		"backups":                   filepath.Join(cwd, "backups"),
		"/media/example/usb/../usb": "/media/example/usb",
		"b2:example-bucket:website": "b2:example-bucket:website",
		SFTPAddress("backup@nas.example.invalid", "/srv/backups"): "sftp:backup@nas.example.invalid:/srv/backups",
	} {
		got, err := m.CheckAddress(in, "")
		if err != nil || got != want {
			t.Errorf("CheckAddress(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

// Under --all, which writes nothing into the current directory, a new local
// destination's path must say where it is: absolute or from ~/. It also
// mustn't be inside the current directory.
func TestANewDestinationUnderAllNeedsAPathThatSaysWhereItIs(t *testing.T) {
	m := testMachine(t, "example-laptop")
	cwd := filepath.Join(m.Home, "Downloads")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwd)
	for _, addr := range []string{"backups", "./backups", "../backups", "~backups"} {
		_, err := m.CheckNewAddressFromAll(addr, cwd)
		if err == nil || !strings.Contains(err.Error(), "absolute path or one starting with ~/") {
			t.Errorf("%q: want a refusal asking for an absolute or ~/ path, got %v", addr, err)
		}
	}
	if _, err := m.CheckNewAddressFromAll(filepath.Join(cwd, "backups"), cwd); err == nil || !strings.Contains(err.Error(), "inside") {
		t.Errorf("a path inside the current directory: want an inside error, got %v", err)
	}
	for in, want := range map[string]string{
		"~/Backups":                 filepath.Join(m.Home, "Backups"),
		"/media/example/usb":        "/media/example/usb",
		"b2:example-bucket:website": "b2:example-bucket:website",
	} {
		if got, err := m.CheckNewAddressFromAll(in, cwd); err != nil || got != want {
			t.Errorf("%q = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestALocalRepositoryCantBeInsideABackedUpFolder(t *testing.T) {
	m := testMachine(t, "example-laptop")
	website := filepath.Join(m.Home, "Projects", "website")
	writeConfig(t, website, NewConfig("example-backups"))
	notes := filepath.Join(m.Home, "notes")

	for _, tc := range []struct{ addr, folder string }{
		{"~/Projects/website/backups", ""}, // inside a backed-up folder
		{"~/Projects/website", ""},         // the backed-up folder itself
		{"~/notes/.backups", notes},        // inside the folder being set up
	} {
		if _, err := m.CheckAddress(tc.addr, tc.folder); err == nil || !strings.Contains(err.Error(), "inside") {
			t.Errorf("CheckAddress(%q, %q): want an inside error, got %v", tc.addr, tc.folder, err)
		}
	}
	if _, err := m.CheckAddress("~/Backups", notes); err != nil {
		t.Errorf("a repository beside the folder: %v", err)
	}

	// Creating refuses before anything runs, so this needs no restic.
	_, err := m.CreateDestination(context.Background(), NewDestination{Name: "example-backups", Address: "~/Projects/website/backups"}, exampleMaster)
	if err == nil || !strings.Contains(err.Error(), "inside") {
		t.Errorf("CreateDestination: want an inside error, got %v", err)
	}
}

// A new destination goes in a new or empty folder. One holding a restic
// repository passes, so creating there can point at connecting instead.
func TestANewDestinationNeedsAnEmptyFolder(t *testing.T) {
	m := testMachine(t, "example-laptop")
	full := filepath.Join(m.Home, "Documents")
	repo := filepath.Join(m.Home, "old-backups")
	stray := filepath.Join(m.Home, "settings")
	for dir, file := range map[string]string{full: "notes.txt", repo: "config", stray: "config"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, file), []byte("example\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A repository has keys/ and data/ beside its config.
	for _, d := range []string{"keys", "data"} {
		if err := os.Mkdir(filepath.Join(repo, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A lone file called config doesn't make a folder a repository.
	if _, err := m.CheckNewAddress("~/settings", ""); err == nil || !strings.Contains(err.Error(), "isn't a restic repository") {
		t.Errorf("a folder with just a config file: want a refusal, got %v", err)
	}
	if _, err := m.CheckNewAddress("~/Documents", ""); err == nil || !strings.Contains(err.Error(), "isn't a restic repository") {
		t.Errorf("a folder of files: want a refusal, got %v", err)
	}
	_, err := m.CreateDestination(context.Background(), NewDestination{Name: "example-backups", Address: full}, exampleMaster)
	if err == nil || !strings.Contains(err.Error(), "isn't a restic repository") {
		t.Errorf("CreateDestination in a folder of files: want a refusal, got %v", err)
	}
	if entries, _ := os.ReadDir(full); len(entries) != 1 {
		t.Errorf("%s was changed: %d entries", full, len(entries))
	}
	if _, err := m.CheckNewAddress("~/old-backups", ""); err != nil {
		t.Errorf("a restic repository: %v", err)
	}
	if _, err := m.CheckNewAddress("~/Documents/notes.txt", ""); err == nil {
		t.Error("a file: want a refusal")
	}
	if _, err := m.CheckNewAddress("~/new-backups", ""); err != nil {
		t.Errorf("a new folder: %v", err)
	}
}

// A one-off backup or a Configure of the folder holding the repository would
// back the repository up into itself.
func TestBackupAndConfigureRefuseAFolderHoldingTheRepository(t *testing.T) {
	m := testMachine(t, "example-laptop")
	root := exampleFolder(t, m)
	d := &Destination{Name: "example-backups", Address: filepath.Join(root, "backups"), m: m, computerKey: "example"}
	if _, err := m.Backup(context.Background(), d, root, nil); err == nil || !strings.Contains(err.Error(), "inside") {
		t.Errorf("Backup: want an inside error, got %v", err)
	}
	if err := m.Configure(context.Background(), d, root, NewConfig(d.Name)); err == nil || !strings.Contains(err.Error(), "inside") {
		t.Errorf("Configure: want an inside error, got %v", err)
	}
}
