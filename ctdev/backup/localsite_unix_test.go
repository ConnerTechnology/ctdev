//go:build unix

package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectLocal(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	full := filepath.Join(dir, "full")
	link := filepath.Join(dir, "link")
	file := filepath.Join(dir, "file")
	for _, d := range []string{empty, full} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(full, "notes.txt"), []byte("example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(empty, link); err != nil {
		t.Fatal(err)
	}
	cases := map[string]siteState{
		filepath.Join(dir, "new"): siteMissing,
		empty:                     siteEmptyDir,
		full:                      siteFull,
		link:                      siteUnknown, // never followed
		file:                      siteUnknown,
	}
	if os.Geteuid() != 0 {
		locked := filepath.Join(dir, "locked")
		if err := os.Mkdir(locked, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(locked, 0o755) })
		cases[filepath.Join(locked, "repo")] = siteUnknown // can't tell: inconclusive
	}
	for path, want := range cases {
		if got := inspectLocal(path).state; got != want {
			t.Errorf("inspectLocal(%s) = %v, want %v", filepath.Base(path), got, want)
		}
	}
}

// Folders restic made on the way to a new repository go too, but only those.
func TestFailedCreateRemovesTheFoldersItMade(t *testing.T) {
	m := testMachine(t, "example-laptop")
	fakeRestic(t, m)
	existing := filepath.Join(m.Home, "media")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(existing, "usb", "ctdev", "backups")
	if _, err := m.CreateDestination(context.Background(), NewDestination{Name: "example-backups", Address: repo}, exampleMaster); err == nil {
		t.Fatal("want a failure")
	}
	if _, err := os.Stat(filepath.Join(existing, "usb")); !os.IsNotExist(err) {
		t.Errorf("%s/usb, made for the repository, is still there", existing)
	}
	if _, err := os.Stat(existing); err != nil {
		t.Errorf("%s was there before and must stay: %v", existing, err)
	}
}

// A failed creation at a symlink leaves the link and what it points at.
func TestFailedCreateLeavesASymlinkAlone(t *testing.T) {
	m := testMachine(t, "example-laptop")
	fakeRestic(t, m)
	target := filepath.Join(m.Home, "usb")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(m.Home, "backups")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	_, err := m.CreateDestination(context.Background(), NewDestination{Name: "example-backups", Address: link}, exampleMaster)
	if err == nil || !strings.Contains(err.Error(), "was left") {
		t.Errorf("want the repository left with a note, got %v", err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the symlink is gone or changed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "config")); err != nil {
		t.Errorf("what restic made through the symlink was removed: %v", err)
	}
}
