package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func claudeDesktopInstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	if !opts.Force && sysutil.CommandExists("claude") {
		fmt.Fprintln(opts.Stdout, "Claude Desktop already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing Claude Desktop...")
	return sysutil.BrewCaskInstall(o, "claude")
}

func claudeDesktopUninstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing Claude Desktop...")
	return sysutil.BrewCaskRemove(o, "claude")
}
