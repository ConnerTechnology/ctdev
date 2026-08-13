package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func logiOptionsInstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := execOpts(opts)
	if !opts.Force && alreadyInstalled("logi-options") {
		fmt.Fprintln(opts.Stdout, "Logi Options+ already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing Logi Options+...")
	return sysutil.BrewCaskInstallVerified(ctx, o, "logi-options+",
		func() bool { return alreadyInstalled("logi-options") })
}

func logiOptionsUninstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Logi Options+...")
	return sysutil.BrewCaskRemove(ctx, o, "logi-options+")
}
