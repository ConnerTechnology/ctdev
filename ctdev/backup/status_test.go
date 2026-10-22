package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStatusOfAConfiguredFolder(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	root := exampleFolder(t, m)
	cfg := NewConfig(d.Name)
	if err := m.Configure(ctx, d, root, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Backup(ctx, d, root, &cfg); err != nil {
		t.Fatal(err)
	}

	s := m.Status(ctx, filepath.Join(root, "docs"))
	if s.ReadErr != nil {
		t.Fatal(s.ReadErr)
	}
	if s.Root != root || s.Target != root || s.Config == nil {
		t.Errorf("Status from a subfolder: root %q target %q", s.Root, s.Target)
	}
	if s.Destination != "example-backups" || s.Backend != "local path" {
		t.Errorf("destination %q (%s)", s.Destination, s.Backend)
	}
	if s.Files != 3 || s.Size == 0 {
		t.Errorf("size now = %d bytes, %d files; want 3 files", s.Size, s.Files)
	}
	if s.Last == nil || s.Last.Failed() || s.Last.LastSuccess.IsZero() {
		t.Fatalf("last run = %+v, want a success", s.Last)
	}
	if !s.Scheduled || s.NextDue.Sub(s.Last.LastSuccess) != 24*time.Hour {
		t.Errorf("scheduled %v, next due %v after last success %v", s.Scheduled, s.NextDue, s.Last.LastSuccess)
	}
	if s.Backups != 1 || s.Oldest.IsZero() || s.Stored == 0 {
		t.Errorf("backups = %d, oldest %v, stored %d", s.Backups, s.Oldest, s.Stored)
	}
	if !s.HasBackups() || s.Failed() {
		t.Errorf("HasBackups %v, Failed %v", s.HasBackups(), s.Failed())
	}
}

// A folder whose config file came from elsewhere (a git clone) isn't on this
// computer's list until Configure adds it.
func TestStatusOfACloneIsNotScheduledHere(t *testing.T) {
	requireRestic(t)
	m := testMachine(t, "example-laptop")
	createExample(t, m)
	root := exampleFolder(t, m)
	writeConfig(t, root, NewConfig("example-backups"))

	s := m.Status(context.Background(), root)
	if s.ReadErr != nil {
		t.Fatal(s.ReadErr)
	}
	if s.Config == nil || s.Scheduled {
		t.Errorf("config %v, scheduled %v; want configured but not scheduled", s.Config, s.Scheduled)
	}
	if s.HasBackups() {
		t.Error("a fresh clone has no backups yet")
	}
}

// A clone whose destination this computer isn't set up on still shows its
// config, with the destination marked missing.
func TestStatusWithADestinationNotSetUpHere(t *testing.T) {
	m := testMachine(t, "example-laptop")
	root := exampleFolder(t, m)
	writeConfig(t, root, NewConfig("example-backups"))

	s := m.Status(context.Background(), root)
	if s.Destination != "example-backups" || !s.DestinationMissing || s.ReadErr != nil {
		t.Errorf("destination %q, missing %v, err %v", s.Destination, s.DestinationMissing, s.ReadErr)
	}
}

// When this computer's destinations can't be listed, that's a read error,
// not a destination that isn't set up here.
func TestStatusWhenDestinationsCantBeListed(t *testing.T) {
	m := testMachine(t, "example-laptop")
	root := exampleFolder(t, m)
	writeConfig(t, root, NewConfig("example-backups"))
	if err := os.MkdirAll(m.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// A file where the destinations folder should be can't be listed.
	if err := os.WriteFile(filepath.Join(m.Dir, "destinations"), []byte("example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := m.Status(context.Background(), root)
	if s.ReadErr == nil || s.DestinationMissing {
		t.Errorf("read err %v, missing %v; want a read error", s.ReadErr, s.DestinationMissing)
	}
}

func TestStatusOfAnUnconfiguredFolderCountsItsOneOffs(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	root := exampleFolder(t, m)

	s := m.Status(ctx, root)
	if s.Config != nil || s.Root != "" || s.Target != root || s.Destination != "" || s.HasBackups() {
		t.Errorf("no destination, no config: got %+v", s)
	}

	d, _ := createExample(t, m)
	if _, err := m.Backup(ctx, d, root, nil); err != nil {
		t.Fatal(err)
	}
	s = m.Status(ctx, root)
	if s.ReadErr != nil {
		t.Fatal(s.ReadErr)
	}
	if s.Destination != "example-backups" || s.Backups != 1 || !s.HasBackups() || s.Scheduled {
		t.Errorf("after a one-off: destination %q, backups %d, scheduled %v", s.Destination, s.Backups, s.Scheduled)
	}
}

// An unreadable folder inside is counted, not a reason to stop counting.
func TestSizeNowSkipsWhatItCantRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	m := testMachine(t, "example-laptop")
	root := exampleFolder(t, m)
	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "secret.txt"), []byte("example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })

	s := m.Status(context.Background(), root)
	if s.Files != 2 || s.Unreadable != 1 {
		t.Errorf("files %d, unreadable %d; want the 2 readable files and 1 unreadable folder", s.Files, s.Unreadable)
	}
	if s.ReadErr != nil {
		t.Errorf("an unreadable subfolder isn't a read error: %v", s.ReadErr)
	}
}

func TestAFailedBackupIsRecorded(t *testing.T) {
	m := testMachine(t, "example-laptop")
	root := exampleFolder(t, m)
	d := &Destination{Name: "example-backups", Address: filepath.Join(t.TempDir(), "no-repo"), m: m, computerKey: "example"}
	if _, err := m.Backup(context.Background(), d, root, nil); err == nil {
		t.Fatal("a backup to a missing repository must fail")
	}
	s := m.Status(context.Background(), root)
	if s.Last == nil || !s.Failed() {
		t.Errorf("last run = %+v, want a failure", s.Last)
	}
}
