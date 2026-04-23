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
	o := execOpts(opts)

	if !opts.Force && sysutil.CommandExists("slack") {
		fmt.Fprintln(opts.Stdout, "Slack already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing Slack...")

	if p.OS == platform.MacOS {
		return sysutil.BrewCaskInstall(ctx, o, "slack")
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
		return installDebWithDepFix(ctx, o, tmp.Name(), "slack-desktop")
	}

	return ErrUnsupportedOS
}

func slackUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Slack...")

	if p.OS == platform.MacOS {
		return sysutil.BrewCaskRemove(ctx, o, "slack")
	}
	if p.PackageManager == "apt" {
		return sysutil.RemovePackage(ctx, o, "slack-desktop")
	}
	return ErrUnsupportedOS
}
