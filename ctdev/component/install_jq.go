package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func jqInstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	if !opts.Force && sysutil.CommandExists("jq") {
		fmt.Fprintln(opts.Stdout, "jq already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing jq...")
	return sysutil.InstallPackage(o, "jq")
}

func jqUninstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing jq...")
	return sysutil.RemovePackage(o, "jq")
}
