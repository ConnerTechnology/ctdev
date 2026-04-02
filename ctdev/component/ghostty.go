package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func ghosttyInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

	if !opts.Force && sysutil.CommandExists("ghostty") {
		fmt.Fprintln(opts.Stdout, "Ghostty already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing Ghostty...")

	switch p.PackageManager {
	case "brew":
		if err := sysutil.BrewCaskInstall(o, "ghostty"); err != nil {
			return err
		}
	case "apt":
		if err := sysutil.Run(o, "bash", "-c", "curl -fsSL https://raw.githubusercontent.com/mkasberg/ghostty-ubuntu/HEAD/install.sh | bash"); err != nil {
			return fmt.Errorf("ghostty ubuntu installer: %w", err)
		}
	case "pacman":
		if err := sysutil.SudoRun(o, "pacman", "-S", "--noconfirm", "ghostty"); err != nil {
			return err
		}
	case "dnf":
		return fmt.Errorf("ghostty does not have an official Fedora package; build from source: https://ghostty.org/docs/install/build")
	default:
		return fmt.Errorf("ghostty install not supported for package manager: %s", p.PackageManager)
	}

	// Deploy config from embedded configs
	if err := deployGhosttyConfig(o); err != nil {
		fmt.Fprintf(opts.Stdout, "warning: could not deploy ghostty config: %v\n", err)
	}

	return nil
}

func deployGhosttyConfig(o sysutil.Opts) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dst := filepath.Join(home, ".config", "ghostty", "config")

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] deploy ghostty config → %s\n", dst)
		return nil
	}
	return sysutil.DeployFileFromFS(Configs, "configs/ghostty/config", dst)
}

func ghosttyUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing Ghostty...")

	switch p.PackageManager {
	case "brew":
		return sysutil.BrewCaskRemove(o, "ghostty")
	case "apt":
		return sysutil.RemovePackage(o, "ghostty")
	case "pacman":
		return sysutil.RemovePackage(o, "ghostty")
	default:
		return ErrUnsupportedOS
	}
}
