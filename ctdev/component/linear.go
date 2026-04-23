package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func linearInstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := execOpts(opts)
	if !opts.Force && alreadyInstalled("linear") {
		fmt.Fprintln(opts.Stdout, "Linear already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing Linear...")
	return sysutil.BrewCaskInstall(ctx, o, "linear-linear")
}

func linearUninstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Linear...")
	return sysutil.BrewCaskRemove(ctx, o, "linear-linear")
}
