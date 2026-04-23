package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func solaarInstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS == platform.MacOS {
		return ErrUnsupportedOS
	}
	o := execOpts(opts)
	if !opts.Force && sysutil.CommandExists("solaar") {
		fmt.Fprintln(opts.Stdout, "solaar already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing solaar...")
	return sysutil.InstallPackage(ctx, o, "solaar")
}

func solaarUninstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS == platform.MacOS {
		return ErrUnsupportedOS
	}
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing solaar...")
	return sysutil.RemovePackage(ctx, o, "solaar")
}
