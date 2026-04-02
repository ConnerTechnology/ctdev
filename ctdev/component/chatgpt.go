package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func chatgptInstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	if !opts.Force && sysutil.CommandExists("chatgpt") {
		fmt.Fprintln(opts.Stdout, "ChatGPT already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing ChatGPT...")
	return sysutil.BrewCaskInstall(o, "chatgpt")
}

func chatgptUninstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing ChatGPT...")
	return sysutil.BrewCaskRemove(o, "chatgpt")
}
