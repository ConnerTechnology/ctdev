package component

import (
	"context"
	"fmt"
	"os"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func cleanmymacInstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := execOpts(opts)
	if !opts.Force {
		if _, err := os.Stat("/Applications/CleanMyMac.app"); err == nil {
			fmt.Fprintln(opts.Stdout, "CleanMyMac already installed")
			return nil
		}
		if _, err := os.Stat("/Applications/CleanMyMac X.app"); err == nil {
			fmt.Fprintln(opts.Stdout, "CleanMyMac already installed")
			return nil
		}
	}
	fmt.Fprintln(opts.Stdout, "Installing CleanMyMac...")
	return sysutil.BrewCaskInstall(o, "cleanmymac")
}

func cleanmymacUninstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing CleanMyMac...")
	return sysutil.BrewCaskRemove(o, "cleanmymac")
}
