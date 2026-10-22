//go:build unix

package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// When the folder's parent exists here but can't be written to, such as /home
// for a folder recorded as ~, the zip goes where it would if the parent
// didn't exist: the current directory.
func TestDownloadUnderAllFallsBackWhenTheParentIsReadOnly(t *testing.T) {
	requireRestic(t)
	if os.Geteuid() == 0 {
		t.Skip("root writes to read-only folders")
	}
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	website, cfg := configuredExample(t, m, d)
	snap := onlyBackup(t, m, d, website, &cfg)
	projects := filepath.Dir(website)
	if err := os.Chmod(projects, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(projects, 0o755) })
	cwd := t.TempDir()

	path, err := m.DownloadFromAll(context.Background(), d, cwd, snap)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != cwd {
		t.Errorf("downloaded to %s, want the current directory %s", path, cwd)
	}
}
