package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// leftoverProfileFixture lays out what machine profiles left behind: a local
// profiles directory with a file in it, and the recorded applied profile.
func leftoverProfileFixture(t *testing.T) (profilesDir, appliedFile string) {
	t.Helper()
	root := t.TempDir()
	profilesDir = filepath.Join(root, "config", "ctdev", "profiles")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "example-node.toml"), []byte("components = [\"git\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	appliedFile = filepath.Join(root, "state", "ctdev", "applied-profile")
	if err := os.MkdirAll(filepath.Dir(appliedFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(appliedFile, []byte("example-node\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return profilesDir, appliedFile
}

func TestRemoveLeftoverProfilesDryRunOnlyReports(t *testing.T) {
	profilesDir, appliedFile := leftoverProfileFixture(t)

	var out bytes.Buffer
	if err := removeLeftoverProfiles(&out, []string{profilesDir, appliedFile}, true); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{profilesDir, appliedFile} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("dry run should leave %s in place: %v", p, err)
		}
		if !strings.Contains(out.String(), "would delete "+p) {
			t.Errorf("dry run should report it would delete %s:\n%s", p, out.String())
		}
	}
}

func TestRemoveLeftoverProfilesDeletesAndReports(t *testing.T) {
	profilesDir, appliedFile := leftoverProfileFixture(t)

	var out bytes.Buffer
	if err := removeLeftoverProfiles(&out, []string{profilesDir, appliedFile}, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{profilesDir, appliedFile} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s should be gone, stat err = %v", p, err)
		}
		if !strings.Contains(out.String(), "deleted "+p) {
			t.Errorf("should report it deleted %s:\n%s", p, out.String())
		}
	}
}

// A machine that never used profiles, or one already cleaned up, hears nothing.
func TestRemoveLeftoverProfilesSilentWhenNothingLeft(t *testing.T) {
	root := t.TempDir()
	paths := []string{filepath.Join(root, "profiles"), filepath.Join(root, "applied-profile")}
	for _, dryRun := range []bool{true, false} {
		var out bytes.Buffer
		if err := removeLeftoverProfiles(&out, paths, dryRun); err != nil {
			t.Fatal(err)
		}
		if out.Len() != 0 {
			t.Errorf("dryRun=%v: expected no output, got:\n%s", dryRun, out.String())
		}
	}
}

func TestLeftoverProfilePaths(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateHome)

	got := leftoverProfilePaths()
	want := []string{
		filepath.Join(home, ".config", "ctdev", "profiles"),
		filepath.Join(stateHome, "ctdev", "applied-profile"),
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("path %d: got %s, want %s", i, got[i], want[i])
		}
	}
}
