package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

func antigravityInstall(ctx context.Context, opts ExecOpts) error {
	if !opts.Force && alreadyInstalled("antigravity-cli") {
		fmt.Fprintln(opts.Stdout, "Antigravity CLI already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing Antigravity CLI...")
	const url = "https://antigravity.google/cli/install.sh"
	if opts.DryRun {
		fmt.Fprintf(opts.Stdout, "[dry-run] download and run %s\n", url)
		return nil
	}
	// Finish the download before running any of it. Even a failed transfer
	// can leave a prefix that bash would interpret as a valid program.
	dir, err := os.MkdirTemp("", "ctdev-antigravity-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	script := filepath.Join(dir, "install.sh")
	o := execOpts(opts)
	if err := sysutil.Run(ctx, o, "curl", "-fsSL", "-o", script, url); err != nil {
		return fmt.Errorf("download Antigravity installer: %w", err)
	}
	if err := sysutil.Run(ctx, o, "bash", script); err != nil {
		return fmt.Errorf("run Antigravity installer: %w", err)
	}
	return nil
}

// antigravityUninstall removes the binary only. ~/.gemini holds the user's
// login and conversations, which the installer put there for the user, not ctdev.
func antigravityUninstall(ctx context.Context, opts ExecOpts) error {
	fmt.Fprintln(opts.Stdout, "Removing Antigravity CLI...")

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	bin := filepath.Join(home, ".local", "bin", "agy")
	if _, err := os.Stat(bin); err != nil {
		return nil
	}
	return sysutil.Run(ctx, execOpts(opts), "rm", "-f", bin)
}
