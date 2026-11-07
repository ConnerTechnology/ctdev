package component

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
)

// fakeResticHost points HOME at a temp dir and PATH at a temp "apt" bin
// directory holding an executable restic, the way /usr/bin/restic sits on a
// host with apt's package. It returns that restic's path and where ours goes.
func fakeResticHost(t *testing.T) (aptRestic, ours string) {
	t.Helper()
	if platform.Detect().OS != platform.Linux {
		t.Skip("ctdev's own restic binary is Linux-only")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	aptBin := filepath.Join(root, "usr", "bin")
	if err := os.MkdirAll(aptBin, 0o755); err != nil {
		t.Fatal(err)
	}
	aptRestic = filepath.Join(aptBin, "restic")
	if err := os.WriteFile(aptRestic, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	ours = filepath.Join(home, ".local", "bin", "restic")
	t.Setenv("HOME", home)
	t.Setenv("PATH", filepath.Dir(ours)+string(os.PathListSeparator)+aptBin)
	return aptRestic, ours
}

// apt's restic on PATH does not count as installed on Linux, so a plain
// (unforced) install still puts the pinned release in place and points at the
// old one. Once ours exists, it counts.
func TestResticAptCopyIsNotInstalledOnLinux(t *testing.T) {
	aptRestic, ours := fakeResticHost(t)
	c := FindByName("restic")
	if c == nil {
		t.Fatal("restic is not in the registry")
	}
	if c.IsInstalled() {
		t.Fatal("apt's restic on PATH counted as installed")
	}

	var buf bytes.Buffer
	if err := resticInstall(context.Background(), ExecOpts{DryRun: true, Stdout: &buf, Stderr: &buf}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "already installed") || !strings.Contains(out, "download restic/restic v"+resticVersion) {
		t.Errorf("unforced install skipped the download:\n%s", out)
	}
	if !strings.Contains(out, "Another restic is at "+aptRestic) || !strings.Contains(out, "sudo apt remove restic") {
		t.Errorf("install did not point at apt's restic:\n%s", out)
	}

	if err := os.MkdirAll(filepath.Dir(ours), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ours, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !c.IsInstalled() {
		t.Error("ctdev's own restic in ~/.local/bin did not count as installed")
	}
}

// Uninstall removes ours and, when apt's is still on PATH, says so.
func TestResticUninstallHintsAtAptCopy(t *testing.T) {
	aptRestic, ours := fakeResticHost(t)
	if err := os.MkdirAll(filepath.Dir(ours), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ours, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := resticUninstall(context.Background(), ExecOpts{Stdout: &buf, Stderr: &buf}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ours); err == nil {
		t.Error("uninstall left ~/.local/bin/restic in place")
	}
	if _, err := os.Stat(aptRestic); err != nil {
		t.Error("uninstall touched apt's restic")
	}
	if out := buf.String(); !strings.Contains(out, "Another restic is at "+aptRestic) {
		t.Errorf("uninstall did not point at apt's restic:\n%s", out)
	}
}

// dryRunLines returns the "[dry-run]" lines of a dry run's output.
func dryRunLines(out string) []string {
	var dry []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "[dry-run]") {
			dry = append(dry, line)
		}
	}
	return dry
}

// restic is only its binary: a dry run of install (forced) or uninstall shows
// one line and nothing else — no systemd units, no scripts, no system config
// directory. On Linux that line is the pinned upstream release going into
// ~/.local/bin (apt's restic is too old); on macOS it is Homebrew.
func TestResticDryRunIsOnlyTheBinary(t *testing.T) {
	type step struct {
		name string
		run  func(context.Context, ExecOpts) error
		want []string // substrings the one dry-run line must contain
	}
	var steps []step
	switch platform.Detect().OS {
	case platform.Linux:
		steps = []step{
			{"install", resticInstall, []string{"download restic/restic v" + resticVersion, "/.local/bin/restic"}},
			{"uninstall", resticUninstall, []string{"rm -f", "/.local/bin/restic"}},
		}
	case platform.MacOS:
		steps = []step{
			{"install", resticInstall, []string{"brew install restic"}},
			{"uninstall", resticUninstall, []string{"brew uninstall restic"}},
		}
	default:
		t.Skip("restic installs on Linux and macOS only")
	}
	for _, s := range steps {
		var buf bytes.Buffer
		opts := ExecOpts{DryRun: true, Force: true, Stdout: &buf, Stderr: &buf}
		if err := s.run(context.Background(), opts); err != nil {
			t.Fatalf("%s dry run: %v", s.name, err)
		}
		dry := dryRunLines(buf.String())
		if len(dry) != 1 {
			t.Errorf("%s: want one dry-run line, got:\n%s", s.name, buf.String())
			continue
		}
		for _, w := range s.want {
			if !strings.Contains(dry[0], w) {
				t.Errorf("%s: dry-run line %q lacks %q", s.name, dry[0], w)
			}
		}
	}
}
