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

func runWriteMissing(t *testing.T, opts ExecOpts) string {
	t.Helper()
	var out bytes.Buffer
	opts.Stdout = &out
	if err := writeMissingClaudeCodeFiles(opts); err != nil {
		t.Fatalf("write missing: %v", err)
	}
	return out.String()
}

func backups(t *testing.T, path string) []string {
	t.Helper()
	b, _ := filepath.Glob(path + ".*.bak")
	return b
}

func TestClaudeCodeInstallWritesMissingFiles(t *testing.T) {
	dir := claudeHome(t)
	runWriteMissing(t, ExecOpts{})
	for _, name := range []string{"settings.json", "CLAUDE.md"} {
		if data, _ := os.ReadFile(filepath.Join(dir, name)); !bytes.Equal(data, baseline(t, name)) {
			t.Errorf("%s not written from the baseline", name)
		}
	}
	drifts, err := ClaudeCodeDrift()
	if err != nil || len(drifts) != 0 {
		t.Errorf("after writing, drift = %v, %v", drifts, err)
	}
}

// The install runs under the progress screen; a drifted file is the review
// step's business, so install must leave it exactly as it is, even with --force.
func TestClaudeCodeInstallLeavesDriftedFile(t *testing.T) {
	dir := claudeHome(t)
	path := filepath.Join(dir, "settings.json")
	writeFile(t, path, `{"model": "sonnet"}`, 0o644)

	runWriteMissing(t, ExecOpts{Force: true})
	if data, _ := os.ReadFile(path); string(data) != `{"model": "sonnet"}` {
		t.Errorf("drifted file was changed: %q", data)
	}
	if b := backups(t, path); len(b) != 0 {
		t.Errorf("drifted file was backed up: %v", b)
	}
}

func TestClaudeCodeInstallDryRunWritesNothing(t *testing.T) {
	dir := claudeHome(t)
	runWriteMissing(t, ExecOpts{DryRun: true})
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); !os.IsNotExist(err) {
		t.Error("dry-run wrote settings.json")
	}
}

func TestClaudeCodeReplaceKeepsModeAndBacksUp(t *testing.T) {
	dir := claudeHome(t)
	path := filepath.Join(dir, "settings.json")
	writeFile(t, path, `{"model": "sonnet"}`, 0o600)

	drifts, err := ClaudeCodeDrift()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range drifts {
		if d.Path != path {
			continue
		}
		backup, err := ReplaceClaudeCodeFile(d)
		if err != nil {
			t.Fatal(err)
		}
		if data, _ := os.ReadFile(backup); string(data) != `{"model": "sonnet"}` {
			t.Errorf("backup content = %q", data)
		}
	}
	if data, _ := os.ReadFile(path); !bytes.Equal(data, baseline(t, "settings.json")) {
		t.Errorf("not replaced: %q", data)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600 kept", fi.Mode().Perm())
	}
}

func TestClaudeCodeDriftReportsUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads any file")
	}
	dir := claudeHome(t)
	path := filepath.Join(dir, "settings.json")
	writeFile(t, path, `{}`, 0o000)

	if _, err := ClaudeCodeDrift(); err == nil {
		t.Error("an unreadable file must be an error, not an empty file")
	}
}

func TestClaudeCodeSymlinkIsDriftAndTargetUntouched(t *testing.T) {
	dir := claudeHome(t)
	target := filepath.Join(t.TempDir(), "elsewhere-CLAUDE.md")
	// Different content, so a write through the link would show up in the target.
	writeFile(t, target, "# someone else's file\n", 0o644)
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
	if data, _ := os.ReadFile(target); string(data) != "# someone else's file\n" {
		t.Error("symlink target was changed")
	}
	if b := backups(t, target); len(b) != 0 {
		t.Errorf("backup made beside the symlink target: %v", b)
	}
}

func TestClaudeCodeIdenticalSymlinkSaysSo(t *testing.T) {
	dir := claudeHome(t)
	target := filepath.Join(t.TempDir(), "same-CLAUDE.md")
	writeFile(t, target, string(baseline(t, "CLAUDE.md")), 0o644)
	if err := os.Symlink(target, filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
	drifts, err := ClaudeCodeDrift()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range drifts {
		if d.LinkTarget == target && !strings.Contains(d.Diff, "contents are identical") {
			t.Errorf("diff = %q", d.Diff)
		}
	}
}

// ccstatusline does nothing without Claude Code, so claude-code makes sure it
// is installed; the baseline's statusLine is what turns it on.
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
