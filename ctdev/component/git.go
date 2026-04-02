package component

import (
	"context"
	"fmt"
	"io"
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

	fmt.Fprintln(opts.Stdout, "Installing git configuration...")

	dotfiles := findDotfilesRoot()
	src := filepath.Join(dotfiles, "components", "git", ".gitconfig")
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dst := filepath.Join(home, ".gitconfig")

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] cp %s %s\n", src, dst)
		return nil
	}

	// Copy (not symlink) so user changes persist
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source gitconfig: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create dest gitconfig: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy gitconfig: %w", err)
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
