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

	"github.com/ConnerTechnology/ctdev/ctdev/claudecli/claudeclitest"
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
	f := claudeclitest.New(t)
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
	f.SetLocalMCPServer(repo, "linear", string(linear.MCPEntry("acme")))

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

func TestAddLocalServer(t *testing.T) {
	f := claudeclitest.New(t)
	ctx := context.Background()
	root := t.TempDir()
	want := linear.MCPEntry("acme")

	// None yet: added, without a question.
	var done bool
	var err error
	captureStdout(t, func() { done, err = addLocalServer(ctx, root, "linear", nil, false, want) })
	if err != nil || !done {
		t.Fatalf("done %v, err %v", done, err)
	}
	if calls := f.Calls(); len(calls) != 1 || !strings.HasPrefix(calls[0], "mcp add-json linear {") || !strings.HasSuffix(calls[0], "--scope local") {
		t.Fatalf("calls %q", calls)
	}

	// The same entry: nothing runs.
	captureStdout(t, func() { done, err = addLocalServer(ctx, root, "linear", want, true, want) })
	if err != nil || !done || len(f.Calls()) != 1 {
		t.Fatalf("same entry: done %v, err %v, calls %q", done, err, f.Calls())
	}

	// A different one, declined: left alone.
	other := json.RawMessage(`{"type":"http","url":"https://mcp.linear.app/mcp","headersHelper":"./scripts/linear-app.sh"}`)
	feedStdin(t, "n\n")
	captureStdout(t, func() { done, err = addLocalServer(ctx, root, "linear", other, true, want) })
	if err != nil || done || len(f.Calls()) != 1 {
		t.Fatalf("declined: done %v, err %v, calls %q", done, err, f.Calls())
	}

	// Accepted: removed, then added.
	feedStdin(t, "y\n")
	captureStdout(t, func() { done, err = addLocalServer(ctx, root, "linear", other, true, want) })
	calls := f.Calls()
	if err != nil || !done || len(calls) != 3 || calls[1] != "mcp remove linear --scope local" {
		t.Fatalf("accepted: done %v, err %v, calls %q", done, err, calls)
	}
}

// A repo set up before CTD-109 has the server in .mcp.json; --show reports
// that workspace, and setting up writes nothing in the repo.
func TestConfigureLinearShowReadsLegacyMCPJSON(t *testing.T) {
	isolateLinear(t)
	fakeLinearAPI(t)
	claudeclitest.New(t)
	flagConfigShow = true
	t.Cleanup(func() { flagConfigShow = false })

	repo := tempGitRepo(t)
	t.Chdir(repo)
	_ = linear.SaveCredentials("acme", linear.Credentials{ClientID: fakeLinearID, ClientSecret: fakeLinearSecret})
	_ = os.WriteFile(filepath.Join(repo, ".mcp.json"), []byte(`{"mcpServers":{"linear":`+string(linear.MCPEntry("acme"))+`}}`), 0o644)

	var err error
	out := captureStdout(t, func() { err = configureLinear(context.Background()) })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".mcp.json names acme", "connected as Claude Code (testhost)"} {
		if !strings.Contains(out, want) {
			t.Errorf("show output should contain %q:\n%s", want, out)
		}
	}
}
