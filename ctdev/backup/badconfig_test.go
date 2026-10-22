package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeBadConfig puts a config file with an unknown schedule at dir.
func writeBadConfig(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"id": "0b6c2a4e-1f3d-4c5b-9a8e-7d6f5e4c3b2a", "destination": "example-backups", "schedule": "monthly", "keep": {}}`
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A config file that can't be read still marks its folder: the status is the
// folder's, with the reason, never "not backed up" for the directory you're in.
func TestFindAndStatusWithAnUnreadableConfig(t *testing.T) {
	m := testMachine(t, "example-laptop")
	root := filepath.Join(m.Home, "Projects", "website")
	writeBadConfig(t, root)
	docs := filepath.Join(root, "docs")

	f, err := Find(docs)
	if err != nil {
		t.Fatal(err)
	}
	if f.Root != root || f.Config != nil || f.ConfigErr == nil || !strings.Contains(f.ConfigErr.Error(), "monthly") {
		t.Errorf("Find = root %q, config %v, err %v; want the folder with the reason", f.Root, f.Config, f.ConfigErr)
	}

	s := m.Status(context.Background(), docs)
	if s.Target != root || s.ConfigErr == nil {
		t.Errorf("Status target %q, config err %v; want %q with the reason", s.Target, s.ConfigErr, root)
	}
}

// Configure and Backup never act on an empty target, which filepath.Abs
// would quietly turn into the current directory, nor overwrite a config file
// that can't be read.
func TestEmptyTargetsAndUnreadableConfigsAreRefused(t *testing.T) {
	m := testMachine(t, "example-laptop")
	d := &Destination{Name: "example-backups", Address: filepath.Join(t.TempDir(), "repo"), m: m, computerKey: "example"}
	ctx := context.Background()
	if _, err := m.Backup(ctx, d, "", nil); err == nil {
		t.Error("Backup with an empty folder: want an error")
	}
	if err := m.Configure(ctx, d, "", NewConfig(d.Name)); err == nil {
		t.Error("Configure with an empty folder: want an error")
	}

	root := filepath.Join(m.Home, "Projects", "website")
	writeBadConfig(t, root)
	err := m.Configure(ctx, d, root, NewConfig(d.Name))
	if err == nil || !strings.Contains(err.Error(), "can't be read") {
		t.Errorf("Configure over an unreadable config: want a refusal, got %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, ConfigFileName)); !strings.Contains(string(data), "monthly") {
		t.Error("the unreadable config file was overwritten")
	}
}
