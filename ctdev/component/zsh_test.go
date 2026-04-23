package component

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// Phase 2 config deploys previously used a map whose iteration order was
// randomized by Go. A user-visible symptom was warning messages printing in
// different orders across runs. Locking the expected order prevents regression.
func TestZshInstall_DeployOrderIsStable(t *testing.T) {
	// A HOME without .oh-my-zsh forces the installer to print Phase 1 steps
	// in dry-run; we care only about the Phase 2 deploy lines.
	t.Setenv("HOME", t.TempDir())

	expected := []string{
		"deploy .zshrc",
		"deploy aliases.zsh",
		"deploy exports.zsh",
		"deploy path.zsh",
	}

	var first bytes.Buffer
	if err := zshInstall(context.Background(), ExecOpts{DryRun: true, Force: true, Stdout: &first}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	assertOrder(t, first.String(), expected, "first")

	// Re-run in a fresh HOME and confirm the same order.
	t.Setenv("HOME", t.TempDir())
	var second bytes.Buffer
	if err := zshInstall(context.Background(), ExecOpts{DryRun: true, Force: true, Stdout: &second}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	assertOrder(t, second.String(), expected, "second")
}

func assertOrder(t *testing.T, out string, expected []string, label string) {
	t.Helper()
	lastIdx := -1
	for _, marker := range expected {
		idx := strings.Index(out, marker)
		if idx == -1 {
			t.Errorf("%s run: missing %q in output:\n%s", label, marker, out)
			return
		}
		if idx <= lastIdx {
			t.Errorf("%s run: %q appeared before an earlier expected marker", label, marker)
		}
		lastIdx = idx
	}
}
