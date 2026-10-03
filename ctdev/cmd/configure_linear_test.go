package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/linear"
)

const (
	fakeLinearID     = "test-client-id"
	fakeLinearSecret = "test-secret"
	fakeLinearToken  = "test-token-abc"
)

// fakeLinearAPI serves a token endpoint that accepts only the fake
// credentials, and a GraphQL endpoint that answers the status query.
func fakeLinearAPI(t *testing.T) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		if id, secret, _ := r.BasicAuth(); id != fakeLinearID || secret != fakeLinearSecret {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"invalid_client","error_description":"Client authentication failed"}`)
			return
		}
		fmt.Fprintf(w, `{"access_token":%q,"expires_in":2592000,"scope":"read write initiative:write"}`, fakeLinearToken)
	})
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeLinearToken {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"errors":[{"message":"Authentication required","extensions":{"code":"AUTHENTICATION_ERROR"}}]}`)
			return
		}
		var req struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if strings.Contains(req.Query, "broken") {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"errors":[{"message":"Example syntax error"}]}`)
			return
		}
		fmt.Fprint(w, `{"data":{"viewer":{"id":"x","name":"Claude Code (testhost)"},"teams":{"nodes":[{"key":"ENG","name":"Engineering"}]}}}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	orig := newLinearClient
	newLinearClient = func() *linear.Client {
		return &linear.Client{TokenURL: srv.URL + "/oauth/token", GraphQLURL: srv.URL + "/graphql", HTTP: srv.Client(), Now: time.Now}
	}
	t.Cleanup(func() { newLinearClient = orig })
}

func isolateLinear(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
}

func feedStdin(t *testing.T, input string) {
	t.Helper()
	orig := stdinScanner
	stdinScanner = bufio.NewScanner(strings.NewReader(input))
	t.Cleanup(func() { stdinScanner = orig })
}

func TestLinearCommandsAreHidden(t *testing.T) {
	lin := childCommand(t, rootCmd, "linear")
	if !lin.Hidden {
		t.Error("ctdev linear should be hidden")
	}
	for _, name := range []string{"mcp-headers", "graphql"} {
		if c := childCommand(t, lin, name); !c.Hidden {
			t.Errorf("ctdev linear %s should be hidden", name)
		}
	}
	if !hasSubcommand(childCommand(t, rootCmd, "configure"), "linear") {
		t.Error("configure should have a linear subcommand")
	}
}

func TestLinearMCPHeadersPrintsOnlyTheHeader(t *testing.T) {
	isolateLinear(t)
	fakeLinearAPI(t)
	if err := linear.SaveCredentials("acme", linear.Credentials{ClientID: fakeLinearID, ClientSecret: fakeLinearSecret}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := linearMCPHeaders(nil, "acme", &out); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), `{"Authorization":"Bearer `+fakeLinearToken+`"}`+"\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestLinearMCPHeadersFailureNamesWorkspaceAndFix(t *testing.T) {
	isolateLinear(t)
	fakeLinearAPI(t)
	if err := linear.SaveCredentials("acme", linear.Credentials{ClientID: fakeLinearID, ClientSecret: "test-wrong-secret"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := linearMCPHeaders(nil, "acme", &out)
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	for _, want := range []string{`"acme"`, "invalid_client", "run 'ctdev configure linear' in this repo to check or replace the credentials"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q should contain %q", msg, want)
		}
	}
	if strings.Contains(msg, "test-wrong-secret") {
		t.Error("error contains the secret")
	}
	if out.Len() != 0 {
		t.Errorf("stdout should be empty on failure, got %q", out.String())
	}

	if err := linearMCPHeaders(nil, "Not/Valid", &out); err == nil || !strings.Contains(err.Error(), "ctdev configure linear") {
		t.Errorf("invalid workspace: got %v", err)
	}
}

func TestLinearGraphQL(t *testing.T) {
	isolateLinear(t)
	fakeLinearAPI(t)
	_ = linear.SaveCredentials("acme", linear.Credentials{ClientID: fakeLinearID, ClientSecret: fakeLinearSecret})

	var out bytes.Buffer
	if err := linearGraphQL(nil, "acme", "{ viewer { name } }", nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Claude Code (testhost)") {
		t.Errorf("body not printed: %q", out.String())
	}

	out.Reset()
	err := linearGraphQL(nil, "acme", "{ broken }", nil, &out)
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("non-200 should fail, got %v", err)
	}
	if !strings.Contains(out.String(), "Example syntax error") {
		t.Errorf("the error body should still print: %q", out.String())
	}

	if err := linearGraphQL(nil, "acme", "{ viewer { id } }", json.RawMessage("{nope"), &out); err == nil ||
		!strings.Contains(err.Error(), "VARIABLES_JSON") {
		t.Errorf("bad variables: got %v", err)
	}
}

