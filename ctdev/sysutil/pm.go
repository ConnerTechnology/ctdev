package sysutil

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

// ErrUnsupportedPM marks package operations on a package manager we don't
// drive (dnf, pacman, ...). The component executor maps it to a Skipped
// result, the same way component-level ErrUnsupportedOS is handled — without
// it, plain components like git/tmux report as *failed* on Fedora/Arch while
// everything else politely skips.
var ErrUnsupportedPM = errors.New("unsupported package manager")

// InstallPackage installs packages using the detected system package manager.
func InstallPackage(ctx context.Context, o Opts, names ...string) error {
	pm := platform.Detect().PackageManager
	switch pm {
	case "apt":
		return SudoRun(ctx, o, "apt-get", append([]string{"install", "-y", "-qq"}, names...)...)
	case "brew":
		return Run(ctx, o, "brew", append([]string{"install"}, names...)...)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedPM, pm)
	}
}

// RemovePackage removes packages using the detected system package manager.
func RemovePackage(ctx context.Context, o Opts, names ...string) error {
	pm := platform.Detect().PackageManager
	switch pm {
	case "apt":
		return SudoRun(ctx, o, "apt-get", append([]string{"remove", "-y"}, names...)...)
	case "brew":
		return Run(ctx, o, "brew", append([]string{"uninstall"}, names...)...)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedPM, pm)
	}
}

// BrewCaskInstall installs a Homebrew cask (macOS GUI apps).
func BrewCaskInstall(ctx context.Context, o Opts, name string) error {
	return Run(ctx, o, "brew", "install", "--cask", name)
}

// BrewCaskInstallVerified installs a cask and then confirms the software is
// actually on disk, retrying once through `brew reinstall` when it isn't.
//
// Homebrew marks a cask installed as soon as its Caskroom entry exists, and that
// entry is written independently of the artifact being put in place. If the
// artifact step is interrupted — a `.pkg` cask stalling on a sudo prompt is how
// this happens in practice — brew goes on believing the job is done: every later
// `brew install --cask` no-ops and exits 0 while nothing is installed. Trusting
// that exit code made ctdev report a successful install of software that had
// never been put on the machine.
//
// installed is the component's own detection predicate, so "is it really there?"
// stays defined in one place per component rather than being restated here.
func BrewCaskInstallVerified(ctx context.Context, o Opts, cask string, installed func() bool) error {
	if err := BrewCaskInstall(ctx, o, cask); err != nil {
		return err
	}
	// A dry run never puts anything in place, so there is nothing to verify and
	// nothing to repair — checking would just preview a spurious reinstall.
	if o.DryRun {
		return nil
	}
	return brewCaskVerifyAndRepair(ctx, o, cask, installed)
}

// brewCaskVerifyAndRepair is the post-install half of BrewCaskInstallVerified,
// split out so the repair path can be tested without the dry-run guard above
// short-circuiting it.
func brewCaskVerifyAndRepair(ctx context.Context, o Opts, cask string, installed func() bool) error {
	if installed() {
		return nil
	}
	fmt.Fprintf(o.Stdout, "%s: Homebrew reported success but the files are missing — reinstalling...\n", cask)
	if err := Run(ctx, o, "brew", "reinstall", "--cask", cask); err != nil {
		return fmt.Errorf("reinstall %s: %w", cask, err)
	}
	if installed() {
		return nil
	}
	return fmt.Errorf("%s: installed without error but is still not present "+
		"(a system extension may be waiting for approval in System Settings → Privacy & Security)", cask)
}

// BrewCaskRemove removes a Homebrew cask.
func BrewCaskRemove(ctx context.Context, o Opts, name string) error {
	return Run(ctx, o, "brew", "uninstall", "--cask", name)
}

// IsPackageInstalled checks if a package is installed via the system package manager.
func IsPackageInstalled(name string) bool {
	pm := platform.Detect().PackageManager
	switch pm {
	case "apt":
		return exec.Command("dpkg", "-s", name).Run() == nil
	case "brew":
		return exec.Command("brew", "list", name).Run() == nil
	default:
		return false
	}
}
