package component

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
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

	if err := updateClaudeStatusLine(quietOpts, true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	wantCcstatuslineStatusLine(t, readClaudeSettings(t, path))
}

func TestClaudeStatusLineKeepsOtherKeys(t *testing.T) {
	path := claudeSettingsPath(t)
	writeClaudeSettings(t, path, `{"model": "opus", "env": {"EXAMPLE": "a<b>&c"}}`)

	if err := updateClaudeStatusLine(quietOpts, true); err != nil {
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

	if err := updateClaudeStatusLine(quietOpts, true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	wantCcstatuslineStatusLine(t, readClaudeSettings(t, path))
}

func TestClaudeStatusLineLeavesInvalidJSONAlone(t *testing.T) {
	path := claudeSettingsPath(t)
	writeClaudeSettings(t, path, `{"model": "opus",`)

	if err := updateClaudeStatusLine(quietOpts, true); err == nil {
		t.Error("expected an error for invalid JSON")
	}
	if data, _ := os.ReadFile(path); string(data) != `{"model": "opus",` {
		t.Errorf("invalid settings were rewritten: %q", data)
	}
}

func TestClaudeStatusLineDryRunWritesNothing(t *testing.T) {
	path := claudeSettingsPath(t)

	if err := updateClaudeStatusLine(sysutil.Opts{Stdout: io.Discard, DryRun: true}, true); err != nil {
		t.Fatalf("dry-run enable: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("dry-run created %s", path)
	}
}

func TestClaudeStatusLineRemoveKeepsOtherKeys(t *testing.T) {
	path := claudeSettingsPath(t)
	writeClaudeSettings(t, path, `{"model": "opus", "statusLine": {"type": "command", "command": "ccstatusline"}}`)

	if err := updateClaudeStatusLine(quietOpts, false); err != nil {
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

	if err := updateClaudeStatusLine(quietOpts, false); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("remove created %s", path)
	}
}