func TestConfigureLinearRejectsBatch(t *testing.T) {
	flagBatch = true
	t.Cleanup(func() { flagBatch = false })
	err := configureLinear(context.Background())
	if err == nil || !strings.Contains(err.Error(), "interactively") {
		t.Errorf("expected an interactive-only error, got %v", err)
	}
}

func tempGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	resolved, _ := filepath.EvalSymlinks(dir)
	return resolved
}

func TestConfigureLinearShow(t *testing.T) {
	isolateLinear(t)
	fakeLinearAPI(t)
	flagConfigShow = true
	t.Cleanup(func() { flagConfigShow = false })

	t.Chdir(t.TempDir())
	t.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir())
	if err := configureLinear(context.Background()); err == nil || !strings.Contains(err.Error(), "git repository") {
		t.Errorf("outside a repo: got %v", err)
	}

	repo := tempGitRepo(t)
	t.Chdir(repo)
	_ = linear.SaveCredentials("acme", linear.Credentials{ClientID: fakeLinearID, ClientSecret: fakeLinearSecret})
	plan, _ := linear.PlanMCPJSON(nil, "acme")
	_ = os.WriteFile(filepath.Join(repo, ".mcp.json"), plan.Content, 0o644)

	var err error
	out := captureStdout(t, func() { err = configureLinear(context.Background()) })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"acme", "saved", "connected as Claude Code (testhost), teams ENG"} {
		if !strings.Contains(out, want) {
			t.Errorf("show output should contain %q:\n%s", want, out)
		}
	}
}

