package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

func antigravityInstall(ctx context.Context, opts ExecOpts) error {
	if !opts.Force && sysutil.CommandExists("agy") {
		fmt.Fprintln(opts.Stdout, "Antigravity CLI already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing Antigravity CLI...")
	if err := sysutil.Run(ctx, execOpts(opts), "bash", "-c", "curl -fsSL https://antigravity.google/cli/install.sh | bash"); err != nil {
		return fmt.Errorf("antigravity installer: %w", err)
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
