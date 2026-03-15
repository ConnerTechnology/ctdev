package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func shellcheckInstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	if !opts.Force && sysutil.CommandExists("shellcheck") {
		fmt.Fprintln(opts.Stdout, "shellcheck already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing shellcheck...")
	return sysutil.InstallPackage(o, "shellcheck")
}

func shellcheckUninstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing shellcheck...")
	return sysutil.RemovePackage(o, "shellcheck")
}
