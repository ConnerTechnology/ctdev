package component

import (
	"bytes"
	"context"
	"os/exec"
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
		"deploy claude.zsh",
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

// The claude wrapper caps every session at a fixed target per known RAM size
// (24G on a 64 GB box, the figure the desktop-freeze guidance was written for),
// so a runaway test suite is killed inside the cap instead of taking the
// desktop down. A machine reports a little under its nominal size, so the
// lookup snaps to the nearest entry. The rule is a pure zsh function so it can
// be checked without systemd.
func TestClaudeMemoryMax(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	src, err := Configs.ReadFile("configs/zsh/claude.zsh")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		totalKB string
		want    string
	}{
		{"4000000", "4G"},     // below the table: smallest entry
		{"8000000", "4G"},     // 8 GB
		{"12000000", "6G"},    // 12 GB
		{"16210000", "8G"},    // 16 GB as reported by the kernel
		{"24500000", "12G"},   // 24 GB
		{"32700000", "16G"},   // 32 GB
		{"49000000", "20G"},   // 48 GB
		{"65805020", "24G"},   // 64 GB (reports as 62.8 GiB)
		{"98000000", "36G"},   // 96 GB
		{"131500000", "48G"},  // 128 GB
		{"197000000", "64G"},  // 192 GB
		{"263000000", "96G"},  // 256 GB
		{"1000000000", "96G"}, // above the table: largest entry
	}
	for _, c := range cases {
		out, err := exec.Command(zsh, "-c", string(src)+"\n_claude_memory_max "+c.totalKB).Output()
		if err != nil {
			t.Fatalf("MemTotal %s: %v", c.totalKB, err)
		}
		if got := strings.TrimSpace(string(out)); got != c.want {
			t.Errorf("MemTotal %s kB: got %s, want %s", c.totalKB, got, c.want)
		}
	}
}

func TestZshInstall_DeploysClaudeWrapper(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out bytes.Buffer
	if err := zshInstall(context.Background(), ExecOpts{DryRun: true, Force: true, Stdout: &out}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "deploy claude.zsh") {
		t.Errorf("claude.zsh not deployed:\n%s", out.String())
	}
}
