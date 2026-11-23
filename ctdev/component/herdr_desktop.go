package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// Herdr's desktop app (github.com/herdrdev/herdr-app) is a window for
// terminals and coding agents. Its terminals run in herdr-core, a separate
// program from the `herdr` component, which the app's installer sets up and
// the app keeps current. Every install updates itself, so it gets no
// `ctdev update` scanner.
const (
	// The cask is Apple Silicon only and marked auto_updates.
	herdrDesktopCask       = "herdr-app"
	herdrDesktopMacApp     = "/Applications/Herdr.app"
	herdrDesktopInstallURL = "https://releases.herdr.dev/app/install.sh"
)

// herdrDesktopDataDir is where install.sh puts the Linux app and its launcher
// entry; it honors XDG_DATA_HOME the same way.
func herdrDesktopDataDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share")
}

// herdrDesktopLinuxPaths lists what install.sh lays down on Linux: the app,
// its `herdr-app` link, the launcher entry and the icon.
func herdrDesktopLinuxPaths() []string {
	data := herdrDesktopDataDir()
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(data, "herdr-app"),
		filepath.Join(home, ".local", "bin", "herdr-app"),
		filepath.Join(data, "applications", "herdr-app.desktop"),
		filepath.Join(data, "icons", "hicolor", "scalable", "apps", "herdr-app.svg"),
	}
}

func herdrDesktopInstalled() bool {
	var path string
	switch runtime.GOOS {
	case "darwin":
		path = herdrDesktopMacApp
	case "linux":
		path = filepath.Join(herdrDesktopDataDir(), "herdr-app", "herdr-app")
	default:
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func herdrDesktopInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	switch p.OS {
	case platform.MacOS:
		if p.Arch != "arm64" {
			return fmt.Errorf("Herdr desktop has no Intel Mac build: %w", ErrUnsupportedOS)
		}
	case platform.Linux:
		if p.Arch != "amd64" && p.Arch != "arm64" {
			return fmt.Errorf("Herdr desktop only available for amd64/arm64 on Linux (got %s): %w", p.Arch, ErrUnsupportedOS)
		}
	default:
		return ErrUnsupportedOS
	}

	if !opts.Force && alreadyInstalled("herdr-desktop") {
		fmt.Fprintln(opts.Stdout, "Herdr desktop already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing Herdr desktop...")

	if p.OS == platform.MacOS {
		return sysutil.BrewCaskInstallVerified(ctx, o, herdrDesktopCask, herdrDesktopInstalled)
	}
	// install.sh checks the tarball against the SHA-256 in herdr's latest.json,
	// installs under $HOME with a launcher entry, and adds herdr-core when none
	// is there. That is the layout the app's own updater expects, so a download
	// of our own would gain nothing.
	if err := sysutil.Run(ctx, o, "sh", "-c", "curl -fsSL "+herdrDesktopInstallURL+" | sh"); err != nil {
		return fmt.Errorf("herdr desktop installer: %w", err)
	}
	// A failed curl leaves the inner sh reading nothing, and it exits 0, so
	// the pipeline's status alone can't tell a failed install from one that
	// worked.
	if !o.DryRun && !herdrDesktopInstalled() {
		return fmt.Errorf("herdr desktop installer finished but %s is missing", herdrDesktopLinuxPaths()[0])
	}
	return nil
}

// herdrDesktopUninstall removes the app and its launcher entry. It leaves
// ~/.config/herdr-app, the user's settings, layouts and mods, and herdr-core,
// which keeps the terminals and may serve other screens.
func herdrDesktopUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Herdr desktop...")

	switch p.OS {
	case platform.MacOS:
		return sysutil.BrewCaskRemove(ctx, o, herdrDesktopCask)
	case platform.Linux:
		return sysutil.Run(ctx, o, "rm", append([]string{"-rf"}, herdrDesktopLinuxPaths()...)...)
	}
	return ErrUnsupportedOS
}
