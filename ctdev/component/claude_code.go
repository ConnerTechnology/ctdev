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

	// Deploy config files from embedded configs
	if err := deployClaudeCodeConfigs(o); err != nil {
		fmt.Fprintf(opts.Stdout, "warning: could not deploy claude-code configs: %v\n", err)
	}

	return nil
}

func deployClaudeCodeConfigs(o sysutil.Opts) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configDir := filepath.Join(home, ".claude")

	files := []struct{ src, dst string }{
		{"configs/claude-code/CLAUDE.md", filepath.Join(configDir, "CLAUDE.md")},
		{"configs/claude-code/settings.json", filepath.Join(configDir, "settings.json")},
		{"configs/claude-code/settings.local.json", filepath.Join(configDir, "settings.local.json")},
	}

	for _, f := range files {
		if o.DryRun {
			fmt.Fprintf(o.Stdout, "[dry-run] deploy %s → %s\n", filepath.Base(f.src), f.dst)
			continue
		}
		if err := sysutil.DeployFileFromFS(Configs, f.src, f.dst); err != nil {
			return fmt.Errorf("deploy %s: %w", filepath.Base(f.src), err)
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
