package sysutil

import (
	"fmt"
	"os/exec"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

// InstallPackage installs packages using the detected system package manager.
func InstallPackage(o Opts, names ...string) error {
	pm := platform.Detect().PackageManager
	switch pm {
	case "apt":
		return SudoRun(o, "apt-get", append([]string{"install", "-y", "-qq"}, names...)...)
	case "brew":
		return Run(o, "brew", append([]string{"install"}, names...)...)
	case "dnf":
		return SudoRun(o, "dnf", append([]string{"install", "-y"}, names...)...)
	case "pacman":
		return SudoRun(o, "pacman", append([]string{"-S", "--noconfirm"}, names...)...)
	default:
		return fmt.Errorf("unsupported package manager: %s", pm)
	}
}

// RemovePackage removes packages using the detected system package manager.
func RemovePackage(o Opts, names ...string) error {
	pm := platform.Detect().PackageManager
	switch pm {
	case "apt":
		return SudoRun(o, "apt-get", append([]string{"remove", "-y"}, names...)...)
	case "brew":
		return Run(o, "brew", append([]string{"uninstall"}, names...)...)
	case "dnf":
		return SudoRun(o, "dnf", append([]string{"remove", "-y"}, names...)...)
	case "pacman":
		return SudoRun(o, "pacman", append([]string{"-R", "--noconfirm"}, names...)...)
	default:
		return fmt.Errorf("unsupported package manager: %s", pm)
	}
}

// BrewCaskInstall installs a Homebrew cask (macOS GUI apps).
func BrewCaskInstall(o Opts, name string) error {
	return Run(o, "brew", "install", "--cask", name)
}

// BrewCaskRemove removes a Homebrew cask.
func BrewCaskRemove(o Opts, name string) error {
	return Run(o, "brew", "uninstall", "--cask", name)
}

// IsPackageInstalled checks if a package is installed via the system package manager.
func IsPackageInstalled(name string) bool {
	pm := platform.Detect().PackageManager
	switch pm {
	case "apt":
		return exec.Command("dpkg", "-s", name).Run() == nil
	case "brew":
		return exec.Command("brew", "list", name).Run() == nil
	case "dnf":
		return exec.Command("rpm", "-q", name).Run() == nil
	default:
		return false
	}
}
