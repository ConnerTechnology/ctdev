package component

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeBinDir creates a tempdir with a single executable `gs` that prints body
// to stdout. Returns the dir path, suitable for prepending to $PATH.
func fakeBinDir(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"" + body + "\"\n"
	path := filepath.Join(dir, "gs")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return dir
}

func TestGitSpiceUninstall_LeavesGhostscriptAlone(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("linux-specific codepath")
	}
	ghostscriptBin := fakeBinDir(t, "GPL Ghostscript 10.03.1 (2024-05-02)")
	t.Setenv("PATH", ghostscriptBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	var out bytes.Buffer
	if err := gitSpiceUninstall(context.Background(), ExecOpts{DryRun: true, Stdout: &out}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "git-spice not detected") {
		t.Errorf("expected leave-alone message; got:\n%s", got)
	}
	if strings.Contains(got, "rm -f /usr/local/bin/gs") {
		t.Errorf("should not attempt rm when gs is Ghostscript; got:\n%s", got)
	}
}

func TestGitSpiceUninstall_RemovesActualGitSpice(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("linux-specific codepath")
	}
	gitSpiceBin := fakeBinDir(t, "git-spice 1.0.0")
	t.Setenv("PATH", gitSpiceBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	var out bytes.Buffer
	if err := gitSpiceUninstall(context.Background(), ExecOpts{DryRun: true, Stdout: &out}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "rm -f /usr/local/bin/gs") {
		t.Errorf("expected rm dry-run line; got:\n%s", got)
	}
	if strings.Contains(got, "not detected") {
		t.Errorf("should not log 'not detected' for real git-spice; got:\n%s", got)
	}
}
