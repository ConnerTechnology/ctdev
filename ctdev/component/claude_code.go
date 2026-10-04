package component

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// claudeCodeFiles are the files ctdev owns under ~/.claude: one baseline,
// identical on every computer (docs/adr/0008-ctdev-owns-root-claude-config.md).
var claudeCodeFiles = []struct{ src, name string }{
	{"configs/claude-code/settings.json", "settings.json"},
	{"configs/claude-code/CLAUDE.md", "CLAUDE.md"},
}

func claudeCodeInstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)

	if opts.Force || !sysutil.CommandExists("claude") {
		fmt.Fprintln(opts.Stdout, "Installing Claude Code...")
		if err := sysutil.Run(ctx, o, "bash", "-c", "curl -fsSL https://claude.ai/install.sh | bash"); err != nil {
			return fmt.Errorf("claude code installer: %w", err)
		}
	}

	return writeMissingClaudeCodeFiles(opts)
}

// writeMissingClaudeCodeFiles writes the baseline where it's missing. A file
// that differs is never touched here: the install runs under the progress
// screen, which can't show a diff or ask, so the drift review runs after it
// (reviewClaudeCode in cmd).
func writeMissingClaudeCodeFiles(opts ExecOpts) error {
	drifts, err := ClaudeCodeDrift()
	if err != nil {
		return err
	}
	for _, d := range drifts {
		if !d.Missing {
			continue
		}
		if opts.DryRun {
			continue // the review after the install reports it
		}
		if _, err := ReplaceClaudeCodeFile(d); err != nil {
			return err
		}
	}
	return nil
}

// ClaudeCodeFileDrift is one owned file that doesn't match ctdev's baseline.
type ClaudeCodeFileDrift struct {
	Path       string // e.g. ~/.claude/settings.json, absolute
	LinkTarget string // set when Path is a symlink
	Missing    bool
	Diff       string // live → ctdev's copy; empty when Missing
	content    []byte // ctdev's copy
}

// ClaudeCodeDrift lists the owned files that are missing, differ from ctdev's
// copy, or are symlinks. Identical regular files aren't listed.
func ClaudeCodeDrift() ([]ClaudeCodeFileDrift, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	var drifts []ClaudeCodeFileDrift
	for _, f := range claudeCodeFiles {
		want, err := Configs.ReadFile(f.src)
		if err != nil {
			return nil, fmt.Errorf("read embedded %s: %w", f.src, err)
		}
		d := ClaudeCodeFileDrift{Path: filepath.Join(home, ".claude", f.name), content: want}

		fi, err := os.Lstat(d.Path)
		if errors.Is(err, os.ErrNotExist) {
			d.Missing = true
			drifts = append(drifts, d)
			continue
		}
		if err != nil {
			return nil, err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			d.LinkTarget, _ = os.Readlink(d.Path)
		}
		// Follows a symlink; a dangling one reads as empty.
		have, err := os.ReadFile(d.Path)
		if err != nil && !(d.LinkTarget != "" && errors.Is(err, os.ErrNotExist)) {
			return nil, fmt.Errorf("read %s: %w", d.Path, err)
		}
		if d.LinkTarget == "" && bytes.Equal(have, want) {
			continue
		}
		if bytes.Equal(have, want) {
			d.Diff = "  (contents are identical; only the link is replaced)\n"
		} else {
			d.Diff = sysutil.LineDiff(string(have), string(want))
		}
		drifts = append(drifts, d)
	}
	return drifts, nil
}

// ReplaceClaudeCodeFile writes ctdev's copy over d.Path and returns the backup
// it made, if any. A regular file is renamed to <file>.<stamp>.bak first and
// its mode kept; a symlink is removed and its target left untouched.
func ReplaceClaudeCodeFile(d ClaudeCodeFileDrift) (backup string, err error) {
	if err := os.MkdirAll(filepath.Dir(d.Path), 0o755); err != nil {
		return "", err
	}
	mode := os.FileMode(0o644)
	switch {
	case d.Missing:
	case d.LinkTarget != "":
		if err := os.Remove(d.Path); err != nil {
			return "", fmt.Errorf("remove symlink %s: %w", d.Path, err)
		}
	default:
		if fi, err := os.Stat(d.Path); err == nil {
			mode = fi.Mode().Perm()
		}
		if backup, err = sysutil.BackupFile(d.Path); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(d.Path, d.content, mode); err != nil {
		// Put the old file back rather than leave nothing in its place.
		if backup != "" {
			_ = os.Rename(backup, d.Path)
		}
		return "", err
	}
	return backup, os.Chmod(d.Path, mode)
}

// ClaudeCodeSettingsLocalPath is ~/.claude/settings.local.json when it exists.
// Claude Code reads settings.local.json only inside a project, so one at the
// user level looks real but does nothing; ctdev backs it up and removes it.
func ClaudeCodeSettingsLocalPath() (string, bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false, err
	}
	path := filepath.Join(home, ".claude", "settings.local.json")
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return path, false, nil
	} else if err != nil {
		return path, false, err
	}
	return path, true, nil
}

// ReplacedNote says what happened to the old file after ReplaceClaudeCodeFile.
func (d ClaudeCodeFileDrift) ReplacedNote(backup string) string {
	switch {
	case d.LinkTarget != "":
		return fmt.Sprintf(" (was a link to %s, left untouched)", d.LinkTarget)
	case backup != "":
		return fmt.Sprintf(" (backup at %s)", backup)
	}
	return ""
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

	// Remove deployed config files (preserve ~/.claude directory)
	configDir := filepath.Join(home, ".claude")
	for _, name := range []string{"CLAUDE.md", "settings.json", "settings.local.json"} {
		f := filepath.Join(configDir, name)
		if _, err := os.Stat(f); err == nil {
			if err := sysutil.Run(ctx, o, "rm", "-f", f); err != nil {
				return err
			}
		}
	}

	return nil
}
