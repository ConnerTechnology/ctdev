package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/checklist"
)

func TestSelfInstallScriptURL_UsesReleaseTag(t *testing.T) {
	got := selfInstallScriptURL("v17.2.0")
	want := "https://raw.githubusercontent.com/ConnerTechnology/ctdev/v17.2.0/install.sh"
	if got != want {
		t.Errorf("url = %q, want %q", got, want)
	}
	if strings.Contains(got, "/main/") {
		t.Errorf("self-update must never fetch install.sh from main: %q", got)
	}
}

func TestCtdevUpdateStep_FetchesTagAndInstallsInPlace(t *testing.T) {
	steps := buildUpdateSteps([]checklist.UpdateItem{
		{Name: "ctdev", Source: "ctdev", CurrentVer: "17.1.0", NewVer: "17.2.0"},
	})
	if len(steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(steps))
	}

	var out bytes.Buffer
	if err := steps[0].run(context.Background(), sysutil.Opts{DryRun: true, Stdout: &out}); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := out.String()

	if !strings.Contains(got, "/ConnerTechnology/ctdev/v17.2.0/install.sh") {
		t.Errorf("dry run should fetch the v17.2.0 install.sh, got %q", got)
	}
	if strings.Contains(got, "/main/") {
		t.Errorf("dry run mentions a main URL: %q", got)
	}
	if !strings.Contains(got, "CTDEV_VERSION=v17.2.0") {
		t.Errorf("dry run should pin CTDEV_VERSION=v17.2.0, got %q", got)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if want := "INSTALL_DIR=" + filepath.Dir(exe); !strings.Contains(got, want) {
		t.Errorf("dry run should install next to the running binary (%s), got %q", want, got)
	}
}

func TestUpdateSelf_RejectsOddTag(t *testing.T) {
	for _, ver := range []string{"main", "17.2", "17.2.0/../../main", ""} {
		var out bytes.Buffer
		if err := updateSelf(context.Background(), sysutil.Opts{DryRun: true, Stdout: &out}, ver); err == nil {
			t.Errorf("updateSelf(%q) = nil, want an error", ver)
		}
	}
}

func TestSelfInstallArgs_SurviveSudo(t *testing.T) {
	got := selfInstallArgs("/usr/local/bin", "v17.2.0", "/tmp/install.sh")
	want := []string{"env", "INSTALL_DIR=/usr/local/bin", "CTDEV_VERSION=v17.2.0", "bash", "/tmp/install.sh"}
	if !slices.Equal(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
}

// withSelfInstallDir points the self-update at dir for the rest of the test.
func withSelfInstallDir(t *testing.T, dir string) {
	t.Helper()
	orig := selfInstallDir
	selfInstallDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { selfInstallDir = orig })
}

// readOnlyDir is a directory the current user can't create files in, like
// /usr/local/bin for a non-root user.
func readOnlyDir(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root can write to any directory")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	return dir
}

func TestUpdateNeedsRoot_CtdevInUnwritableDir(t *testing.T) {
	withSelfInstallDir(t, readOnlyDir(t))
	items := []checklist.UpdateItem{{Source: "ctdev", Name: "ctdev"}}
	if !updateNeedsRoot(items) {
		t.Error("updateNeedsRoot() = false for a ctdev binary in an unwritable dir, want true")
	}
}

func TestUpdateNeedsRoot_CtdevInWritableDir(t *testing.T) {
	withSelfInstallDir(t, t.TempDir())
	items := []checklist.UpdateItem{{Source: "ctdev", Name: "ctdev"}}
	if updateNeedsRoot(items) {
		t.Error("updateNeedsRoot() = true for a ctdev binary in a writable dir, want false")
	}
}

func TestUpdateSelf_DryRunSaysAsRoot(t *testing.T) {
	dir := readOnlyDir(t)
	withSelfInstallDir(t, dir)
	var out bytes.Buffer
	if err := updateSelf(context.Background(), sysutil.Opts{DryRun: true, Stdout: &out}, "17.2.0"); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "(as root)") || !strings.Contains(got, "INSTALL_DIR="+dir) {
		t.Errorf("dry run = %q, want it to say (as root) and INSTALL_DIR=%s", got, dir)
	}
}
