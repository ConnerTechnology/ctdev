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
