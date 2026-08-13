package diagnose

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// capture runs a read-only command and returns its trimmed stdout, or "" if it
// fails. Every probe in this package is best-effort: a missing tool or a
// non-zero exit is a check we couldn't make, not an error to propagate.
func capture(ctx context.Context, name string, args ...string) string {
	out, _ := captureErr(ctx, name, args...)
	return out
}

// captureErr is capture for the callers that need to tell "empty output" from
// "the command isn't here".
func captureErr(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	// Deliberately no Stderr: tools like smartctl and iw are chatty about
	// permissions, and none of it belongs in a report.
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(stdout.String()), err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// sudoCapture runs a read-only command as root without ever prompting. A
// missing password is a failed probe, not a stall — see sysutil.SudoNoPrompt.
func sudoCapture(ctx context.Context, name string, args ...string) (string, bool) {
	if !sysutil.CanElevateQuietly(ctx) {
		return "", false
	}
	cmd := sysutil.SudoNoPrompt(ctx, name, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return strings.TrimSpace(stdout.String()), true
}

// powershell runs a PowerShell snippet on Windows and returns its trimmed
// output. -NonInteractive matters: a probe must never sit waiting for input on
// a machine we're only visiting.
func powershell(ctx context.Context, script string) string {
	return capture(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
}

func commandExists(name string) bool { return sysutil.CommandExists(name) }

// globPaths is filepath.Glob with the error dropped — a malformed pattern is a
// bug in a literal here, not a runtime condition worth propagating.
func globPaths(pattern string) []string {
	matches, _ := filepath.Glob(pattern)
	return matches
}

// firstLine returns the first line of s, trimmed.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// lines splits s into non-empty trimmed lines.
func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
