//go:build unix

package backup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// loggingRestic puts a restic in front of the real one that logs each call's
// arguments, a line per call, to the returned file.
func loggingRestic(t *testing.T, m *Machine) (log string) {
	t.Helper()
	real, err := exec.LookPath("restic")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	log = filepath.Join(dir, "restic.log")
	m.Restic = filepath.Join(dir, "restic")
	script := "#!/bin/sh\necho \"$*\" >> \"" + log + "\"\nexec \"" + real + "\" \"$@\"\n"
	if err := os.WriteFile(m.Restic, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return log
}

// statsCalls counts the restic stats runs in a loggingRestic log.
func statsCalls(t *testing.T, log string) int {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "stats ") {
			n++
		}
	}
	return n
}

// The overview reads keys and backups only. The space stored, which has
// restic read every backup, is StoredAll's own call.
func TestOverviewRunsNoStats(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	website, cfg := configuredExample(t, m, d)
	backUp(t, m, d, website, &cfg)
	log := loggingRestic(t, m)

	o, err := d.Overview(ctx)
	if err != nil || o.FolderCount != 1 {
		t.Fatalf("Overview() = %d folders, %v", o.FolderCount, err)
	}
	if n := statsCalls(t, log); n != 0 {
		t.Errorf("Overview ran restic stats %d times, want none", n)
	}
	if stored, err := d.StoredAll(ctx); err != nil || stored <= 0 {
		t.Fatalf("StoredAll() = %d, %v", stored, err)
	}
	if n := statsCalls(t, log); n != 1 {
		t.Errorf("StoredAll ran restic stats %d times, want once", n)
	}
}

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
