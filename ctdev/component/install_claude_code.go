package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func claudeCodeInstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

	if !opts.Force && sysutil.CommandExists("claude") {
		fmt.Fprintln(opts.Stdout, "Claude Code already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing Claude Code...")

	if err := sysutil.Run(o, "bash", "-c", "curl -fsSL https://claude.ai/install.sh | bash"); err != nil {
		return fmt.Errorf("claude code installer: %w", err)
	}

	// Symlink config files from dotfiles components directory
	if err := symlinkClaudeCodeConfigs(o); err != nil {
		fmt.Fprintf(opts.Stdout, "warning: could not symlink claude-code configs: %v\n", err)
	}

	return nil
}

func symlinkClaudeCodeConfigs(o sysutil.Opts) error {
	dotfiles := findDotfilesRoot()
	if dotfiles == "" {
		return fmt.Errorf("could not determine dotfiles root")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	links := []struct{ src, dst string }{
		{filepath.Join(dotfiles, "components", "claude-code", "CLAUDE.md"), filepath.Join(configDir, "CLAUDE.md")},
		{filepath.Join(dotfiles, "components", "claude-code", "settings.json"), filepath.Join(configDir, "settings.json")},
		{filepath.Join(dotfiles, "components", "claude-code", "settings.local.json"), filepath.Join(configDir, "settings.local.json")},
	}

	for _, l := range links {
		if _, err := os.Stat(l.src); err != nil {
			continue // skip if source doesn't exist
		}
		if o.DryRun {
			fmt.Fprintf(o.Stdout, "[dry-run] symlink %s → %s\n", l.src, l.dst)
			continue
		}
		if err := sysutil.SafeSymlink(l.src, l.dst); err != nil {
			return fmt.Errorf("symlink %s: %w", filepath.Base(l.src), err)
		}
	}
	return nil
}

func claudeCodeUninstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing Claude Code...")

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// Remove CLI binary
	claudeBin := filepath.Join(home, ".local", "bin", "claude")
	if _, err := os.Stat(claudeBin); err == nil {
		if err := sysutil.Run(o, "rm", "-f", claudeBin); err != nil {
			return err
		}
	}

	// Remove config symlinks (preserve ~/.claude directory)
	configDir := filepath.Join(home, ".claude")
	for _, name := range []string{"CLAUDE.md", "settings.json", "settings.local.json"} {
		link := filepath.Join(configDir, name)
		if fi, err := os.Lstat(link); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			if err := sysutil.Run(o, "rm", "-f", link); err != nil {
				return err
			}
		}
	}

	return nil
}
