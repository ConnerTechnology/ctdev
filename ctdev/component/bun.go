package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func bunInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && sysutil.CommandExists("bun") {
		fmt.Fprintln(opts.Stdout, "Bun already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing Bun...")

	if p.OS == platform.MacOS {
		return sysutil.Run(o, "brew", "install", "oven-sh/bun/bun")
	}

	// Linux: use official installer
	return sysutil.Run(o, "bash", "-c", "curl -fsSL https://bun.sh/install | bash")
}

func bunUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Bun...")

	if p.OS == platform.MacOS {
		return sysutil.Run(o, "brew", "uninstall", "bun")
	}

	// Linux: remove ~/.bun directory
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	bunDir := filepath.Join(home, ".bun")
	if _, err := os.Stat(bunDir); err == nil {
		return sysutil.Run(o, "rm", "-rf", bunDir)
	}
	return nil
}