func TestEnterLinearCredentialsValidatesBeforeSaving(t *testing.T) {
	isolateLinear(t)
	fakeLinearAPI(t)
	client := newLinearClient()

	// A wrong secret, then decline to retry: nothing is saved.
	feedStdin(t, fakeLinearID+"\ntest-wrong-secret\nn\n")
	var saved bool
	var err error
	out := captureStdout(t, func() { saved, err = enterLinearCredentials(context.Background(), client, "acme") })
	if err != nil || saved {
		t.Fatalf("saved %v, err %v", saved, err)
	}
	if _, err := os.Stat(linear.CredentialsPath("acme")); !os.IsNotExist(err) {
		t.Error("credentials were saved after a failed check")
	}
	if !strings.Contains(out, "invalid_client") || strings.Contains(out, "test-wrong-secret") {
		t.Errorf("output should show Linear's error but not the secret:\n%s", out)
	}
	if !strings.Contains(out, "https://linear.app/settings/api/applications/new") || !strings.Contains(out, "Claude Code (") {
		t.Errorf("the app-creation steps are missing:\n%s", out)
	}

	// A wrong secret, retry, the right one: saved, and the stale cache is gone.
	if err := os.MkdirAll(linear.CacheDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(linear.CachePath("acme"), []byte(`{"access_token":"test-old"}`), 0o600)
	feedStdin(t, fakeLinearID+"\ntest-wrong-secret\ny\n"+fakeLinearID+"\n"+fakeLinearSecret+"\n")
	captureStdout(t, func() { saved, err = enterLinearCredentials(context.Background(), client, "acme") })
	if err != nil || !saved {
		t.Fatalf("saved %v, err %v", saved, err)
	}
	if got, err := linear.LoadCredentials("acme"); err != nil || got.ClientSecret != fakeLinearSecret {
		t.Errorf("saved credentials: %+v, %v", got, err)
	}
	if _, err := os.Stat(linear.CachePath("acme")); !os.IsNotExist(err) {
		t.Error("the old token cache should be deleted")
	}
}

func TestWriteLinearRepoFilesSomeoneElsesRepo(t *testing.T) {
	repo := tempGitRepo(t)
	r := linear.Repo{Root: repo}
	ctx := context.Background()
	existing := []byte(`{"mcpServers":{"notion":{"type":"http","url":"https://mcp.example.invalid"}}}`)
	_ = os.WriteFile(r.Path(".mcp.json"), existing, 0o644)

	var done bool
	var err error
	captureStdout(t, func() { done, err = writeLinearRepoFiles(ctx, r, existing, "acme", false) })
	if err != nil || !done {
		t.Fatalf("done %v, err %v", done, err)
	}
	mcp, _ := os.ReadFile(r.Path(".mcp.json"))
	if ws, ok := linear.CurrentWorkspace(mcp); !ok || ws != "acme" || !strings.Contains(string(mcp), "notion") {
		t.Errorf(".mcp.json:\n%s", mcp)
	}
	settings, _ := os.ReadFile(r.Path(".claude/settings.local.json"))
	if !strings.Contains(string(settings), `"linear"`) {
		t.Errorf("settings.local.json:\n%s", settings)
	}
	for _, rel := range []string{".mcp.json", ".claude/settings.local.json"} {
		if !r.IsIgnored(ctx, rel) {
			t.Errorf("%s should be git-ignored", rel)
		}
	}

	// Running it again changes nothing and duplicates no exclude line.
	captureStdout(t, func() { _, err = writeLinearRepoFiles(ctx, r, mcp, "acme", false) })
	if err != nil {
		t.Fatal(err)
	}
	exclude, _ := r.ExcludeFile(ctx)
	data, _ := os.ReadFile(exclude)
	if strings.Count(string(data), ".mcp.json") != 1 || strings.Count(string(data), "settings.local.json") != 1 {
		t.Errorf("exclude file:\n%s", data)
	}
}

func TestWriteLinearRepoFilesOwnRepo(t *testing.T) {
	repo := tempGitRepo(t)
	r := linear.Repo{Root: repo}
	ctx := context.Background()
	// A repo that already ignores settings.local.json, as this one does.
	_ = os.WriteFile(r.Path(".gitignore"), []byte(".claude/settings.local.json\n"), 0o644)

	var err error
	out := captureStdout(t, func() { _, err = writeLinearRepoFiles(ctx, r, nil, "acme", true) })
	if err != nil {
		t.Fatal(err)
	}
	if r.IsIgnored(ctx, ".mcp.json") {
		t.Error(".mcp.json should stay committable in your own repo")
	}
	if !strings.Contains(out, "Commit .mcp.json") {
		t.Errorf("should say to commit .mcp.json:\n%s", out)
	}
	exclude, _ := r.ExcludeFile(ctx)
	if data, _ := os.ReadFile(exclude); strings.Contains(string(data), "settings.local.json") {
		t.Error("an already-ignored settings.local.json should not be added to the exclude file")
	}
}

// A settings.local.json someone else made, and git would track, gets excluded
// too: the check runs on every write, not only when ctdev creates the file.
func TestWriteLinearRepoFilesExcludesExistingUnignoredSettings(t *testing.T) {
	repo := tempGitRepo(t)
	r := linear.Repo{Root: repo}
	ctx := context.Background()
	if err := os.MkdirAll(r.Path(".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(r.Path(".claude/settings.local.json"), []byte(`{"permissions":{"allow":["Bash(ls)"]}}`), 0o600)
	if r.IsIgnored(ctx, ".claude/settings.local.json") {
		t.Fatal("precondition: the file should not be ignored yet")
	}

	var err error
	captureStdout(t, func() { _, err = writeLinearRepoFiles(ctx, r, nil, "acme", true) })
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsIgnored(ctx, ".claude/settings.local.json") {
		t.Error("a pre-existing settings.local.json should end up git-ignored")
	}
	settings, _ := os.ReadFile(r.Path(".claude/settings.local.json"))
	if !strings.Contains(string(settings), "Bash(ls)") || !strings.Contains(string(settings), `"linear"`) {
		t.Errorf("settings.local.json:\n%s", settings)
	}
	fi, _ := os.Stat(r.Path(".claude/settings.local.json"))
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %o; the existing mode should be kept", fi.Mode().Perm())
	}
}

func writeBrokenMCPJSON(t *testing.T) string {
	t.Helper()
	repo := tempGitRepo(t)
	t.Chdir(repo)
	_ = os.WriteFile(filepath.Join(repo, ".mcp.json"), []byte(`{"mcpServers": {`), 0o644)
	return repo
}

func TestConfigureLinearShowRejectsInvalidMCPJSON(t *testing.T) {
	isolateLinear(t)
	fakeLinearAPI(t)
	flagConfigShow = true
	t.Cleanup(func() { flagConfigShow = false })
	writeBrokenMCPJSON(t)

	var err error
	out := captureStdout(t, func() { err = configureLinear(context.Background()) })
	if err == nil || !strings.Contains(err.Error(), ".mcp.json") {
		t.Fatalf("got %v, want an error naming .mcp.json", err)
	}
	if strings.Contains(out, "not set up") {
		t.Errorf("should stop before reporting status:\n%s", out)
	}
}

// The wizard must stop before its first prompt, so nothing (credentials
// included) is saved for a repo it then can't write to.
func TestLinearWizardRejectsInvalidMCPJSONBeforePrompting(t *testing.T) {
	isolateLinear(t)
	fakeLinearAPI(t)
	writeBrokenMCPJSON(t)
	// Answers that would create a workspace and save credentials, if asked.
	feedStdin(t, "acme\n"+fakeLinearID+"\n"+fakeLinearSecret+"\n\n")

	var err error
	out := captureStdout(t, func() { err = linearWizard(context.Background()) })
	if err == nil || !strings.Contains(err.Error(), ".mcp.json") {
		t.Fatalf("got %v, want an error naming .mcp.json", err)
	}
	if strings.Contains(out, "Workspace name") || strings.Contains(out, "Client ID") {
		t.Errorf("prompted before validating:\n%s", out)
	}
	if ws, _ := linear.Workspaces(); len(ws) != 0 {
		t.Errorf("credentials were saved: %v", ws)
	}
}

func TestWriteLinearRepoFilesConfirmsReplacingOtherEntry(t *testing.T) {
	repo := tempGitRepo(t)
	r := linear.Repo{Root: repo}
	existing := []byte(`{"mcpServers":{"linear":{"type":"http","url":"https://mcp.linear.app/mcp","headersHelper":"./scripts/linear-app.sh --mcp-headers"}}}`)
	_ = os.WriteFile(r.Path(".mcp.json"), existing, 0o644)

	feedStdin(t, "n\n")
	var done bool
	var err error
	out := captureStdout(t, func() { done, err = writeLinearRepoFiles(context.Background(), r, existing, "acme", true) })
	if err != nil || done {
		t.Fatalf("declining should stop: done %v, err %v", done, err)
	}
	if !strings.Contains(out, "linear-app.sh") || !strings.Contains(out, "--workspace acme") {
		t.Errorf("should show the old and new entries:\n%s", out)
	}
	if got, _ := os.ReadFile(r.Path(".mcp.json")); !bytes.Equal(got, existing) {
		t.Error(".mcp.json changed after declining")
	}
}
