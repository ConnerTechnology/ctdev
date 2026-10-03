package component

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

	// Always deploy config files (keeps dotfiles in sync)
	if err := deployClaudeCodeConfigs(opts); err != nil {
		fmt.Fprintf(opts.Stdout, "warning: could not deploy claude-code configs: %v\n", err)
	}

	return nil
}

// deployClaudeCodeConfigs writes a missing baseline file and leaves an
// identical one alone. A drifted file is only replaced with --force; otherwise
// it's reported, and the configure step shows the diff and asks.
func deployClaudeCodeConfigs(opts ExecOpts) error {
	drifts, err := ClaudeCodeDrift()
	if err != nil {
		return err
	}
	for _, d := range drifts {
		switch {
		case d.Missing && opts.DryRun:
			fmt.Fprintf(opts.Stdout, "[dry-run] write %s\n", d.Path)
		case d.Missing:
			if _, err := ReplaceClaudeCodeFile(d); err != nil {
				return err
			}
		case opts.DryRun:
			fmt.Fprintf(opts.Stdout, "[dry-run] %s has drifted from ctdev's copy:\n%s", d.Path, d.Diff)
		case opts.Force:
			backup, err := ReplaceClaudeCodeFile(d)
			if err != nil {
				return err
			}
			fmt.Fprintf(opts.Stdout, "Replaced %s with ctdev's copy%s\n", d.Path, d.ReplacedNote(backup))
		default:
			fmt.Fprintf(opts.Stdout, "%s has drifted from ctdev's copy; left unchanged. Run ctdev install claude-code in a terminal to review.\n", d.Path)
		}
	}
	return removeClaudeCodeSettingsLocal(opts)
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
		have, _ := os.ReadFile(d.Path)
		if d.LinkTarget == "" && bytes.Equal(have, want) {
			continue
		}
		d.Diff = lineDiff(string(have), string(want))
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
		if backup, err = backupFile(d.Path); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(d.Path, d.content, mode); err != nil {
		return backup, err
	}
	return backup, os.Chmod(d.Path, mode)
}

// removeClaudeCodeSettingsLocal backs up and removes ~/.claude/settings.local.json:
// Claude Code reads settings.local.json only inside a project, so one at the
// user level looks real but does nothing.
func removeClaudeCodeSettingsLocal(opts ExecOpts) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".claude", "settings.local.json")
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if opts.DryRun {
		fmt.Fprintf(opts.Stdout, "[dry-run] back up and remove %s (Claude Code never reads it)\n", path)
		return nil
	}
	backup, err := backupFile(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(opts.Stdout, "Removed %s (Claude Code never reads it); backup at %s\n", path, backup)
	return nil
}

// backupFile renames path to <path>.<YYYY-MM-DDTHH-MM-SS>.bak, the name
// sysutil.DeployFile gives its backups.
func backupFile(path string) (string, error) {
	backup := fmt.Sprintf("%s.%s.bak", path, time.Now().Format("2006-01-02T15-04-05"))
	if err := os.Rename(path, backup); err != nil {
		return "", fmt.Errorf("back up %s: %w", path, err)
	}
	return backup, nil
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

// lineDiff renders a line diff from a to b: "-" lines only in a, "+" lines
// only in b, and up to two unchanged lines of context around each change.
func lineDiff(a, b string) string {
	x := strings.Split(strings.TrimSuffix(a, "\n"), "\n")
	y := strings.Split(strings.TrimSuffix(b, "\n"), "\n")
	if a == "" {
		x = nil
	}

	// Longest common subsequence table, filled from the end.
	lcs := make([][]int, len(x)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(y)+1)
	}
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}

	type line struct {
		op   byte
		text string
	}
	var lines []line
	i, j := 0, 0
	for i < len(x) || j < len(y) {
		switch {
		case i < len(x) && j < len(y) && x[i] == y[j]:
			lines = append(lines, line{' ', x[i]})
			i++
			j++
		case i < len(x) && (j == len(y) || lcs[i+1][j] >= lcs[i][j+1]):
			lines = append(lines, line{'-', x[i]})
			i++
		default:
			lines = append(lines, line{'+', y[j]})
			j++
		}
	}

	const contextLines = 2
	var out strings.Builder
	skipped := false
	for k, l := range lines {
		near := false
		for m := max(0, k-contextLines); m <= min(len(lines)-1, k+contextLines); m++ {
			if lines[m].op != ' ' {
				near = true
				break
			}
		}
		if !near {
			skipped = true
			continue
		}
		if skipped {
			out.WriteString("  ...\n")
			skipped = false
		}
		fmt.Fprintf(&out, "%c %s\n", l.op, l.text)
	}
	return out.String()
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
