// Package claudecli drives the `claude` CLI for the per-repo Claude Code setup
// ctdev does: MCP servers and the plugin, always at local scope, so nothing it
// does lands in a file the repo commits (CTD-109). Writes go through the CLI,
// which owns its files; reads that the CLI has no machine-readable form for
// look at ~/.claude.json directly.
package claudecli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Bin is the command run. Tests put a fake one first on PATH.
const Bin = "claude"

// ConfigFile is where Claude Code keeps per-project state, including
// local-scope MCP servers: .claude.json in CLAUDE_CONFIG_DIR when that is set,
// else in the home directory.
func ConfigFile() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, ".claude.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude.json")
}

// LocalMCPServer returns the local-scope MCP server name for the project at
// root, as its JSON entry. ok is false when there is none.
func LocalMCPServer(root, name string) (entry json.RawMessage, ok bool, err error) {
	data, err := os.ReadFile(ConfigFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var doc struct {
		Projects map[string]struct {
			MCPServers map[string]json.RawMessage `json:"mcpServers"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, false, fmt.Errorf("%s: %w", ConfigFile(), err)
	}
	entry, ok = doc.Projects[root].MCPServers[name]
	return entry, ok, nil
}

// SameEntry reports whether two JSON documents, such as two MCP server
// entries, say the same thing whatever their key order and spacing.
func SameEntry(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return bytes.Equal(xb, yb)
}

// AddLocalMCPServer adds an MCP server at local scope for the project at root:
// private to this machine and this repo, stored in ~/.claude.json.
func AddLocalMCPServer(ctx context.Context, root, name string, entry json.RawMessage) error {
	var compact bytes.Buffer
	if err := json.Compact(&compact, entry); err != nil {
		return err
	}
	_, err := run(ctx, root, "mcp", "add-json", name, compact.String(), "--scope", "local")
	return err
}

// RemoveLocalMCPServer removes a local-scope MCP server for the project at root.
func RemoveLocalMCPServer(ctx context.Context, root, name string) error {
	_, err := run(ctx, root, "mcp", "remove", name, "--scope", "local")
	return err
}

// AddMarketplace declares a plugin marketplace in the repo's
// .claude/settings.local.json. Adding the same source again changes nothing;
// a different source repoints the marketplace. env is added to the
// environment the CLI, and the git clone it runs, see.
func AddMarketplace(ctx context.Context, root, source string, env ...string) (message string, err error) {
	res, err := runJSONEnv(ctx, root, env, "plugin", "marketplace", "add", source, "--scope", "local", "--json")
	return res.Message, err
}

// GitHubCredentialEnv makes git, for one command, ask gh for github.com
// credentials, after any helper the user's git config already names. It is
// how a private marketplace clones over the machine's gh login without
// touching ~/.gitconfig.
func GitHubCredentialEnv() []string {
	return []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_0=!gh auth git-credential",
	}
}

// InstallResult is what `claude plugin install --json` reports.
type InstallResult struct {
	Message          string `json:"message"`
	InstalledVersion string `json:"installedVersion"`
	AvailableVersion string `json:"availableVersion"`
}

// Stale reports whether the plugin was already installed at an older version
// than its marketplace now offers.
func (r InstallResult) Stale() bool {
	return r.InstalledVersion != "" && r.AvailableVersion != "" && r.InstalledVersion != r.AvailableVersion
}

// InstallPlugin installs plugin (name@marketplace) at local scope for the
// project at root. Installing it again changes nothing.
func InstallPlugin(ctx context.Context, root, plugin string) (InstallResult, error) {
	res, err := runJSON(ctx, root, "plugin", "install", plugin, "--scope", "local", "--json")
	return res.InstallResult, err
}

// UpdatePlugin brings a local-scope plugin up to its marketplace's version.
func UpdatePlugin(ctx context.Context, root, plugin string) error {
	_, err := run(ctx, root, "plugin", "update", plugin, "--scope", "local")
	return err
}

// LocalPluginInstalled reports whether plugin is installed and enabled at
// local scope for the project at root.
func LocalPluginInstalled(ctx context.Context, root, plugin string) (bool, error) {
	out, err := run(ctx, root, "plugin", "list", "--json")
	if err != nil {
		return false, err
	}
	var list []struct {
		ID          string `json:"id"`
		Scope       string `json:"scope"`
		Enabled     bool   `json:"enabled"`
		ProjectPath string `json:"projectPath"`
	}
	if err := json.Unmarshal(lastJSON(out), &list); err != nil {
		return false, fmt.Errorf("claude plugin list --json: %w", err)
	}
	for _, p := range list {
		if p.ID == plugin && p.Scope == "local" && p.Enabled && samePath(p.ProjectPath, root) {
			return true, nil
		}
	}
	return false, nil
}

type jsonResult struct {
	Outcome string `json:"outcome"`
	InstallResult
}

// runJSON runs a --json subcommand, whose last stdout line is its result.
func runJSON(ctx context.Context, root string, args ...string) (jsonResult, error) {
	return runJSONEnv(ctx, root, nil, args...)
}

func runJSONEnv(ctx context.Context, root string, env []string, args ...string) (jsonResult, error) {
	out, err := runEnv(ctx, root, env, args...)
	var res jsonResult
	if jerr := json.Unmarshal(lastJSON(out), &res); jerr == nil && res.Outcome != "" && res.Outcome != "ok" {
		return res, fmt.Errorf("claude %s: %s", strings.Join(args[:2], " "), res.Message)
	}
	return res, err
}

// lastJSON is the last non-empty line of out, or all of it when out is one
// JSON document over several lines.
func lastJSON(out []byte) []byte {
	trimmed := bytes.TrimSpace(out)
	if json.Valid(trimmed) {
		return trimmed
	}
	lines := bytes.Split(trimmed, []byte("\n"))
	return lines[len(lines)-1]
}

func run(ctx context.Context, root string, args ...string) ([]byte, error) {
	return runEnv(ctx, root, nil, args...)
}

func runEnv(ctx context.Context, root string, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, Bin, args...)
	cmd.Dir = root
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(string(out))
		}
		if msg != "" {
			return out, fmt.Errorf("claude %s: %s", strings.Join(args[:2], " "), msg)
		}
		return out, fmt.Errorf("claude %s: %w", strings.Join(args[:2], " "), err)
	}
	return out, nil
}

func samePath(a, b string) bool {
	if a == b {
		return true
	}
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}
