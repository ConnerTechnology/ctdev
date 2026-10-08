package component

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// ccstatusline renders the Claude Code status line. Installed from npm on
// every OS so there is one code path; the layout ships with it. On its own it
// does nothing: claude-code depends on it, and the claude-code baseline
// settings.json is what points Claude Code at it. ccstatusline never writes
// ~/.claude/settings.json, so that file has one writer.
func ccstatuslineInstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)

	// Phase 1: install the binary (skip if present unless --force)
	if opts.Force || !alreadyInstalled("ccstatusline") {
		fmt.Fprintln(opts.Stdout, "Installing ccstatusline...")
		npm, err := npmPath(o)
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

	npm, err := npmPath(o)
	if err != nil {
		return err
	}
	if err := sysutil.Run(ctx, o, npm, "uninstall", "-g", "ccstatusline"); err != nil {
		return fmt.Errorf("npm uninstall ccstatusline: %w", err)
	}

	// Remove the layout only once the package is gone
	dst, err := ccstatuslineConfigPath()
	if err != nil {
		return err
	}
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] rm %s\n", dst)
		return nil
	}
	if err := os.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove ccstatusline config: %w", err)
	}
	return nil
}

func ccstatuslineConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "ccstatusline", "settings.json"), nil
}
