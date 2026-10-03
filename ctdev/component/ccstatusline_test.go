package component

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

func claudeSettingsPath(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return filepath.Join(home, ".claude", "settings.json")
}

func readClaudeSettings(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	settings := map[string]any{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings are not valid JSON: %v", err)
	}
	return settings
}

func writeClaudeSettings(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func wantCcstatuslineStatusLine(t *testing.T, settings map[string]any) {
	t.Helper()
	sl, ok := settings["statusLine"].(map[string]any)
	if !ok {
		t.Fatalf("statusLine missing or not an object: %v", settings["statusLine"])
	}
	if sl["type"] != "command" || sl["command"] != "ccstatusline" || sl["padding"] != float64(0) || sl["refreshInterval"] != float64(10) {
		t.Errorf("statusLine = %v", sl)
	}
}

var quietOpts = sysutil.Opts{Stdout: io.Discard}

func TestClaudeStatusLineCreatesMissingFile(t *testing.T) {
	path := claudeSettingsPath(t)

	if err := enableClaudeStatusLine(quietOpts); err != nil {
		t.Fatalf("enable: %v", err)
	}
	wantCcstatuslineStatusLine(t, readClaudeSettings(t, path))
}

func TestClaudeStatusLineKeepsOtherKeys(t *testing.T) {
	path := claudeSettingsPath(t)
	writeClaudeSettings(t, path, `{"model": "opus", "env": {"EXAMPLE": "a<b>&c"}}`)

	if err := enableClaudeStatusLine(quietOpts); err != nil {
		t.Fatalf("enable: %v", err)
	}
	settings := readClaudeSettings(t, path)
	wantCcstatuslineStatusLine(t, settings)
	if settings["model"] != "opus" {
		t.Errorf("model = %v, want opus", settings["model"])
	}
	if env, _ := settings["env"].(map[string]any); env["EXAMPLE"] != "a<b>&c" {
		t.Errorf("env = %v", settings["env"])
	}
}

func TestClaudeStatusLineReplacesExistingEntry(t *testing.T) {
	path := claudeSettingsPath(t)
	writeClaudeSettings(t, path, `{"statusLine": {"type": "command", "command": "other-tool"}}`)

	if err := enableClaudeStatusLine(quietOpts); err != nil {
		t.Fatalf("enable: %v", err)
	}
	wantCcstatuslineStatusLine(t, readClaudeSettings(t, path))
}

func TestClaudeStatusLineLeavesInvalidJSONAlone(t *testing.T) {
	path := claudeSettingsPath(t)
	writeClaudeSettings(t, path, `{"model": "opus",`)

	if err := enableClaudeStatusLine(quietOpts); err == nil {
		t.Error("expected an error for invalid JSON")
	}
	if data, _ := os.ReadFile(path); string(data) != `{"model": "opus",` {
		t.Errorf("invalid settings were rewritten: %q", data)
	}
}

func TestClaudeStatusLineDryRunWritesNothing(t *testing.T) {
	path := claudeSettingsPath(t)

	if err := enableClaudeStatusLine(sysutil.Opts{Stdout: io.Discard, DryRun: true}); err != nil {
		t.Fatalf("dry-run enable: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("dry-run created %s", path)
	}
}

func TestClaudeStatusLineRemoveKeepsOtherKeys(t *testing.T) {
	path := claudeSettingsPath(t)
	writeClaudeSettings(t, path, `{"model": "opus", "statusLine": {"type": "command", "command": "ccstatusline"}}`)

	if err := disableClaudeStatusLine(quietOpts); err != nil {
		t.Fatalf("remove: %v", err)
	}
	settings := readClaudeSettings(t, path)
	if _, ok := settings["statusLine"]; ok {
		t.Error("statusLine still present")
	}
	if settings["model"] != "opus" {
		t.Errorf("model = %v, want opus", settings["model"])
	}
}

func TestClaudeStatusLineRemoveWithoutFileIsNoOp(t *testing.T) {
	path := claudeSettingsPath(t)

	if err := disableClaudeStatusLine(quietOpts); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("remove created %s", path)
	}
}

func TestClaudeStatusLineLeavesNonObjectAlone(t *testing.T) {
	for _, content := range []string{`null`, `[]`, `"x"`} {
		path := claudeSettingsPath(t)
		writeClaudeSettings(t, path, content)

		if err := enableClaudeStatusLine(quietOpts); err == nil {
			t.Errorf("%s: expected an error", content)
		}
		if data, _ := os.ReadFile(path); string(data) != content {
			t.Errorf("%s: settings were rewritten: %q", content, data)
		}
	}
}

func TestClaudeStatusLineKeepsOtherValuesExact(t *testing.T) {
	path := claudeSettingsPath(t)
	writeClaudeSettings(t, path, `{"big": 9007199254740993, "ratio": 1.0}`)

	if err := enableClaudeStatusLine(quietOpts); err != nil {
		t.Fatalf("enable: %v", err)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{`"big": 9007199254740993`, `"ratio": 1.0`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("settings lost %s:\n%s", want, data)
		}
	}
}

// A file that already has the entry is not rewritten, so its key order and
// layout survive and no backup piles up.
func TestClaudeStatusLineAlreadySetIsNoOp(t *testing.T) {
	path := claudeSettingsPath(t)
	content := `{"statusLine": {"type": "command", "command": "ccstatusline", "padding": 0, "refreshInterval": 10}, "model": "opus"}`
	writeClaudeSettings(t, path, content)

	var out strings.Builder
	if err := enableClaudeStatusLine(sysutil.Opts{Stdout: &out, DryRun: true}); err != nil {
		t.Fatalf("dry-run enable: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("dry-run reported a change: %q", out.String())
	}
	if err := enableClaudeStatusLine(quietOpts); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != content {
		t.Errorf("settings were rewritten: %q", data)
	}
}

func TestClaudeStatusLineWritesThroughSymlink(t *testing.T) {
	path := claudeSettingsPath(t)
	target := filepath.Join(t.TempDir(), "dotfiles-settings.json")
	if err := os.WriteFile(target, []byte(`{"model": "opus"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}

	if err := enableClaudeStatusLine(quietOpts); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if fi, err := os.Lstat(path); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symlink was replaced: %v", err)
	}
	wantCcstatuslineStatusLine(t, readClaudeSettings(t, target))
}

func TestClaudeStatusLineKeepsFileMode(t *testing.T) {
	path := claudeSettingsPath(t)
	writeClaudeSettings(t, path, `{"model": "opus"}`)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := enableClaudeStatusLine(quietOpts); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}
}
