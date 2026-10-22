//go:build unix

package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// downloadStandin sets m up with a stand-in restic whose dump writes
// "example zip" to its --target. With takeName, it meanwhile also saves
// someone else's file under the zip's final name.
func downloadStandin(t *testing.T, m *Machine, takeName bool) (d *Destination, root string, snap Snapshot, final string) {
	t.Helper()
	dir := t.TempDir()
	m.Restic = filepath.Join(dir, "restic")
	root = exampleFolder(t, m)
	snap = Snapshot{ID: "0123456789abcdef", Time: time.Date(2026, 10, 22, 14, 15, 0, 0, time.UTC)}
	final = filepath.Join(filepath.Dir(root), "website-20261022T141500Z.zip")
	other := ""
	if takeName {
		other = `echo "example file someone else saved" > "` + final + `"`
	}
	script := `#!/bin/sh
case "$1" in
version) echo "restic 0.19.1 compiled with go1.26.4 on linux/amd64" ;;
dump)
  while [ "$1" != "--target" ]; do shift; done
  echo "example zip" > "$2"
  ` + other + ` ;;
esac
`
	if err := os.WriteFile(m.Restic, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return &Destination{Name: "example-backups", Address: filepath.Join(dir, "repo"), m: m}, root, snap, final
}

// noHardLinks stands in for a filesystem without hard links, such as an
// exFAT USB drive, for the rest of the test.
func noHardLinks(t *testing.T) {
	t.Helper()
	saved := linkFile
	linkFile = func(oldname, newname string) error {
		return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: syscall.EPERM}
	}
	t.Cleanup(func() { linkFile = saved })
}

func noPartials(t *testing.T, dir string) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".partial") {
			t.Errorf("left %s behind", e.Name())
		}
	}
}

// A file that appears under the zip's name while restic writes the zip is
// left alone: the finished zip is linked into place, which fails rather than
// overwrite.
func TestDownloadNeverOverwritesAFileThatAppearsMeanwhile(t *testing.T) {
	m := testMachine(t, "example-laptop")
	d, root, snap, final := downloadStandin(t, m, true)

	if _, err := m.Download(context.Background(), d, root, snap); err == nil {
		t.Fatal("want an error: the zip's name was taken while it downloaded")
	}
	if got, _ := os.ReadFile(final); string(got) != "example file someone else saved\n" {
		t.Errorf("the file under the zip's name now holds %q", got)
	}
	noPartials(t, filepath.Dir(root))
}

// Where hard links aren't supported, the zip is copied into a file created
// only if the name is free.
func TestDownloadWorksWithoutHardLinks(t *testing.T) {
	noHardLinks(t)
	m := testMachine(t, "example-laptop")
	d, root, snap, final := downloadStandin(t, m, false)

	path, err := m.Download(context.Background(), d, root, snap)
	if err != nil {
		t.Fatal(err)
	}
	if path != final {
		t.Errorf("downloaded to %s, want %s", path, final)
	}
	if got, _ := os.ReadFile(final); string(got) != "example zip\n" {
		t.Errorf("the zip holds %q", got)
	}
	noPartials(t, filepath.Dir(root))
}

func TestDownloadWithoutHardLinksNeverOverwritesEither(t *testing.T) {
	noHardLinks(t)
	m := testMachine(t, "example-laptop")
	d, root, snap, final := downloadStandin(t, m, true)

	if _, err := m.Download(context.Background(), d, root, snap); err == nil {
		t.Fatal("want an error: the zip's name was taken while it downloaded")
	}
	if got, _ := os.ReadFile(final); string(got) != "example file someone else saved\n" {
		t.Errorf("the file under the zip's name now holds %q", got)
	}
	noPartials(t, filepath.Dir(root))
}

// The weekly data check holds the repository's exclusive lock, so reading a
// folder's backups waits for it rather than failing (CTD-87). A stand-in
// restic logs each call's arguments.
func TestReadingBackupsWaitsForTheLock(t *testing.T) {
	m := testMachine(t, "example-laptop")
	dir := t.TempDir()
	log := filepath.Join(dir, "restic.log")
	m.Restic = filepath.Join(dir, "restic")
	script := `#!/bin/sh
echo "$*" >> "` + log + `"
case "$1" in
version) echo "restic 0.19.1 compiled with go1.26.4 on linux/amd64" ;;
snapshots) echo "[]" ;;
stats) echo '{"total_size": 0}' ;;
esac
`
	if err := os.WriteFile(m.Restic, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	d := &Destination{Name: "example-backups", Address: filepath.Join(dir, "repo"), m: m}
	ref := Ref{FolderID: "00000000-0000-4000-8000-000000000000"}
	if _, err := d.Snapshots(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Stored(context.Background(), []Snapshot{{ID: "0123456789abcdef"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.HasPrefix(line, "version") {
			continue
		}
		if !strings.Contains(line, "--retry-lock 30m") {
			t.Errorf("restic %s doesn't wait for the lock", line)
		}
	}
}
