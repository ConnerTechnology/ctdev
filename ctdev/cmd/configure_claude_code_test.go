package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/component"
)

const driftedSettings = `{"model": "sonnet"}` + "\n"

// driftedClaudeHome has a matching CLAUDE.md and a drifted settings.json.
func driftedClaudeHome(t *testing.T) (dir, settings string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir = filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	md, _ := component.Configs.ReadFile("configs/claude-code/CLAUDE.md")
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), md, 0o644); err != nil {
		t.Fatal(err)
	}
	settings = filepath.Join(dir, "settings.json")
	if err := os.WriteFile(settings, []byte(driftedSettings), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, settings
}

func runReview(t *testing.T, r claudeCodeReview) (string, error) {
	t.Helper()
	var err error
	out := captureStdout(t, func() { err = reviewClaudeCode(context.Background(), r) })
	return out, err
}

func readString(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

func dotBaks(path string) []string {
	b, _ := filepath.Glob(path + ".*.bak")
	return b
}

func TestReviewClaudeCodeDeclineLeavesFile(t *testing.T) {
	_, settings := driftedClaudeHome(t)
	feedStdin(t, "n\n")

	out, err := runReview(t, claudeCodeReview{interactive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `- {"model": "sonnet"}`) || !strings.Contains(out, `+   "model": "opus",`) {
		t.Errorf("diff not shown:\n%s", out)
	}
	if readString(settings) != driftedSettings {
		t.Error("declined file was changed")
	}
	if strings.Contains(out, "CLAUDE.md has drifted") {
		t.Error("identical CLAUDE.md reported as drifted")
	}
}

func TestReviewClaudeCodeAcceptBacksUpAndReplaces(t *testing.T) {
	_, settings := driftedClaudeHome(t)
	feedStdin(t, "y\n")

	out, err := runReview(t, claudeCodeReview{interactive: true})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := component.Configs.ReadFile("configs/claude-code/settings.json")
	if readString(settings) != string(want) {
		t.Error("settings not replaced")
	}
	b := dotBaks(settings)
	if len(b) != 1 || readString(b[0]) != driftedSettings {
		t.Fatalf("expected one dated backup of the old file, got %v", b)
	}
	if !strings.Contains(out, "backup at "+b[0]) {
		t.Errorf("backup location not reported:\n%s", out)
	}
}

func TestReviewClaudeCodeForceReplacesWithoutAsking(t *testing.T) {
	_, settings := driftedClaudeHome(t)
	feedStdin(t, "") // any prompt would read EOF and cancel

	out, err := runReview(t, claudeCodeReview{force: true, interactive: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "[y/N]") {
		t.Error("--force asked")
	}
	if len(dotBaks(settings)) != 1 || !strings.Contains(out, "Replaced "+settings) {
		t.Errorf("--force did not back up, replace and report:\n%s", out)
	}
}

func TestReviewClaudeCodeDryRunShowsDiffWritesNothing(t *testing.T) {
	dir, settings := driftedClaudeHome(t)
	local := filepath.Join(dir, "settings.local.json")
	if err := os.WriteFile(local, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}

	out, err := runReview(t, claudeCodeReview{dryRun: true, force: true, interactive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `- {"model": "sonnet"}`) {
		t.Errorf("dry-run did not show the diff:\n%s", out)
	}
	if readString(settings) != driftedSettings || len(dotBaks(settings)) != 0 {
		t.Error("dry-run changed settings.json")
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Error("dry-run wrote CLAUDE.md")
	}
	if _, err := os.Stat(local); err != nil {
		t.Error("dry-run removed settings.local.json")
	}
}

func TestReviewClaudeCodeNoTerminalLeavesFileAndSays(t *testing.T) {
	_, settings := driftedClaudeHome(t)

	out, err := runReview(t, claudeCodeReview{})
	if err != nil {
		t.Fatal(err)
	}
	if readString(settings) != driftedSettings {
		t.Error("no-terminal review changed the file")
	}
	if !strings.Contains(out, "has drifted from ctdev's copy") || !strings.Contains(out, "Run ctdev install claude-code in a terminal") {
		t.Errorf("drift not reported:\n%s", out)
	}
}

func TestReviewClaudeCodeRemovesSettingsLocal(t *testing.T) {
	dir, _ := driftedClaudeHome(t)
	local := filepath.Join(dir, "settings.local.json")
	if err := os.WriteFile(local, []byte(`{"permissions": {"allow": ["WebSearch"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runReview(t, claudeCodeReview{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(local); !os.IsNotExist(err) {
		t.Error("settings.local.json still present")
	}
	b := dotBaks(local)
	if len(b) != 1 || readString(b[0]) != `{"permissions": {"allow": ["WebSearch"]}}` {
		t.Fatalf("expected one dated backup, got %v", b)
	}
	if !strings.Contains(out, "backup at "+b[0]) {
		t.Errorf("backup location not reported:\n%s", out)
	}
}

// Every question comes before any write, so cancelling really changes nothing.
func TestReviewClaudeCodeCancelWritesNothing(t *testing.T) {
	dir, settings := driftedClaudeHome(t)
	if err := os.Remove(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
	feedStdin(t, "") // EOF at the prompt

	if _, err := runReview(t, claudeCodeReview{interactive: true}); err == nil {
		t.Fatal("expected the prompt to be cancelled")
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Error("a missing file was written before the cancelled prompt")
	}
	if readString(settings) != driftedSettings {
		t.Error("settings.json changed")
	}
}

func TestReviewClaudeCodeMissingOnlyIsNotAMatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	out, err := runReview(t, claudeCodeReview{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Wrote ") || strings.Contains(out, "match ctdev's copy") {
		t.Errorf("output = %q", out)
	}
}

func TestReviewClaudeCodeMatchSaysSo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := runReview(t, claudeCodeReview{}); err != nil {
		t.Fatal(err)
	}
	out, err := runReview(t, claudeCodeReview{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "Claude Code settings match ctdev's copy." {
		t.Errorf("output = %q", out)
	}
}
