package component

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func claudeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func baseline(t *testing.T, name string) []byte {
	t.Helper()
	data, err := Configs.ReadFile("configs/claude-code/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func runDeploy(t *testing.T, opts ExecOpts) string {
	t.Helper()
	var out bytes.Buffer
	opts.Stdout = &out
	if err := deployClaudeCodeConfigs(opts); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	return out.String()
}

func backups(t *testing.T, path string) []string {
	t.Helper()
	b, _ := filepath.Glob(path + ".*.bak")
	return b
}

func TestClaudeCodeDeployWritesMissingFiles(t *testing.T) {
	dir := claudeHome(t)
	runDeploy(t, ExecOpts{})
	for _, name := range []string{"settings.json", "CLAUDE.md"} {
		if data, _ := os.ReadFile(filepath.Join(dir, name)); !bytes.Equal(data, baseline(t, name)) {
			t.Errorf("%s not written from the baseline", name)
		}
	}
}

func TestClaudeCodeDeployIdenticalIsNoOp(t *testing.T) {
	dir := claudeHome(t)
	runDeploy(t, ExecOpts{})
	out := runDeploy(t, ExecOpts{})
	if out != "" {
		t.Errorf("identical files produced output: %q", out)
	}
	if b := backups(t, filepath.Join(dir, "settings.json")); len(b) != 0 {
		t.Errorf("identical file was backed up: %v", b)
	}
}

func TestClaudeCodeDeployLeavesDriftWithoutForce(t *testing.T) {
	dir := claudeHome(t)
	path := filepath.Join(dir, "settings.json")
	writeFile(t, path, `{"model": "sonnet"}`, 0o644)

	out := runDeploy(t, ExecOpts{})
	if data, _ := os.ReadFile(path); string(data) != `{"model": "sonnet"}` {
		t.Errorf("drifted file was changed: %q", data)
	}
	if !strings.Contains(out, "settings.json has drifted from ctdev's copy; left unchanged") {
		t.Errorf("drift not reported: %q", out)
	}
}

func TestClaudeCodeDeployForceBacksUpAndReplaces(t *testing.T) {
	dir := claudeHome(t)
	path := filepath.Join(dir, "settings.json")
	writeFile(t, path, `{"model": "sonnet"}`, 0o600)

	runDeploy(t, ExecOpts{Force: true})
	if data, _ := os.ReadFile(path); !bytes.Equal(data, baseline(t, "settings.json")) {
		t.Errorf("not replaced: %q", data)
	}
	b := backups(t, path)
	if len(b) != 1 {
		t.Fatalf("expected one dated backup, got %v", b)
	}
	if data, _ := os.ReadFile(b[0]); string(data) != `{"model": "sonnet"}` {
		t.Errorf("backup content = %q", data)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600 kept", fi.Mode().Perm())
	}
}

func TestClaudeCodeDeployDryRunWritesNothing(t *testing.T) {
	dir := claudeHome(t)
	path := filepath.Join(dir, "settings.json")
	writeFile(t, path, `{"model": "sonnet"}`, 0o644)
	local := filepath.Join(dir, "settings.local.json")
	writeFile(t, local, `{}`, 0o644)

	out := runDeploy(t, ExecOpts{DryRun: true, Force: true})
	if data, _ := os.ReadFile(path); string(data) != `{"model": "sonnet"}` {
		t.Errorf("dry-run changed settings.json: %q", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Error("dry-run wrote CLAUDE.md")
	}
	if _, err := os.Stat(local); err != nil {
		t.Error("dry-run removed settings.local.json")
	}
	if !strings.Contains(out, "has drifted") || !strings.Contains(out, `- {"model": "sonnet"}`) {
		t.Errorf("dry-run did not report the drift and diff: %q", out)
	}
}

func TestClaudeCodeSymlinkIsDriftAndTargetUntouched(t *testing.T) {
	dir := claudeHome(t)
	target := filepath.Join(t.TempDir(), "elsewhere-CLAUDE.md")
	// Same content as the baseline: a link still counts as drift.
	writeFile(t, target, string(baseline(t, "CLAUDE.md")), 0o644)
	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}

	drifts, err := ClaudeCodeDrift()
	if err != nil {
		t.Fatal(err)
	}
	var found *ClaudeCodeFileDrift
	for i := range drifts {
		if drifts[i].Path == path {
			found = &drifts[i]
		}
	}
	if found == nil || found.LinkTarget != target {
		t.Fatalf("symlink not reported as drift: %+v", drifts)
	}

	if _, err := ReplaceClaudeCodeFile(*found); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(path); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Error("symlink was not replaced by a regular file")
	}
	if data, _ := os.ReadFile(target); !bytes.Equal(data, baseline(t, "CLAUDE.md")) {
		t.Error("symlink target was changed")
	}
}

func TestClaudeCodeSettingsLocalIsBackedUpAndRemoved(t *testing.T) {
	dir := claudeHome(t)
	local := filepath.Join(dir, "settings.local.json")
	writeFile(t, local, `{"permissions": {"allow": ["WebSearch"]}}`, 0o644)

	runDeploy(t, ExecOpts{})
	if _, err := os.Stat(local); !os.IsNotExist(err) {
		t.Error("settings.local.json still present")
	}
	b := backups(t, local)
	if len(b) != 1 {
		t.Fatalf("expected one dated backup, got %v", b)
	}
	if data, _ := os.ReadFile(b[0]); string(data) != `{"permissions": {"allow": ["WebSearch"]}}` {
		t.Errorf("backup content = %q", data)
	}
}

func TestClaudeCodeDependsOnCcstatusline(t *testing.T) {
	c := FindByName("claude-code")
	if c == nil {
		t.Fatal("claude-code missing from registry")
	}
	if len(c.Dependencies) != 1 || c.Dependencies[0] != "ccstatusline" {
		t.Errorf("expected claude-code to depend on ccstatusline, got %v", c.Dependencies)
	}
}

func TestClaudeCodeBaselineRunsCcstatusline(t *testing.T) {
	if !strings.Contains(string(baseline(t, "settings.json")), `"command": "ccstatusline"`) {
		t.Error("baseline settings.json must point statusLine at ccstatusline")
	}
	if _, err := Configs.ReadFile("configs/claude-code/settings.local.json"); err == nil {
		t.Error("settings.local.json is still shipped")
	}
}

func TestLineDiff(t *testing.T) {
	got := lineDiff("a\nb\nc\n", "a\nB\nc\n")
	want := "  a\n- b\n+ B\n  c\n"
	if got != want {
		t.Errorf("lineDiff =\n%s\nwant\n%s", got, want)
	}
}
