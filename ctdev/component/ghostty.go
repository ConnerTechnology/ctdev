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
			// Ghostty has no official Ubuntu packages; this community installer
			// picks the right .deb for the distro/version. It's a third-party
			// personal repo, so pin by commit (tags can be force-pushed) and
			// verify the script's hash before executing — neither a moved tag
			// nor a compromised upstream can change what runs here. Bump both
			// constants together when updating (commit for tag 1.3.1-0-ppa2).
			const (
				ghosttyInstallerURL    = "https://raw.githubusercontent.com/mkasberg/ghostty-ubuntu/3b2899e3bfb7f21c4ccdb4c038b042c52be18dce/install.sh"
				ghosttyInstallerSHA256 = "7517776f6d862ec523e627840af4806e13385302f653ae9f7a86aa6d5af1cae5"
			)
			if o.DryRun {
				fmt.Fprintf(o.Stdout, "[dry-run] download, verify, and run %s\n", ghosttyInstallerURL)
				break
			}
			tmp, err := os.CreateTemp("", "ghostty-install-*.sh")
			if err != nil {
				return err
			}
			defer os.Remove(tmp.Name())
			tmp.Close()
			if err := sysutil.DownloadFile(ctx, ghosttyInstallerURL, tmp.Name()); err != nil {
				return fmt.Errorf("download ghostty installer: %w", err)
			}
			if err := sysutil.VerifyChecksum(tmp.Name(), ghosttyInstallerSHA256); err != nil {
				return fmt.Errorf("ghostty installer: %w", err)
			}
			if err := sysutil.Run(ctx, o, "bash", tmp.Name()); err != nil {
				return fmt.Errorf("ghostty ubuntu installer: %w", err)
			}
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
	default:
		return ErrUnsupportedOS
	}
}
