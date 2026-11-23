package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

func devcontainerInstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)

	// Phase 1: install the CLI (skip if present unless --force).
	if opts.Force || !alreadyInstalled("devcontainer") {
		fmt.Fprintln(opts.Stdout, "Installing @devcontainers/cli...")
		if err := npmInstallGlobal(ctx, o, "@devcontainers/cli"); err != nil {
			return err
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

func devcontainerUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing @devcontainers/cli and dx wrapper...")

	_ = npmUninstallGlobal(ctx, o, "@devcontainers/cli")

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
