package component

import (
	"context"
	"fmt"
	"os"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func slackInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

	if !opts.Force && sysutil.CommandExists("slack") {
		fmt.Fprintln(opts.Stdout, "Slack already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing Slack...")

	if p.OS == platform.MacOS {
		return sysutil.BrewCaskInstall(o, "slack")
	}

	if p.PackageManager == "apt" {
		if p.Arch != "amd64" {
			return fmt.Errorf("slack not available on %s architecture", p.Arch)
		}
		if o.DryRun {
			fmt.Fprintln(o.Stdout, "[dry-run] download and install slack-desktop .deb")
			return nil
		}
		tmp, err := os.CreateTemp("", "slack-*.deb")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		tmp.Close()

		if err := sysutil.DownloadFile("https://downloads.slack-edge.com/releases/linux/slack-desktop-amd64.deb", tmp.Name()); err != nil {
			return fmt.Errorf("download slack: %w", err)
		}
		if err := sysutil.SudoRun(o, "dpkg", "-i", tmp.Name()); err != nil {
			if fixErr := sysutil.SudoRun(o, "apt-get", "install", "-f", "-y"); fixErr != nil {
				return fmt.Errorf("dpkg failed and apt-get fix failed: %w", fixErr)
			}
		}
		return nil
	}

	return ErrUnsupportedOS
}

func slackUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing Slack...")

	if p.OS == platform.MacOS {
		return sysutil.BrewCaskRemove(o, "slack")
	}
	if p.PackageManager == "apt" {
		return sysutil.RemovePackage(o, "slack-desktop")
	}
	return ErrUnsupportedOS
}
