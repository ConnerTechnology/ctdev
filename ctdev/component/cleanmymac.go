package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func cleanmymacInstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := execOpts(opts)
	if !opts.Force && alreadyInstalled("cleanmymac") {
		fmt.Fprintln(opts.Stdout, "CleanMyMac already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing CleanMyMac...")
	return sysutil.BrewCaskInstallVerified(ctx, o, "cleanmymac",
		func() bool { return alreadyInstalled("cleanmymac") })
}

func cleanmymacUninstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing CleanMyMac...")
	return sysutil.BrewCaskRemove(ctx, o, "cleanmymac")
}
