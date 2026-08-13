package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// Claude Code's configuration lives in the private ConnerTechnology/AI repo, not in
// this one. ctdev owns the machine; that repo owns Claude — its CLAUDE.md, settings,
// plugins, MCP servers, skills, and agents. ctdev installs the binary and hands off.
const (
	aiRepoURL     = "git@github.com:ConnerTechnology/AI.git"
	aiRepoRelPath = "Repos/github.com/ConnerTechnology/AI"
	aiRepoEnvVar  = "CT_AI_REPO"
)

func claudeCodeInstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)

	if opts.Force || !sysutil.CommandExists("claude") {
		fmt.Fprintln(opts.Stdout, "Installing Claude Code...")
		if err := sysutil.Run(ctx, o, "bash", "-c", "curl -fsSL https://claude.ai/install.sh | bash"); err != nil {
			return fmt.Errorf("claude code installer: %w", err)
		}
	}

	// A failure here leaves a working CLI with unconfigured Claude, which is worth a
	// warning but not a failed install — the repo is private, so a machine without
	// SSH access to it can legitimately get this far and no further.
	if err := syncClaudeConfig(ctx, o); err != nil {
		fmt.Fprintf(opts.Stdout, "warning: could not sync Claude config: %v\n", err)
	}

	return nil
}

// aiRepoDir returns where the AI repo should live, honouring CT_AI_REPO so a machine
// that keeps its checkouts somewhere else can say so.
func aiRepoDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv(aiRepoEnvVar)); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, aiRepoRelPath), nil
}

// syncClaudeConfig clones or updates the AI repo and runs its setup script. That
// script owns ~/.claude: it symlinks agents, commands, skills, and CLAUDE.md, merges
// settings.json, installs plugins, and registers MCP servers.
//
// ctdev deliberately no longer deploys those files itself. It used to, by copying them
// over the live ones, which reverted every setting changed interactively and left the
// committed copies drifting behind the real config.
func syncClaudeConfig(ctx context.Context, o sysutil.Opts) error {
	dest, err := aiRepoDir()
	if err != nil {
		return err
	}

	// Only a missing checkout is fatal. If the repo is already here, a failed pull —
	// no upstream yet, offline, unpushed local commits — should not stop us
	// configuring the machine from what's on disk. Stale config beats none.
	if dirExists(dest) {
		if err := sysutil.Run(ctx, o, "git", "-C", dest, "pull", "--ff-only"); err != nil {
			fmt.Fprintf(o.Stdout, "warning: could not update %s: %v (configuring from the existing checkout)\n", dest, err)
		}
	} else if err := ensureGitClone(ctx, o, aiRepoURL, dest); err != nil {
		return fmt.Errorf("clone %s: %w (private repo — needs SSH access to GitHub)", aiRepoURL, err)
	}

	setup := filepath.Join(dest, "scripts", "setup.sh")
	if !o.DryRun {
		if _, err := os.Stat(setup); err != nil {
			return fmt.Errorf("%s not found — is the checkout complete?", setup)
		}
	}

	fmt.Fprintln(o.Stdout, "Configuring Claude Code from the AI repo...")
	return sysutil.Run(ctx, o, "bash", setup)
}

func claudeCodeUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Claude Code...")

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// Remove CLI binary
	claudeBin := filepath.Join(home, ".local", "bin", "claude")
	if _, err := os.Stat(claudeBin); err == nil {
		if err := sysutil.Run(ctx, o, "rm", "-f", claudeBin); err != nil {
			return err
		}
	}

	// Remove only the symlinks pointing into the AI repo. Everything else under
	// ~/.claude is either the user's own or holds real content — settings.json in
	// particular is merged rather than deployed, so deleting it would discard machine
	// state ctdev never owned. Re-running the repo's setup.sh restores these.
	if err := removeAIRepoLinks(ctx, o, home); err != nil {
		fmt.Fprintf(opts.Stdout, "warning: could not remove Claude config links: %v\n", err)
	}

	return nil
}

// removeAIRepoLinks deletes entries under ~/.claude that are symlinks into the AI
// repo, leaving real files and unrelated links alone.
func removeAIRepoLinks(ctx context.Context, o sysutil.Opts, home string) error {
	repo, err := aiRepoDir()
	if err != nil {
		return err
	}
	configDir := filepath.Join(home, ".claude")

	candidates := []string{
		filepath.Join(configDir, "CLAUDE.md"),
		filepath.Join(configDir, "agents"),
		filepath.Join(configDir, "commands"),
	}
	if entries, err := os.ReadDir(filepath.Join(configDir, "skills")); err == nil {
		for _, e := range entries {
			candidates = append(candidates, filepath.Join(configDir, "skills", e.Name()))
		}
	}

	for _, path := range candidates {
		fi, err := os.Lstat(path)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			continue
		}
		target, err := os.Readlink(path)
		if err != nil || !strings.HasPrefix(target, repo) {
			continue
		}
		if err := sysutil.Run(ctx, o, "rm", "-f", path); err != nil {
			return err
		}
	}

	return nil
}
