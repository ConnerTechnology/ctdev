package component

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// In a dry run nothing is installed, so on a computer without Node the node
// component in the same run can't have put npm in place yet. The npm-based
// components report the command they would run instead of failing.
func TestNpmComponents_DryRunWithoutNpm(t *testing.T) {
	cases := []struct {
		name    string
		install func(context.Context, ExecOpts) error
		want    string
	}{
		{"ccstatusline", ccstatuslineInstall, "[dry-run] npm install -g ccstatusline"},
		{"ccusage", FindByName("ccusage").GoInstall, "[dry-run] npm install -g ccusage"},
		{"codex", FindByName("codex").GoInstall, "[dry-run] npm install -g @openai/codex"},
		{"devcontainer", devcontainerInstall, "[dry-run] npm install -g @devcontainers/cli"},
		{"gemini-cli", FindByName("gemini-cli").GoInstall, "[dry-run] npm install -g @google/gemini-cli"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir()) // no ~/.nodenv/shims/npm
			t.Setenv("PATH", t.TempDir()) // no npm on PATH
			var out bytes.Buffer
			err := tc.install(context.Background(), ExecOpts{DryRun: true, Force: true, Stdout: &out})
			if err != nil {
				t.Fatalf("dry run without npm: unexpected error: %v", err)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Errorf("expected %q in output; got:\n%s", tc.want, out.String())
			}
		})
	}
}

// A real run still fails clearly when npm is missing.
func TestNpmPath_RealRunWithoutNpmFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	if _, err := npmPath(execOpts(ExecOpts{})); err == nil {
		t.Fatal("expected an error when npm is missing outside a dry run")
	}
}

func TestNpmGlobalComponents_DryRunUninstall(t *testing.T) {
	cases := map[string]string{
		"ccusage":    "[dry-run] npm uninstall -g ccusage",
		"codex":      "[dry-run] npm uninstall -g @openai/codex",
		"gemini-cli": "[dry-run] npm uninstall -g @google/gemini-cli",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("PATH", t.TempDir())
			var out bytes.Buffer
			if err := FindByName(name).GoUninstall(context.Background(), ExecOpts{DryRun: true, Stdout: &out}); err != nil {
				t.Fatalf("dry-run uninstall: %v", err)
			}
			if !strings.Contains(out.String(), want) {
				t.Errorf("expected %q in output; got:\n%s", want, out.String())
			}
		})
	}
}
