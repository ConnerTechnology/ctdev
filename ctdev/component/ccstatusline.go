package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// ccstatusline renders the Claude Code status line. Installed from npm on
// every OS so there is one code path; the layout ships with it.
func ccstatuslineInstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)

	// Phase 1: install the binary (skip if present unless --force)
	if opts.Force || !alreadyInstalled("ccstatusline") {
		fmt.Fprintln(opts.Stdout, "Installing ccstatusline...")
		npm, err := npmPath()
		if err != nil {
			return err
		}
		if err := sysutil.Run(ctx, o, npm, "install", "-g", "ccstatusline"); err != nil {
			return fmt.Errorf("npm install ccstatusline: %w", err)
		}
	} else {
		fmt.Fprintln(opts.Stdout, "ccstatusline already installed")
	}

	// Phase 2: always deploy the layout (keeps dotfiles in sync)
	dst, err := ccstatuslineConfigPath()
	if err != nil {
		return err
	}
	if err := deployOrDryRun(o, "configs/ccstatusline/settings.json", dst); err != nil {
		return fmt.Errorf("deploy ccstatusline config: %w", err)
	}
	return nil
}

func ccstatuslineUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing ccstatusline...")

	if dst, err := ccstatuslineConfigPath(); err == nil {
		if o.DryRun {
			fmt.Fprintf(o.Stdout, "[dry-run] rm %s\n", dst)
		} else {
			_ = os.Remove(dst)
		}
	}

	npm, err := npmPath()
	if err != nil {
		return err
	}
	return sysutil.Run(ctx, o, npm, "uninstall", "-g", "ccstatusline")
}

func ccstatuslineConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "ccstatusline", "settings.json"), nil
}
