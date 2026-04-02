package component

import (
	"context"
	"fmt"
	"os"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func logiOptionsInstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	if !opts.Force {
		// Check common app bundle names (case varies by version)
		for _, name := range []string{"Logi Options+.app", "Logi Options.app", "logioptionsplus.app"} {
			if _, err := os.Stat("/Applications/" + name); err == nil {
				fmt.Fprintln(opts.Stdout, "Logi Options+ already installed")
				return nil
			}
		}
	}
	fmt.Fprintln(opts.Stdout, "Installing Logi Options+...")
	return sysutil.BrewCaskInstall(o, "logi-options+")
}

func logiOptionsUninstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing Logi Options+...")
	return sysutil.BrewCaskRemove(o, "logi-options+")
}
