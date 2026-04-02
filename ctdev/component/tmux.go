package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func tmuxInstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

	if !opts.Force && sysutil.CommandExists("tmux") {
		fmt.Fprintln(opts.Stdout, "tmux already installed")
	} else {
		fmt.Fprintln(opts.Stdout, "Installing tmux...")
		if err := sysutil.InstallPackage(o, "tmux"); err != nil {
			return err
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dst := filepath.Join(home, ".tmux.conf")

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] deploy .tmux.conf → %s\n", dst)
		return nil
	}
	if err := sysutil.DeployFileFromFS(Configs, "configs/tmux/.tmux.conf", dst); err != nil {
		return fmt.Errorf("deploy tmux config: %w", err)
	}

	fmt.Fprintln(opts.Stdout, "tmux configuration installed")
	return nil
}

func tmuxUninstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing tmux...")
	return sysutil.RemovePackage(o, "tmux")
}
