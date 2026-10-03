package linear

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCurrentWorkspace(t *testing.T) {
	cases := []struct {
		doc  string
		ws   string
		want bool
	}{
		{`{"mcpServers":{"linear":{"headersHelper":"ctdev linear mcp-headers --workspace acme"}}}`, "acme", true},
		{`{"mcpServers":{"linear":{"headersHelper":"./scripts/linear-app.sh --mcp-headers"}}}`, "", false},
		{`{"mcpServers":{"other":{"headersHelper":"ctdev linear mcp-headers --workspace acme"}}}`, "", false},
		{`{"mcpServers":{"linear":{"headersHelper":"ctdev linear mcp-headers --workspace Bad_Name"}}}`, "", false},
		{`not json`, "", false},
		{``, "", false},
	}
	for _, tc := range cases {
		ws, ok := CurrentWorkspace([]byte(tc.doc))
		if ws != tc.ws || ok != tc.want {
			t.Errorf("%s: got %q, %v", tc.doc, ws, ok)
		}
	}
}

func TestPlanMCPJSONNewFile(t *testing.T) {
	plan, err := PlanMCPJSON(nil, "acme")
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "mcpServers": {
    "linear": {
      "type": "http",
      "url": "https://mcp.linear.app/mcp",
      "headersHelper": "ctdev linear mcp-headers --workspace acme"
    }
  }
}
`
	if string(plan.Content) != want {
		t.Errorf("content:\n%s\nwant:\n%s", plan.Content, want)
	}
	if !plan.Changed || plan.Previous != nil {
		t.Errorf("changed %v, previous %s", plan.Changed, plan.Previous)
	}
}

func TestPlanMCPJSONPreservesOtherServersAndKeys(t *testing.T) {
	existing := `{
  "zfirst": true,
  "mcpServers": {
    "notion": {"type": "http", "url": "https://mcp.example.invalid/notion"},
    "linear": {"type": "http", "url": "https://mcp.linear.app/mcp", "headersHelper": "./scripts/linear-app.sh --mcp-headers"},
    "files": {"command": "npx", "args": ["-y", "example-files"]}
  },
  "after": {"keep": [1, 2]}
}`
	plan, err := PlanMCPJSON([]byte(existing), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Changed || !strings.Contains(string(plan.Previous), "linear-app.sh") {
		t.Errorf("changed %v, previous %s", plan.Changed, plan.Previous)
	}
	var doc struct {
		Zfirst     bool                       `json:"zfirst"`
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
		After      map[string][]int           `json:"after"`
	}
	if err := json.Unmarshal(plan.Content, &doc); err != nil {
		t.Fatal(err)
	}
	if !doc.Zfirst || len(doc.After["keep"]) != 2 || len(doc.MCPServers) != 3 {
		t.Errorf("lost something: %s", plan.Content)
	}
	if !strings.Contains(string(doc.MCPServers["files"]), "example-files") ||
		!strings.Contains(string(doc.MCPServers["notion"]), "mcp.example.invalid") {
		t.Errorf("other servers changed: %s", plan.Content)
	}
	if ws, ok := CurrentWorkspace(plan.Content); !ok || ws != "acme" {
		t.Errorf("linear entry not rewritten: %s", plan.Content)
	}
	// Key order survives the merge.
	s := string(plan.Content)
	if !(strings.Index(s, `"zfirst"`) < strings.Index(s, `"mcpServers"`) &&
		strings.Index(s, `"notion"`) < strings.Index(s, `"linear"`) &&
		strings.Index(s, `"linear"`) < strings.Index(s, `"files"`) &&
		strings.Index(s, `"files"`) < strings.Index(s, `"after"`)) {
		t.Errorf("key order changed:\n%s", s)
	}
	if !strings.HasSuffix(s, "}\n") || !strings.Contains(s, "\n  \"mcpServers\": {\n    \"notion\"") {
		t.Errorf("not 2-space indented with a trailing newline:\n%s", s)
	}
}

func TestPlanMCPJSONUnchanged(t *testing.T) {
	first, _ := PlanMCPJSON(nil, "acme")
	again, err := PlanMCPJSON(first.Content, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if again.Changed || string(again.Content) != string(first.Content) {
		t.Errorf("re-planning the same workspace should be a no-op: %+v", again)
	}
	other, _ := PlanMCPJSON(first.Content, "zeta")
	if !other.Changed || other.Previous == nil {
		t.Error("switching workspace should be a change with a previous entry")
	}
}

func TestPlanMCPJSONRejectsNonObject(t *testing.T) {
	if _, err := PlanMCPJSON([]byte(`[1]`), "acme"); err == nil {
		t.Error("expected an error for a non-object .mcp.json")
	}
}

func TestMergeSettingsLocal(t *testing.T) {
	out, changed, err := MergeSettingsLocal(nil)
	if err != nil || !changed {
		t.Fatalf("new file: %v %v", changed, err)
	}
	if string(out) != "{\n  \"enabledMcpjsonServers\": [\n    \"linear\"\n  ]\n}\n" {
		t.Errorf("new file content:\n%s", out)
	}

	existing := `{"permissions":{"allow":["Bash(ls)"]},"enabledMcpjsonServers":["notion"]}`
	out, changed, err = MergeSettingsLocal([]byte(existing))
	if err != nil || !changed {
		t.Fatalf("merge: %v %v", changed, err)
	}
	var doc struct {
		Permissions struct {
			Allow []string `json:"allow"`
		} `json:"permissions"`
		Enabled []string `json:"enabledMcpjsonServers"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Permissions.Allow) != 1 || strings.Join(doc.Enabled, ",") != "notion,linear" {
		t.Errorf("merged: %s", out)
	}

	_, changed, err = MergeSettingsLocal(out)
	if err != nil || changed {
		t.Errorf("already enabled should be unchanged: %v %v", changed, err)
	}
}

