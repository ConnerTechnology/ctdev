package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func claudeDesktopInstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := execOpts(opts)
	if !opts.Force && dirExists("/Applications/Claude.app") {
		fmt.Fprintln(opts.Stdout, "Claude Desktop already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing Claude Desktop...")
	return sysutil.BrewCaskInstall(o, "claude")
}

func claudeDesktopUninstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Claude Desktop...")
	return sysutil.BrewCaskRemove(o, "claude")
}
