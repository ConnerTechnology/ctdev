package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// execOpts converts component ExecOpts to sysutil Opts.
func execOpts(opts ExecOpts) sysutil.Opts {
	return sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
}

func SimplePackageInstaller(name string) func(context.Context, ExecOpts) error {
	return func(ctx context.Context, opts ExecOpts) error {
		o := execOpts(opts)
		if !opts.Force && sysutil.CommandExists(name) {
			fmt.Fprintf(opts.Stdout, "%s already installed\n", name)
			return nil
		}
		fmt.Fprintf(opts.Stdout, "Installing %s...\n", name)
		return sysutil.InstallPackage(o, name)
	}
}

func SimplePackageUninstaller(name string) func(context.Context, ExecOpts) error {
	return func(ctx context.Context, opts ExecOpts) error {
		o := execOpts(opts)
		fmt.Fprintf(opts.Stdout, "Removing %s...\n", name)
		return sysutil.RemovePackage(o, name)
	}
}
