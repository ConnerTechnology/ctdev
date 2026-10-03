package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/component"
)

func driftedClaudeSettings(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// CLAUDE.md matches, settings.json has drifted.
	md, _ := component.Configs.ReadFile("configs/claude-code/CLAUDE.md")
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), md, 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"model": "sonnet"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigureClaudeCodeDeclineLeavesFile(t *testing.T) {
	path := driftedClaudeSettings(t)
	feedStdin(t, "n\n")

	out := captureStdout(t, func() {
		if err := configureClaudeCode(context.Background()); err != nil {
			t.Errorf("configure: %v", err)
		}
	})
	if !strings.Contains(out, `- {"model": "sonnet"}`) || !strings.Contains(out, `+   "model": "opus",`) {
		t.Errorf("diff not shown:\n%s", out)
	}
	if data, _ := os.ReadFile(path); string(data) != `{"model": "sonnet"}`+"\n" {
		t.Errorf("declined file was changed: %q", data)
	}
	if strings.Contains(out, "CLAUDE.md has drifted") {
		t.Error("identical CLAUDE.md reported as drifted")
	}
}

func TestConfigureClaudeCodeAcceptBacksUpAndReplaces(t *testing.T) {
	path := driftedClaudeSettings(t)
	feedStdin(t, "y\n")

	captureStdout(t, func() {
		if err := configureClaudeCode(context.Background()); err != nil {
			t.Errorf("configure: %v", err)
		}
	})
	want, _ := component.Configs.ReadFile("configs/claude-code/settings.json")
	if data, _ := os.ReadFile(path); string(data) != string(want) {
		t.Errorf("settings not replaced:\n%s", data)
	}
	backups, _ := filepath.Glob(path + ".*.bak")
	if len(backups) != 1 {
		t.Fatalf("expected one dated backup, got %v", backups)
	}
	if data, _ := os.ReadFile(backups[0]); string(data) != `{"model": "sonnet"}`+"\n" {
		t.Errorf("backup content = %q", data)
	}
}

func TestClaudeCodeHasConfigureStep(t *testing.T) {
	if !componentHasConfigure("claude-code") {
		t.Error("ctdev install claude-code must run the drift review after installing")
	}
}
