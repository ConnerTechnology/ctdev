package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func gitInstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

	// Ensure git is installed
	if !sysutil.CommandExists("git") {
		fmt.Fprintln(opts.Stdout, "Installing git...")
		if err := sysutil.InstallPackage(o, "git"); err != nil {
			return err
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dst := filepath.Join(home, ".gitconfig")

	if !opts.Force {
		if _, err := os.Stat(dst); err == nil {
			fmt.Fprintln(opts.Stdout, "git configuration already exists")
			return nil
		}
	}

	fmt.Fprintln(opts.Stdout, "Installing git configuration...")

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] deploy .gitconfig → %s\n", dst)
		return nil
	}
	if err := sysutil.DeployFileFromFS(Configs, "configs/git/.gitconfig", dst); err != nil {
		return err
	}

	fmt.Fprintln(opts.Stdout, "Git configuration installed")
	return nil
}

func gitUninstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing git configuration...")

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	gitconfig := filepath.Join(home, ".gitconfig")

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] rm %s\n", gitconfig)
		return nil
	}

	_ = os.Remove(gitconfig)
	fmt.Fprintln(opts.Stdout, ".gitconfig.local preserved")
	return nil
}
