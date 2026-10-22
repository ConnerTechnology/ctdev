package claudecli_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/claudecli"
	"github.com/ConnerTechnology/ctdev/ctdev/claudecli/claudeclitest"
)

func TestConfigFileFollowsClaudeConfigDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/tmp/example-config")
	if got := claudecli.ConfigFile(); got != "/tmp/example-config/.claude.json" {
		t.Errorf("got %s", got)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("HOME", "/tmp/example-home")
	if got := claudecli.ConfigFile(); got != "/tmp/example-home/.claude.json" {
		t.Errorf("got %s", got)
	}
}

func TestLocalMCPServer(t *testing.T) {
	f := claudeclitest.New(t)
	root := t.TempDir()
	if _, ok, err := claudecli.LocalMCPServer(root, "linear"); ok || err != nil {
		t.Fatalf("no config file: ok=%v err=%v", ok, err)
	}
	f.SetLocalMCPServer(root, "linear", `{"type":"http","url":"https://mcp.linear.app/mcp"}`)
	entry, ok, err := claudecli.LocalMCPServer(root, "linear")
	if !ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !claudecli.SameEntry(entry, json.RawMessage(`{ "url": "https://mcp.linear.app/mcp", "type": "http" }`)) {
		t.Errorf("entry %s", entry)
	}
	if _, ok, _ := claudecli.LocalMCPServer(t.TempDir(), "linear"); ok {
		t.Error("another project's server leaked")
	}
	if _, ok, _ := claudecli.LocalMCPServer(root, "atlassian"); ok {
		t.Error("another server name matched")
	}
}

func TestAddLocalMCPServerRunsInRepoAtLocalScope(t *testing.T) {
	f := claudeclitest.New(t)
	root := t.TempDir()
	if err := claudecli.AddLocalMCPServer(context.Background(), root, "linear", json.RawMessage("{\n  \"type\": \"http\"\n}")); err != nil {
		t.Fatal(err)
	}
	want := []string{`mcp add-json linear {"type":"http"} --scope local`}
	if got := f.Calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("calls %q, want %q", got, want)
	}
}

func TestFailureCarriesClaudesMessage(t *testing.T) {
	f := claudeclitest.New(t)
	f.Reply("mcp-remove", "", 1)
	f.Reply("plugin-marketplace-add", `{"command":"marketplace-add","outcome":"failed","message":"Example failure"}`+"\n", 1)
	_, err := claudecli.AddMarketplace(context.Background(), t.TempDir(), "Example/repo@v1")
	if err == nil || !strings.Contains(err.Error(), "Example failure") {
		t.Errorf("got %v", err)
	}
	if err := claudecli.RemoveLocalMCPServer(context.Background(), t.TempDir(), "x"); err == nil {
		t.Error("a non-zero exit should be an error")
	}
}

func TestInstallPluginReportsStale(t *testing.T) {
	f := claudeclitest.New(t)
	f.Reply("plugin-install", "Some progress\n"+`{"outcome":"ok","message":"already installed","installedVersion":"1.0.0","availableVersion":"1.1.0"}`+"\n", 0)
	res, err := claudecli.InstallPlugin(context.Background(), t.TempDir(), "example@example")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Stale() || res.Message != "already installed" {
		t.Errorf("got %+v", res)
	}
	if got := f.Calls(); got[0] != "plugin install example@example --scope local --json" {
		t.Errorf("call %q", got[0])
	}
	if (claudecli.InstallResult{}).Stale() {
		t.Error("a fresh install is not stale")
	}
}

func TestLocalPluginInstalled(t *testing.T) {
	f := claudeclitest.New(t)
	root := t.TempDir()
	other := t.TempDir()
	list := []map[string]any{
		{"id": "example@example", "scope": "local", "enabled": true, "projectPath": other},
		{"id": "example@example", "scope": "user", "enabled": true},
	}
	b, _ := json.MarshalIndent(list, "", "  ")
	f.Reply("plugin-list", string(b), 0)
	if ok, err := claudecli.LocalPluginInstalled(context.Background(), root, "example@example"); ok || err != nil {
		t.Fatalf("installed elsewhere only: ok=%v err=%v", ok, err)
	}
	list = append(list, map[string]any{"id": "example@example", "scope": "local", "enabled": true, "projectPath": root})
	b, _ = json.MarshalIndent(list, "", "  ")
	f.Reply("plugin-list", string(b), 0)
	if ok, err := claudecli.LocalPluginInstalled(context.Background(), root, "example@example"); !ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}
