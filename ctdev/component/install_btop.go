package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func btopInstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	if !opts.Force && sysutil.CommandExists("btop") {
		fmt.Fprintln(opts.Stdout, "btop already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing btop...")
	return sysutil.InstallPackage(o, "btop")
}

func btopUninstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing btop...")
	return sysutil.RemovePackage(o, "btop")
}
