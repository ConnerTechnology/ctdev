package component

import (
	"bytes"
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHerdrDesktopUninstall_DryRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	var out bytes.Buffer
	if err := herdrDesktopUninstall(context.Background(), ExecOpts{DryRun: true, Stdout: &out}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	want := []string{"brew uninstall --cask herdr-app"}
	if runtime.GOOS == "linux" {
		want = []string{
			filepath.Join(home, ".local/share/herdr-app") + " ",
			filepath.Join(home, ".local/bin/herdr-app"),
			filepath.Join(home, ".local/share/applications/herdr-app.desktop"),
			filepath.Join(home, ".local/share/icons/hicolor/scalable/apps/herdr-app.svg"),
		}
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("expected %q in dry-run output; got:\n%s", w, got)
		}
	}
	for _, keep := range []string{".config/herdr-app", "herdr-core"} {
		if strings.Contains(got, keep) {
			t.Errorf("uninstall must leave %s alone; got:\n%s", keep, got)
		}
	}
}

func TestHerdrDesktopFollowsXDGDataHome(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	if got := herdrDesktopLinuxPaths()[0]; got != filepath.Join(data, "herdr-app") {
		t.Errorf("app dir = %q, want it under XDG_DATA_HOME %q", got, data)
	}
}

func TestHerdrDesktopRegistered(t *testing.T) {
	c := componentByName["herdr-desktop"]
	if c == nil {
		t.Fatal("herdr-desktop is not in the registry")
	}
	if c.GoInstall == nil || c.GoUninstall == nil {
		t.Error("herdr-desktop needs both an installer and an uninstaller")
	}
	if c.Root != RootNever {
		t.Errorf("herdr-desktop installs under $HOME or through a cask and must declare RootNever, got %v", c.Root)
	}
}
