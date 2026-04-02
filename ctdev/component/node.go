package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

const nodeVersion = "24.0.0"

func nodeInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	nodenvDir := filepath.Join(home, ".nodenv")

	if !opts.Force && dirExists(nodenvDir) {
		fmt.Fprintln(opts.Stdout, "Node.js already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing Node.js via nodenv...")

	switch p.PackageManager {
	case "brew":
		if err := sysutil.Run(o, "brew", "install", "nodenv", "node-build"); err != nil {
			return err
		}
	default:
		// Linux: git clone nodenv and node-build
		if err := ensureGitClone(o, "https://github.com/nodenv/nodenv.git", nodenvDir); err != nil {
			return fmt.Errorf("clone nodenv: %w", err)
		}
		pluginDir := filepath.Join(nodenvDir, "plugins", "node-build")
		if err := ensureGitClone(o, "https://github.com/nodenv/node-build.git", pluginDir); err != nil {
			return fmt.Errorf("clone node-build: %w", err)
		}
	}

	// Use nodenv from the install location
	nodenvBin := "nodenv"
	if p.PackageManager != "brew" {
		nodenvBin = filepath.Join(nodenvDir, "bin", "nodenv")
	}

	fmt.Fprintf(opts.Stdout, "Installing Node.js %s...\n", nodeVersion)
	if err := sysutil.Run(o, nodenvBin, "install", nodeVersion); err != nil {
		return fmt.Errorf("nodenv install %s: %w", nodeVersion, err)
	}

	if err := sysutil.Run(o, nodenvBin, "global", nodeVersion); err != nil {
		return fmt.Errorf("nodenv global %s: %w", nodeVersion, err)
	}

	return nil
}

func nodeUninstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing Node.js (nodenv)...")

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	nodenvDir := filepath.Join(home, ".nodenv")
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] rm -rf %s\n", nodenvDir)
		return nil
	}

	return os.RemoveAll(nodenvDir)
}

// ensureGitClone clones a repo if the destination doesn't exist, or pulls if it does.
func ensureGitClone(o sysutil.Opts, url, dest string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] git clone %s %s\n", url, dest)
		return nil
	}

	if dirExists(dest) {
		return sysutil.Run(o, "git", "-C", dest, "pull", "--ff-only")
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	return sysutil.Run(o, "git", "clone", url, dest)
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
