package component

import (
	"context"
	"fmt"
	"os"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func chromeInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && sysutil.CommandExists("google-chrome") {
		fmt.Fprintln(opts.Stdout, "Chrome already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing Chrome...")

	switch p.PackageManager {
	case "brew":
		return sysutil.BrewCaskInstall(ctx, o, "google-chrome")
	case "apt":
		if p.Arch != "amd64" {
			return fmt.Errorf("chrome .deb only available on amd64 (got %s)", p.Arch)
		}
		if o.DryRun {
			fmt.Fprintln(o.Stdout, "[dry-run] download and install google-chrome-stable .deb")
			return nil
		}
		tmp, err := os.CreateTemp("", "chrome-*.deb")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		tmp.Close()

		if err := sysutil.DownloadFile(ctx, "https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb", tmp.Name()); err != nil {
			return fmt.Errorf("download chrome: %w", err)
		}
		return installDebWithDepFix(ctx, o, tmp.Name(), "google-chrome-stable")
	case "dnf":
		if err := sysutil.SudoRun(ctx, o, "dnf", "install", "-y", "fedora-workstation-repositories"); err != nil {
			return fmt.Errorf("install fedora-workstation-repositories: %w", err)
		}
		if err := sysutil.SudoRun(ctx, o, "dnf", "config-manager", "--set-enabled", "google-chrome"); err != nil {
			return fmt.Errorf("enable google-chrome repo: %w", err)
		}
		return sysutil.SudoRun(ctx, o, "dnf", "install", "-y", "google-chrome-stable")
	default:
		return unsupportedPMError("chrome", p.PackageManager)
	}
}

func chromeUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Chrome...")

	switch p.PackageManager {
	case "brew":
		return sysutil.BrewCaskRemove(ctx, o, "google-chrome")
	case "apt":
		return sysutil.RemovePackage(ctx, o, "google-chrome-stable")
	case "dnf":
		return sysutil.RemovePackage(ctx, o, "google-chrome-stable")
	default:
		return ErrUnsupportedOS
	}
}
