package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
)

// The repo files `ctdev configure linear` writes, relative to the repo root.
const (
	MCPJSONFile       = ".mcp.json"
	SettingsLocalFile = ".claude/settings.local.json"
)

// ServerName is the .mcp.json server key, and the name pre-approved in
// settings.local.json.
const ServerName = "linear"

// MCPURL is Linear's hosted MCP server.
const MCPURL = "https://mcp.linear.app/mcp"

const headersHelperPrefix = "ctdev linear mcp-headers --workspace "

var headersHelperPattern = regexp.MustCompile(`^ctdev linear mcp-headers --workspace ([a-z0-9][a-z0-9-]*)$`)

// HeadersHelper is the .mcp.json headersHelper command for a workspace.
func HeadersHelper(workspace string) string {
	return headersHelperPrefix + workspace
}

// mcpServerEntry is the .mcp.json "linear" server for a workspace.
type mcpServerEntry struct {
	Type          string `json:"type"`
	URL           string `json:"url"`
	HeadersHelper string `json:"headersHelper"`
}

// MCPEntry is the "linear" server entry for a workspace, as indented JSON.
func MCPEntry(workspace string) json.RawMessage {
	b, _ := json.MarshalIndent(mcpServerEntry{"http", MCPURL, HeadersHelper(workspace)}, "", "  ")
	return b
}

// CurrentWorkspace reads the workspace a repo's .mcp.json points at. ok is
// false when there is no linear server or its headersHelper is not ours.
func CurrentWorkspace(mcpJSON []byte) (workspace string, ok bool) {
	var doc struct {
		MCPServers map[string]struct {
			HeadersHelper string `json:"headersHelper"`
		} `json:"mcpServers"`
	}
	if json.Unmarshal(mcpJSON, &doc) != nil {
		return "", false
	}
	m := headersHelperPattern.FindStringSubmatch(strings.TrimSpace(doc.MCPServers[ServerName].HeadersHelper))
	if m == nil {
		return "", false
	}
	return m[1], true
}

// MCPPlan is what writing the linear server into a .mcp.json would do.
type MCPPlan struct {
	// Content is the whole new file.
	Content []byte
	// Previous is the linear entry the file already has, nil when it has none.
	Previous json.RawMessage
	// Changed is false when the file already has exactly this entry.
	Changed bool
}

// parseMCPJSON reads a .mcp.json (nil or empty for none) into its top-level
// object and its mcpServers object.
func parseMCPJSON(existing []byte) (root, servers *orderedObject, err error) {
	root, servers = &orderedObject{}, &orderedObject{}
	if len(bytes.TrimSpace(existing)) > 0 {
		if err := json.Unmarshal(existing, root); err != nil {
			return nil, nil, fmt.Errorf("%s is not a valid JSON object: %w", MCPJSONFile, err)
		}
	}
	if raw, ok := root.get("mcpServers"); ok {
		if err := json.Unmarshal(raw, servers); err != nil {
			return nil, nil, fmt.Errorf("%s: mcpServers is not a valid JSON object: %w", MCPJSONFile, err)
		}
	}
	return root, servers, nil
}

// ValidateMCPJSON checks that an existing .mcp.json (nil or empty for none) is
// one ctdev can merge into: a JSON object whose mcpServers, if present, is an
// object too.
func ValidateMCPJSON(existing []byte) error {
	_, _, err := parseMCPJSON(existing)
	return err
}

// HasLinearServer reports whether a .mcp.json has a linear server of any kind,
// ctdev's or not.
func HasLinearServer(existing []byte) bool {
	_, servers, err := parseMCPJSON(existing)
	if err != nil {
		return false
	}
	_, ok := servers.get(ServerName)
	return ok
}

// PlanMCPJSON merges the linear server for a workspace into an existing
// .mcp.json (nil or empty for a new file). Every other server and key, their
// order and their values, is kept.
func PlanMCPJSON(existing []byte, workspace string) (MCPPlan, error) {
	root, servers, err := parseMCPJSON(existing)
	if err != nil {
		return MCPPlan{}, err
	}
	entry := MCPEntry(workspace)
	plan := MCPPlan{Changed: true}
	if prev, ok := servers.get(ServerName); ok {
		plan.Previous = prev
		plan.Changed = !jsonEqual(prev, entry)
	}
	servers.set(ServerName, entry)
	serversJSON, err := marshalLiteral(servers, "")
	if err != nil {
		return MCPPlan{}, err
	}
	root.set("mcpServers", serversJSON)
	plan.Content, err = indentJSON(root)
	return plan, err
}

// MergeSettingsLocal adds "linear" to enabledMcpjsonServers in a
// settings.local.json (nil or empty for a new file), keeping everything else.
// changed is false when it is already there.
func MergeSettingsLocal(existing []byte) (content []byte, changed bool, err error) {
	root := &orderedObject{}
	if len(bytes.TrimSpace(existing)) > 0 {
		if err := json.Unmarshal(existing, root); err != nil {
			return nil, false, fmt.Errorf("%s is not a JSON object: %w", SettingsLocalFile, err)
		}
	}
	var enabled []json.RawMessage
	if raw, ok := root.get("enabledMcpjsonServers"); ok {
		if err := json.Unmarshal(raw, &enabled); err != nil {
			return nil, false, fmt.Errorf("%s: enabledMcpjsonServers is not a list: %w", SettingsLocalFile, err)
		}
	}
	for _, e := range enabled {
		var s string
		if json.Unmarshal(e, &s) == nil && s == ServerName {
			content, err = indentJSON(root)
			return content, false, err
		}
	}
	name, _ := json.Marshal(ServerName)
	enabled = append(enabled, name)
	list, err := marshalLiteral(enabled, "")
	if err != nil {
		return nil, false, err
	}
	root.set("enabledMcpjsonServers", list)
	content, err = indentJSON(root)
	return content, true, err
}

