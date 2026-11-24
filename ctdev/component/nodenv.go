package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/ctdev/ctdev/nodenv"
	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// nodenvInstall installs nodenv and node-build and no Node version: which
// Node runs, globally or per project, is the user's call through
// `ctdev configure node` (ADR-0023).
func nodenvInstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)

	if !opts.Force && alreadyInstalled("nodenv") {
		fmt.Fprintln(opts.Stdout, "nodenv already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing nodenv and node-build...")

	if platform.Detect().PackageManager == "brew" {
		return sysutil.Run(ctx, o, "brew", "install", "nodenv", "node-build")
	}
	root := nodenv.Root()
	if err := ensureGitClone(ctx, o, "https://github.com/nodenv/nodenv.git", root); err != nil {
		return fmt.Errorf("clone nodenv: %w", err)
	}
	if err := ensureGitClone(ctx, o, "https://github.com/nodenv/node-build.git", filepath.Join(root, "plugins", "node-build")); err != nil {
		return fmt.Errorf("clone node-build: %w", err)
	}
	return nil
}

func nodenvUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing nodenv and every Node version it installed...")

	if platform.Detect().PackageManager == "brew" {
		if err := sysutil.Run(ctx, o, "brew", "uninstall", "nodenv", "node-build"); err != nil {
			return err
		}
	}
	root := nodenv.Root()
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] rm -rf %s\n", root)
		return nil
	}
	return os.RemoveAll(root)
}