// initRepo makes a throwaway git repo.
func initRepo(t *testing.T) Repo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	r, err := RepoRoot(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestRepoRootOutsideRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir())
	if _, err := RepoRoot(context.Background(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "git repository") {
		t.Errorf("got %v", err)
	}
}

func TestAppendExcludeIsIdempotent(t *testing.T) {
	r := initRepo(t)
	ctx := context.Background()
	exclude, err := r.ExcludeFile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(exclude) {
		t.Errorf("exclude path %q is not absolute", exclude)
	}

	if err := os.WriteFile(r.Path(MCPJSONFile), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r.IsIgnored(ctx, MCPJSONFile) {
		t.Fatal(".mcp.json ignored before anything was excluded")
	}

	added, err := AppendExclude(exclude, MCPJSONFile)
	if err != nil || !added {
		t.Fatalf("first append: %v %v", added, err)
	}
	added, err = AppendExclude(exclude, MCPJSONFile)
	if err != nil || added {
		t.Fatalf("second append: %v %v", added, err)
	}
	data, _ := os.ReadFile(exclude)
	if n := strings.Count(string(data), MCPJSONFile); n != 1 {
		t.Errorf("exclude names .mcp.json %d times:\n%s", n, data)
	}
	if !r.IsIgnored(ctx, MCPJSONFile) {
		t.Error("git does not ignore .mcp.json after the append")
	}
}

func TestAppendExcludeAddsNewlineAndSeesAnchoredLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "info", "exclude")
	if added, err := AppendExclude(path, MCPJSONFile); err != nil || !added {
		t.Fatalf("missing file: %v %v", added, err)
	}

	path = filepath.Join(t.TempDir(), "exclude")
	_ = os.WriteFile(path, []byte("*.log"), 0o644)
	if _, err := AppendExclude(path, MCPJSONFile); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "*.log\n.mcp.json\n" {
		t.Errorf("got %q", data)
	}

	_ = os.WriteFile(path, []byte("/.mcp.json\n"), 0o644)
	if added, _ := AppendExclude(path, MCPJSONFile); added {
		t.Error("an anchored /.mcp.json line should count as present")
	}
}

func TestExcludeFileInWorktree(t *testing.T) {
	r := initRepo(t)
	runGit(t, r.Root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-q", "--allow-empty", "-m", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	runGit(t, r.Root, "worktree", "add", "-q", wt)
	w, err := RepoRoot(context.Background(), wt)
	if err != nil {
		t.Fatal(err)
	}
	exclude, err := w.ExcludeFile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AppendExclude(exclude, MCPJSONFile); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(w.Path(MCPJSONFile), []byte("{}\n"), 0o644)
	if !w.IsIgnored(context.Background(), MCPJSONFile) {
		t.Errorf("worktree does not ignore .mcp.json via %s", exclude)
	}
}

func TestOwnRepoDefault(t *testing.T) {
	cases := map[string]bool{
		"origin\tgit@github.com:ConnerTechnology/ctdev.git (fetch)\n":      true,
		"origin\thttps://github.com/connertechnology/example.git (push)\n": true,
		"upstream\thttps://github.com/example-vendor/tool.git (fetch)\n":   false,
		"": false,
		"a\thttps://x.invalid/a (fetch)\nb\tgit@github.com:CONNERTECHNOLOGY/b": true,
	}
	for remotes, want := range cases {
		if got := OwnRepoDefault(remotes); got != want {
			t.Errorf("%q: got %v, want %v", remotes, got, want)
		}
	}
}

func TestRemotes(t *testing.T) {
	r := initRepo(t)
	runGit(t, r.Root, "remote", "add", "origin", "https://github.com/ConnerTechnology/example.git")
	if !OwnRepoDefault(r.Remotes(context.Background())) {
		t.Error("a ConnerTechnology origin should default to own repo")
	}
}