// indentJSON is the file form: 2-space indent and a trailing newline.
func indentJSON(v any) ([]byte, error) {
	return marshalLiteral(v, "  ")
}

// marshalLiteral encodes without HTML escaping, so a preserved value with &,
// < or > in it comes back out as written rather than as & and friends.
// indent "" gives compact output with no trailing newline.
func marshalLiteral(v any, indent string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if indent != "" {
		enc.SetIndent("", indent)
	}
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	if indent == "" {
		return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
	}
	return buf.Bytes(), nil
}

func jsonEqual(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

// orderedObject is a JSON object that remembers its key order, so a merge
// leaves a hand-written file's layout alone apart from what it adds.
type orderedObject struct {
	keys []string
	vals map[string]json.RawMessage
}

func (o *orderedObject) get(key string) (json.RawMessage, bool) {
	v, ok := o.vals[key]
	return v, ok
}

func (o *orderedObject) set(key string, val json.RawMessage) {
	if o.vals == nil {
		o.vals = map[string]json.RawMessage{}
	}
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = val
}

func (o *orderedObject) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return errors.New("expected a JSON object")
	}
	*o = orderedObject{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := tok.(string)
		if !ok {
			return errors.New("expected an object key")
		}
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return err
		}
		o.set(key, val)
	}
	_, err = dec.Token() // the closing brace
	return err
}

func (o orderedObject) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := marshalLiteral(k, "")
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(o.vals[k])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// OwnRepoDefault guesses from `git remote -v` output whether the repo is the
// user's own: any remote under ConnerTechnology, in any case.
func OwnRepoDefault(remotes string) bool {
	return strings.Contains(strings.ToLower(remotes), "connertechnology")
}

// AppendExclude adds line to a git exclude file unless it, or the same path
// anchored with a leading slash, is already there. added is false when
// nothing was written.
func AppendExclude(path, line string) (added bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	bare := strings.TrimPrefix(line, "/")
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimSpace(l)
		if l == bare || l == "/"+bare {
			return false, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	prefix := ""
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		prefix = "\n"
	}
	if _, err := f.WriteString(prefix + line + "\n"); err != nil {
		f.Close()
		return false, err
	}
	return true, f.Close()
}

// Repo is a git work tree, located by RepoRoot.
type Repo struct {
	Root string
}

// ErrNotARepo means the directory is not inside a git work tree.
var ErrNotARepo = errors.New("not inside a git repository (run this from the repo you want to connect to Linear)")

// RepoRoot finds the top of the git work tree dir is in. Outside a work tree
// it returns ErrNotARepo; any other failure (git missing, say) comes back as
// itself.
func RepoRoot(ctx context.Context, dir string) (Repo, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			stderr := strings.TrimSpace(string(ee.Stderr))
			if strings.Contains(stderr, "not a git repository") {
				return Repo{}, ErrNotARepo
			}
			if stderr != "" {
				return Repo{}, fmt.Errorf("git rev-parse --show-toplevel: %s", stderr)
			}
		}
		return Repo{}, fmt.Errorf("cannot run git: %w", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		// A bare repository, or inside .git: there is no work tree to set up.
		return Repo{}, ErrNotARepo
	}
	return Repo{Root: root}, nil
}

// Path joins a repo-relative path onto the root.
func (r Repo) Path(rel string) string {
	return filepath.Join(r.Root, filepath.FromSlash(rel))
}

// ExcludeFile is the repo's info/exclude, wherever git keeps it, which in a
// worktree is not under the work tree's own .git.
func (r Repo) ExcludeFile(ctx context.Context) (string, error) {
	out, err := gitOutput(ctx, r.Root, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return "", fmt.Errorf("cannot find the git exclude file: %w", err)
	}
	p := strings.TrimSpace(out)
	if !filepath.IsAbs(p) {
		p = filepath.Join(r.Root, p)
	}
	return p, nil
}

// IsIgnored reports whether git ignores a repo-relative path.
func (r Repo) IsIgnored(ctx context.Context, rel string) bool {
	return exec.CommandContext(ctx, "git", "-C", r.Root, "check-ignore", "-q", rel).Run() == nil
}

// IsTracked reports whether a repo-relative path is committed or staged.
func (r Repo) IsTracked(ctx context.Context, rel string) bool {
	return exec.CommandContext(ctx, "git", "-C", r.Root, "ls-files", "--error-unmatch", rel).Run() == nil
}

// Remotes is `git remote -v`, empty when there are none.
func (r Repo) Remotes(ctx context.Context) string {
	out, _ := gitOutput(ctx, r.Root, "remote", "-v")
	return out
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}
