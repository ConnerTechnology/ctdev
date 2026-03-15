package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func tmuxInstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	if !opts.Force && sysutil.CommandExists("tmux") {
		fmt.Fprintln(opts.Stdout, "tmux already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing tmux...")
	return sysutil.InstallPackage(o, "tmux")
}

func tmuxUninstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing tmux...")
	return sysutil.RemovePackage(o, "tmux")
}
