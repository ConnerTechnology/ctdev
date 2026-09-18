package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

func claudeDesktopInstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := execOpts(opts)
	if !opts.Force && alreadyInstalled("claude-desktop") {
		fmt.Fprintln(opts.Stdout, "Claude Desktop already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing Claude Desktop...")
	return sysutil.BrewCaskInstall(ctx, o, "claude")
}

func claudeDesktopUninstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Claude Desktop...")
	return sysutil.BrewCaskRemove(ctx, o, "claude")
}
