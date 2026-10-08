package component

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBetterbirdMajor(t *testing.T) {
	cases := []struct {
		file, want string
		wantErr    bool
	}{
		{file: "betterbird-153.4.0esr-bb10.en-US.linux-x86_64.tar.xz", want: "153"},
		{file: "betterbird-140.13.0esr-bb25.en-US.linux-x86_64.tar.xz", want: "140"},
		{file: "thunderbird-153.4.0esr.en-US.linux-x86_64.tar.xz", wantErr: true},
		{file: "betterbird-esr.en-US.linux-x86_64.tar.xz", wantErr: true},
		{file: "betterbird-", wantErr: true},
	}
	for _, c := range cases {
		got, err := betterbirdMajor(c.file)
		if (err != nil) != c.wantErr {
			t.Errorf("betterbirdMajor(%q) err = %v, wantErr %v", c.file, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("betterbirdMajor(%q) = %q, want %q", c.file, got, c.want)
		}
	}
}

func TestPgrepMatched(t *testing.T) {
	// Real *exec.ExitErrors, since that is what sysutil.Run wraps.
	exitWith := func(code string) error {
		err := exec.Command("sh", "-c", "exit "+code).Run()
		return fmt.Errorf("pgrep: %w", err)
	}
	cases := []struct {
		name        string
		err         error
		wantRunning bool
		wantErr     bool
	}{
		{name: "exit 0 is running", err: nil, wantRunning: true},
		{name: "exit 1 is not running", err: exitWith("1")},
		{name: "exit 2 is an error", err: exitWith("2"), wantErr: true},
		{name: "exit 3 is an error", err: exitWith("3"), wantErr: true},
		{name: "missing pgrep is an error", err: fmt.Errorf("pgrep: %w", exec.ErrNotFound), wantErr: true},
	}
	for _, c := range cases {
		running, err := pgrepMatched(c.err)
		if running != c.wantRunning || (err != nil) != c.wantErr {
			t.Errorf("%s: got (%v, %v), want running=%v wantErr=%v", c.name, running, err, c.wantRunning, c.wantErr)
		}
	}
}

func TestBetterbirdCheckTree(t *testing.T) {
	good := filepath.Join(t.TempDir(), "betterbird")
	if err := os.MkdirAll(good, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "betterbird"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := betterbirdCheckTree(good); err != nil {
		t.Errorf("tree with binary: %v", err)
	}

	empty := filepath.Join(t.TempDir(), "betterbird")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	err := betterbirdCheckTree(empty)
	if err == nil || !strings.Contains(err.Error(), filepath.Join(empty, "betterbird")) {
		t.Errorf("tree without binary: got %v, want an error naming the expected path", err)
	}

	// A directory where the binary should be is not a binary.
	dirBin := filepath.Join(t.TempDir(), "betterbird")
	if err := os.MkdirAll(filepath.Join(dirBin, "betterbird"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := betterbirdCheckTree(dirBin); err == nil {
		t.Error("tree with a directory in place of the binary: got nil error")
	}
}

func TestBetterbirdDesktopEntryPointsAtInstallDir(t *testing.T) {
	// The tarball has no `betterbird` on PATH, so the launcher must use the
	// absolute path or the menu entry does nothing.
	data, err := Configs.ReadFile("configs/betterbird/eu.betterbird.Betterbird.desktop")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Exec=" + betterbirdInstallDir + "/betterbird %u",
		"Icon=" + betterbirdInstallDir + "/chrome/icons/default/default256.png",
	} {
		if !strings.Contains(string(data), "\n"+want+"\n") {
			t.Errorf("desktop entry missing line %q", want)
		}
	}
}

func TestBetterbirdCheckArch(t *testing.T) {
	if err := betterbirdCheckArch("amd64"); err != nil {
		t.Errorf("amd64: unexpected error %v", err)
	}
	err := betterbirdCheckArch("arm64")
	if !errors.Is(err, ErrUnsupportedOS) {
		t.Errorf("arm64: err = %v, want ErrUnsupportedOS so the executor reports Skipped", err)
	}
	if err != nil && !strings.Contains(err.Error(), "arm64") {
		t.Errorf("arm64: err %q does not name the arch", err)
	}
}

func TestBetterbirdAfterFailedAside(t *testing.T) {
	cases := []struct {
		name                     string
		installExists, oldExists bool
		want                     asideRecovery
	}{
		{name: "rename never happened", installExists: true, oldExists: false, want: asideClean},
		{name: "rename landed before the error", installExists: false, oldExists: true, want: asideRestore},
		{name: "both present", installExists: true, oldExists: true, want: asideKeep},
		{name: "neither present", installExists: false, oldExists: false, want: asideClean},
	}
	for _, c := range cases {
		if got := betterbirdAfterFailedAside(c.installExists, c.oldExists); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPathMayExist(t *testing.T) {
	dir := t.TempDir()
	if !pathMayExist(dir) {
		t.Errorf("pathMayExist(%q) = false for an existing dir", dir)
	}
	if pathMayExist(filepath.Join(dir, "missing")) {
		t.Errorf("pathMayExist reported a missing path as present")
	}
}
