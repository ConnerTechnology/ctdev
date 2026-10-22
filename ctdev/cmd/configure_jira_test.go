package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/claudecli/claudeclitest"
	"github.com/ConnerTechnology/ctdev/ctdev/jira"
)

const (
	fakeJiraEmail = "test@example.invalid"
	fakeJiraToken = "test-jira-token"
)

// fakeJiraAPI serves /rest/api/3/myself for the fake credentials only.
func fakeJiraAPI(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, _ := r.BasicAuth(); user != fakeJiraEmail || pass != fakeJiraToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"displayName":"Example Person"}`)
	}))
	t.Cleanup(srv.Close)
	orig := newJiraClient
	newJiraClient = func() *jira.Client {
		return &jira.Client{HTTP: srv.Client(), BaseURL: func(string) string { return srv.URL }}
	}
	t.Cleanup(func() { newJiraClient = orig })
}

func TestJiraCommandsAreHidden(t *testing.T) {
	j := childCommand(t, rootCmd, "jira")
	if !j.Hidden || !childCommand(t, j, "auth-header").Hidden {
		t.Error("ctdev jira and its auth-header should be hidden")
	}
	if !hasSubcommand(childCommand(t, rootCmd, "configure"), "jira") {
		t.Error("configure should have a jira subcommand")
	}
}

func TestConfigureJiraRejectsBatch(t *testing.T) {
	flagBatch = true
	t.Cleanup(func() { flagBatch = false })
	if err := configureJira(context.Background()); err == nil || !strings.Contains(err.Error(), "interactively") {
		t.Errorf("got %v", err)
	}
}

func TestJiraAuthHeader(t *testing.T) {
	isolateLinear(t)
	var out bytes.Buffer
	err := jiraAuthHeader("example", &out)
	if err == nil || !strings.Contains(err.Error(), "ctdev configure jira") || out.Len() != 0 {
		t.Fatalf("no token: err %v, out %q", err, out.String())
	}
	creds := jira.Credentials{Email: fakeJiraEmail, Token: fakeJiraToken}
	if err := jira.SaveCredentials("example", creds); err != nil {
		t.Fatal(err)
	}
	if err := jiraAuthHeader("example", &out); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "Authorization: "+creds.AuthHeader()+"\n"; got != want {
		t.Errorf("stdout %q, want %q", got, want)
	}
}

func TestEnterJiraCredentialsValidatesBeforeSaving(t *testing.T) {
	isolateLinear(t)
	fakeJiraAPI(t)
	client := newJiraClient()

	feedStdin(t, fakeJiraEmail+"\ntest-wrong-token\nn\n")
	var saved bool
	var err error
	out := captureStdout(t, func() { saved, err = enterJiraCredentials(context.Background(), client, "example") })
	if err != nil || saved {
		t.Fatalf("saved %v, err %v", saved, err)
	}
	if _, err := os.Stat(jira.CredentialsPath("example")); !os.IsNotExist(err) {
		t.Error("the token was saved after a failed check")
	}
	if !strings.Contains(out, "rejected") || strings.Contains(out, "test-wrong-token") || !strings.Contains(out, jira.TokensURL) {
		t.Errorf("output should show the steps and the error, not the token:\n%s", out)
	}

	feedStdin(t, fakeJiraEmail+"\ntest-wrong-token\ny\n"+fakeJiraEmail+"\n"+fakeJiraToken+"\n")
	captureStdout(t, func() { saved, err = enterJiraCredentials(context.Background(), client, "example") })
	if err != nil || !saved {
		t.Fatalf("saved %v, err %v", saved, err)
	}
	if got, err := jira.LoadCredentials("example"); err != nil || got.Token != fakeJiraToken {
		t.Errorf("saved %+v, %v", got, err)
	}
}

func TestConfigureJiraSetsUpLocallyAndWritesNothingInTheRepo(t *testing.T) {
	isolateLinear(t)
	fakeJiraAPI(t)
	f := claudeclitest.New(t)
	useFakeTool(t, true)
	repo := tempGitRepo(t)
	t.Chdir(repo)

	// A new site, typed as its URL; the token; no existing server.
	feedStdin(t, "https://example.atlassian.net\n"+fakeJiraEmail+"\n"+fakeJiraToken+"\n")
	var err error
	out := captureStdout(t, func() { err = jiraWizard(context.Background()) })
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, err := jira.LoadCredentials("example"); err != nil {
		t.Error(err)
	}
	calls := f.Calls()
	if len(calls) != 1 || calls[0] != `mcp add-json atlassian {"type":"http","url":"`+jira.MCPURL+`"} --scope local` {
		t.Errorf("calls %q", calls)
	}
	if !strings.Contains(out, "ctdev jira auth-header --site example") {
		t.Errorf("output should name the scripts' command:\n%s", out)
	}
	entries, _ := os.ReadDir(repo)
	if len(entries) != 1 || entries[0].Name() != ".git" {
		t.Errorf("the repo gained files: %v", entries)
	}

	// --show reports the server and the site.
	f.SetLocalMCPServer(repo, "atlassian", string(jira.MCPEntry()))
	flagConfigShow = true
	t.Cleanup(func() { flagConfigShow = false })
	out = captureStdout(t, func() { err = configureJira(context.Background()) })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"set up (" + jira.MCPURL + ")", "https://example.atlassian.net", "works, as Example Person"} {
		if !strings.Contains(out, want) {
			t.Errorf("show should contain %q:\n%s", want, out)
		}
	}
}
