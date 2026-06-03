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
	o := execOpts(opts)

	if opts.Force || !sysutil.CommandExists("ghostty") {
		fmt.Fprintln(opts.Stdout, "Installing Ghostty...")

		switch p.PackageManager {
		case "brew":
			if err := sysutil.BrewCaskInstall(ctx, o, "ghostty"); err != nil {
				return err
			}
		case "apt":
			// Pin to a tagged release of the third-party installer rather than
			// HEAD so a push to the upstream default branch can't inject code on
			// our machines. Bump this tag periodically to pick up new releases.
			const ghosttyInstaller = "https://raw.githubusercontent.com/mkasberg/ghostty-ubuntu/1.3.1-0-ppa2/install.sh"
			if err := sysutil.Run(ctx, o, "bash", "-c", "curl -fsSL "+ghosttyInstaller+" | bash"); err != nil {
				return fmt.Errorf("ghostty ubuntu installer: %w", err)
			}
		case "pacman":
			if err := sysutil.SudoRun(ctx, o, "pacman", "-S", "--noconfirm", "ghostty"); err != nil {
				return err
			}
		case "dnf":
			fmt.Fprintln(opts.Stdout, "ghostty has no official Fedora package; build from source: https://ghostty.org/docs/install/build")
			return ErrUnsupportedOS
		default:
			return unsupportedPMError("ghostty", p.PackageManager)
		}
	}

	// Always deploy config (keeps dotfiles in sync)
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
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Ghostty...")

	// Remove deployed config
	if home, err := os.UserHomeDir(); err == nil {
		configPath := filepath.Join(home, ".config", "ghostty", "config")
		if o.DryRun {
			fmt.Fprintf(o.Stdout, "[dry-run] rm %s\n", configPath)
		} else {
			_ = os.Remove(configPath)
		}
	}

	switch p.PackageManager {
	case "brew":
		return sysutil.BrewCaskRemove(ctx, o, "ghostty")
	case "apt":
		return sysutil.RemovePackage(ctx, o, "ghostty")
	case "pacman":
		return sysutil.RemovePackage(ctx, o, "ghostty")
	default:
		return ErrUnsupportedOS
	}
}
