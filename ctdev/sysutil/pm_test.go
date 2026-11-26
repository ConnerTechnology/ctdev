package sysutil

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
)

func TestInstallPackageDryRun(t *testing.T) {
	var buf bytes.Buffer
	o := Opts{Stdout: &buf, DryRun: true}
	err := InstallPackage(context.Background(), o, "testpkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "[dry-run]") || !strings.Contains(out, "testpkg") {
		t.Errorf("expected dry-run preview naming testpkg, got %q", out)
	}
}

func TestRemovePackageDryRun(t *testing.T) {
	var buf bytes.Buffer
	o := Opts{Stdout: &buf, DryRun: true}
	err := RemovePackage(context.Background(), o, "testpkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "[dry-run]") || !strings.Contains(out, "testpkg") {
		t.Errorf("expected dry-run preview naming testpkg, got %q", out)
	}
}

func TestIsPackageInstalled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stubs require unix")
	}
	pm := platform.Detect().PackageManager
	if pm != "apt" && pm != "brew" {
		t.Skipf("IsPackageInstalled stubbed for apt/brew, not %q", pm)
	}

	bin := t.TempDir()
	switch pm {
	case "apt":
		script := "#!/bin/sh\nif [ \"$3\" = installed-pkg ]; then echo 'installed 1.0.0'; exit 0; fi\nexit 1\n"
		if err := os.WriteFile(filepath.Join(bin, "dpkg-query"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	case "brew":
		script := "#!/bin/sh\nif [ \"$2\" = installed-pkg ]; then exit 0; fi\nexit 1\n"
		if err := os.WriteFile(filepath.Join(bin, "brew"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)

	if !IsPackageInstalled("installed-pkg") {
		t.Error("expected IsPackageInstalled(installed-pkg) == true")
	}
	if IsPackageInstalled("missing-pkg") {
		t.Error("expected IsPackageInstalled(missing-pkg) == false")
	}
}

// A dry run puts nothing on disk, so it must not conclude the install failed and
// go off reinstalling.
func TestBrewCaskInstallVerified_DryRunSkipsVerification(t *testing.T) {
	var buf bytes.Buffer
	o := Opts{Stdout: &buf, DryRun: true}

	calls := 0
	err := BrewCaskInstallVerified(context.Background(), o, "tailscale", func() bool {
		calls++
		return false
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 0 {
		t.Errorf("verification ran %d times during a dry run, want 0", calls)
	}
	if got := buf.String(); !strings.Contains(got, "brew install --cask tailscale") {
		t.Errorf("expected the cask install to be previewed, got %q", got)
	}
	if strings.Contains(buf.String(), "reinstall") {
		t.Errorf("dry run should not attempt a repair, got %q", buf.String())
	}
}

// The bug this guards: brew records a cask as installed before the artifact is
// in place, so an interrupted .pkg step leaves later installs no-opping with
// exit 0 while nothing is on the machine. Success is the artifact, not the exit
// code.
func TestBrewCaskInstallVerified_RepairsWhenArtifactMissing(t *testing.T) {
	var buf bytes.Buffer
	o := Opts{Stdout: &buf, DryRun: true}

	// Report missing on the first check and present after the repair, the shape
	// of a successful reinstall. DryRun keeps this from touching the machine.
	checks := 0
	err := brewCaskVerifyAndRepair(context.Background(), o, "tailscale", func() bool {
		checks++
		return checks > 1
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checks != 2 {
		t.Errorf("verified %d times, want 2 (once after install, once after repair)", checks)
	}
	if got := buf.String(); !strings.Contains(got, "brew reinstall --cask tailscale") {
		t.Errorf("expected a reinstall to repair the missing artifact, got %q", got)
	}
}

// When even the repair leaves nothing behind, the step has to fail loudly rather
// than report a success the user can't use.
func TestBrewCaskInstallVerified_FailsWhenRepairDoesNotHelp(t *testing.T) {
	var buf bytes.Buffer
	o := Opts{Stdout: &buf, DryRun: true}

	err := brewCaskVerifyAndRepair(context.Background(), o, "tailscale", func() bool { return false })
	if err == nil {
		t.Fatal("expected an error when the cask is still missing after a reinstall")
	}
	if !strings.Contains(err.Error(), "still not present") {
		t.Errorf("error should say the software is absent, got %v", err)
	}
}
