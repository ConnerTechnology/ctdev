//go:build unix

package backup

import (
	"context"
	"errors"
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

// A path through a symlink into the folder is still inside it.
func TestARepositoryReachedThroughASymlinkIsStillInside(t *testing.T) {
	m := testMachine(t, "example-laptop")
	root := exampleFolder(t, m)
	link := filepath.Join(m.Home, "site")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CheckAddress("~/site/.backups", root); err == nil || !strings.Contains(err.Error(), "inside") {
		t.Errorf("CheckAddress through a symlink: want an inside error, got %v", err)
	}
	d := &Destination{Name: "example-backups", Address: filepath.Join(link, "backups"), m: m, computerKey: "example"}
	if _, err := m.Backup(context.Background(), d, root, nil); err == nil || !strings.Contains(err.Error(), "inside") {
		t.Errorf("Backup: want an inside error, got %v", err)
	}
}

// A path that was missing when inspected but is a symlink by cleanup time
// is left alone, link and target both.
func TestCleanupLeavesASymlinkThatAppearedSince(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backups")
	site := inspectLocal(path)
	if site.state != siteMissing {
		t.Fatalf("state = %v, want missing", site.state)
	}
	target := filepath.Join(dir, "elsewhere")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "notes.txt"), []byte("example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	err := site.cleanup(errors.New("example failure"))
	if err == nil || !strings.Contains(err.Error(), "was left") {
		t.Errorf("want a note that it was left, got %v", err)
	}
	if fi, err := os.Lstat(path); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the symlink is gone or changed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "notes.txt")); err != nil {
		t.Errorf("the link's target lost its file: %v", err)
	}
}

// makeLayout puts restic's repository layout at dir, plus a stray file that
// arrived during creation.
func makeLayout(t *testing.T, dir string) {
	t.Helper()
	for _, d := range []string{"data/00", "index", "keys", "locks", "snapshots"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"config", "keys/example", "stray.txt"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("example\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// Cleanup removes restic's layout and nothing else: a file that arrived in
// the folder during creation stays, and so do the folders holding it.
func TestCleanupRemovesOnlyResticsLayout(t *testing.T) {
	for name, existed := range map[string]bool{"an empty folder": true, "a new path": false} {
		base := t.TempDir()
		path := filepath.Join(base, "made", "backups")
		if existed {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		site := inspectLocal(path)
		makeLayout(t, path)
		if err := site.cleanup(errors.New("example failure")); err == nil {
			t.Fatalf("%s: cleanup must still report the failure", name)
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			t.Fatalf("%s: %s is gone though it holds a stray file: %v", name, path, err)
		}
		if len(entries) != 1 || entries[0].Name() != "stray.txt" {
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Errorf("%s: left %v, want only stray.txt", name, names)
		}
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
