//go:build unix

package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A file that appears under the zip's name while restic writes the zip is
// left alone: the finished zip is linked into place, which fails rather than
// overwrite. The stand-in restic writes the zip and, meanwhile, the file.
func TestDownloadNeverOverwritesAFileThatAppearsMeanwhile(t *testing.T) {
	m := testMachine(t, "example-laptop")
	dir := t.TempDir()
	m.Restic = filepath.Join(dir, "restic")
	root := exampleFolder(t, m)
	snap := Snapshot{ID: "0123456789abcdef", Time: time.Date(2026, 10, 22, 14, 15, 0, 0, time.UTC)}
	final := filepath.Join(filepath.Dir(root), "website-20261022T141500Z.zip")
	script := `#!/bin/sh
case "$1" in
version) echo "restic 0.19.1 compiled with go1.26.4 on linux/amd64" ;;
dump)
  while [ "$1" != "--target" ]; do shift; done
  echo "example zip" > "$2"
  echo "example file someone else saved" > "` + final + `" ;;
esac
`
	if err := os.WriteFile(m.Restic, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	d := &Destination{Name: "example-backups", Address: filepath.Join(dir, "repo"), m: m}

	if _, err := m.Download(context.Background(), d, root, snap); err == nil {
		t.Fatal("want an error: the zip's name was taken while it downloaded")
	}
	if got, _ := os.ReadFile(final); string(got) != "example file someone else saved\n" {
		t.Errorf("the file under the zip's name now holds %q", got)
	}
	entries, _ := os.ReadDir(filepath.Dir(root))
	for _, e := range entries {
		if strings.Contains(e.Name(), ".partial") {
			t.Errorf("left %s behind", e.Name())
		}
	}
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
