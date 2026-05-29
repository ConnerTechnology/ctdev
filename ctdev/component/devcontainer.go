package component

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func devcontainerInstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)

	// Phase 1: install the CLI (skip if present unless --force).
	if opts.Force || !sysutil.CommandExists("devcontainer") {
		fmt.Fprintln(opts.Stdout, "Installing @devcontainers/cli...")
		npm, err := npmPath()
		if err != nil {
			return err
		}
		if err := sysutil.Run(ctx, o, npm, "install", "-g", "@devcontainers/cli"); err != nil {
			return fmt.Errorf("npm install @devcontainers/cli: %w", err)
		}
	} else {
		fmt.Fprintln(opts.Stdout, "@devcontainers/cli already installed, updating wrappers...")
	}

	// Phase 2: always deploy the dx wrapper (keeps the helper in sync).
	return deployDxWrapper(o)
}

// deployDxWrapper deploys the `dx` helper to ~/.local/bin and makes it
// executable (embedded files arrive as 0644).
func deployDxWrapper(o sysutil.Opts) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dst := filepath.Join(home, ".local", "bin", "dx")

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] deploy dx → %s\n", dst)
		return nil
	}
	if err := sysutil.DeployFileFromFS(Configs, "configs/devcontainer/dx", dst); err != nil {
		return fmt.Errorf("deploy dx: %w", err)
	}
	if err := os.Chmod(dst, 0o755); err != nil {
		return fmt.Errorf("chmod dx: %w", err)
	}
	return nil
}

// npmPath finds npm, falling back to the nodenv shim when nodenv isn't yet on
// PATH (common right after `ctdev install node` in the same shell session).
func npmPath() (string, error) {
	if p, err := exec.LookPath("npm"); err == nil {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	shim := filepath.Join(home, ".nodenv", "shims", "npm")
	if _, err := os.Stat(shim); err == nil {
		return shim, nil
	}
	return "", fmt.Errorf("npm not found — install the node component first")
}

func devcontainerUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing @devcontainers/cli and dx wrapper...")

	if npm, err := npmPath(); err == nil {
		_ = sysutil.Run(ctx, o, npm, "uninstall", "-g", "@devcontainers/cli")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dst := filepath.Join(home, ".local", "bin", "dx")
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] rm %s\n", dst)
		return nil
	}
	_ = os.Remove(dst)
	return nil
}
