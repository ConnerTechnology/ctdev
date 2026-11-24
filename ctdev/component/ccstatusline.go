package component

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
		if err := npmInstallGlobal(ctx, o, "ccstatusline"); err != nil {
			return err
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

	if err := npmUninstallGlobal(ctx, o, "ccstatusline"); err != nil {
		return err
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
