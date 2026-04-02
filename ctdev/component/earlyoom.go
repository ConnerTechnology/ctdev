package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func earlyoomInstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS == platform.MacOS {
		return ErrUnsupportedOS
	}
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	if !opts.Force && sysutil.CommandExists("earlyoom") {
		fmt.Fprintln(opts.Stdout, "earlyoom already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing earlyoom...")
	if err := sysutil.InstallPackage(o, "earlyoom"); err != nil {
		return err
	}
	if err := sysutil.ServiceEnable(o, "earlyoom"); err != nil {
		return err
	}
	return sysutil.ServiceStart(o, "earlyoom")
}

func earlyoomUninstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS == platform.MacOS {
		return ErrUnsupportedOS
	}
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing earlyoom...")
	_ = sysutil.ServiceDisable(o, "earlyoom")
	return sysutil.RemovePackage(o, "earlyoom")
}
