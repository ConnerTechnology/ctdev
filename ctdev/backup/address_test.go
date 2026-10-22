package backup

import (
	"context"
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
