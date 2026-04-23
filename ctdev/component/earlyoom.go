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
	o := execOpts(opts)
	if !opts.Force && sysutil.CommandExists("earlyoom") {
		fmt.Fprintln(opts.Stdout, "earlyoom already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing earlyoom...")
	if err := sysutil.InstallPackage(ctx, o, "earlyoom"); err != nil {
		return err
	}
	if err := sysutil.ServiceEnable(ctx, o, "earlyoom"); err != nil {
		return err
	}
	return sysutil.ServiceStart(ctx, o, "earlyoom")
}

func earlyoomUninstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS == platform.MacOS {
		return ErrUnsupportedOS
	}
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing earlyoom...")
	_ = sysutil.ServiceDisable(ctx, o, "earlyoom")
	return sysutil.RemovePackage(ctx, o, "earlyoom")
}
