package component

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// ccstatusline renders the Claude Code status line. It is part of the
// claude-code component, not a component of its own: on its own it does
// nothing, and the baseline settings.json is what points Claude Code at it.
// Installed from npm on every OS so there is one code path; the layout ships
// with it.
func installCcstatusline(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)

	if opts.Force || !sysutil.CommandExists("ccstatusline") {
		fmt.Fprintln(opts.Stdout, "Installing ccstatusline...")
		npm, err := npmPath()
		if err != nil {
			return err
		}
		if err := sysutil.Run(ctx, o, npm, "install", "-g", "ccstatusline"); err != nil {
			return fmt.Errorf("npm install ccstatusline: %w", err)
		}
	}

	// Always deploy the layout (keeps dotfiles in sync)
	dst, err := ccstatuslineConfigPath()
	if err != nil {
		return err
	}
	if err := deployOrDryRun(o, "configs/ccstatusline/settings.json", dst); err != nil {
		return fmt.Errorf("deploy ccstatusline config: %w", err)
	}
	return nil
}

func uninstallCcstatusline(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	if sysutil.CommandExists("ccstatusline") {
		fmt.Fprintln(opts.Stdout, "Removing ccstatusline...")
		npm, err := npmPath()
		if err != nil {
			return err
		}
		if err := sysutil.Run(ctx, o, npm, "uninstall", "-g", "ccstatusline"); err != nil {
			return fmt.Errorf("npm uninstall ccstatusline: %w", err)
		}
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
