package linear

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testClientID = "test-client-id"
	testSecret   = "test-secret-do-not-leak"
)

var testNow = time.Unix(1_900_000_000, 0)

// fakeLinear stands in for api.linear.app: one handler for the token
// endpoint, one for GraphQL. Tests set the fields to shape its answers.
type fakeLinear struct {
	t *testing.T

	mu          sync.Mutex
	tokenCalls  int
	viewerCalls int
	issued      string // token handed out by the next token request

	// tokenStatus and tokenBody, when set, replace the success answer.
	tokenStatus int
	tokenBody   string
	// rejected tokens get this answer from GraphQL ("401" or "autherr").
	rejected map[string]string
	// graphQLStatus/graphQLBody, when set, answer every GraphQL request.
	graphQLStatus int
	graphQLBody   string
	lastGraphQL   map[string]any
}

func newFake(t *testing.T) (*fakeLinear, *Client) {
	f := &fakeLinear{t: t, issued: "test-token-fresh", rejected: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", f.token)
	mux.HandleFunc("/graphql", f.graphql)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := &Client{
		TokenURL:   srv.URL + "/oauth/token",
		GraphQLURL: srv.URL + "/graphql",
		HTTP:       srv.Client(),
		Now:        func() time.Time { return testNow },
	}
	return f, c
}

func (f *fakeLinear) token(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokenCalls++
	user, pass, ok := r.BasicAuth()
	if !ok || user != testClientID || pass != testSecret {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"invalid_client","error_description":"Client authentication failed"}`)
		return
	}
	_ = r.ParseForm()
	if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("scope") != Scope {
		f.t.Errorf("token request form = %v", r.Form)
	}
	if f.tokenStatus != 0 {
		w.WriteHeader(f.tokenStatus)
		fmt.Fprint(w, f.tokenBody)
		return
	}
	fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":2591999,"scope":"write read initiative:write"}`, f.issued)
}

func (f *fakeLinear) graphql(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	body, _ := io.ReadAll(r.Body)
	var req map[string]any
	_ = json.Unmarshal(body, &req)
	f.lastGraphQL = req
	if q, _ := req["query"].(string); q == "{ viewer { id } }" {
		f.viewerCalls++
	}
	switch f.rejected[token] {
	case "401":
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"errors":[{"message":"unauthorized"}]}`)
		return
	case "autherr":
		fmt.Fprint(w, `{"errors":[{"message":"Authentication required","extensions":{"code":"AUTHENTICATION_ERROR"}}]}`)
		return
	}
	if f.graphQLStatus != 0 {
		w.WriteHeader(f.graphQLStatus)
		fmt.Fprint(w, f.graphQLBody)
		return
	}
	fmt.Fprint(w, `{"data":{"viewer":{"id":"x","name":"Claude Code (testhost)"},"teams":{"nodes":[{"key":"ENG","name":"Engineering"},{"key":"OPS","name":"Operations"}]}}}`)
}

func saveTestCreds(t *testing.T, ws string) {
	t.Helper()
	if err := SaveCredentials(ws, Credentials{testClientID, testSecret}); err != nil {
		t.Fatal(err)
	}
}

func seedCache(t *testing.T, ws string, tok CachedToken) {
	t.Helper()
	if err := writeCache(ws, tok); err != nil {
		t.Fatal(err)
	}
}

func goodCache(token string) CachedToken {
	return CachedToken{AccessToken: token, ExpiresAt: testNow.Add(10 * 24 * time.Hour).Unix(), Scope: "initiative:write read write"}
}

func TestFresh(t *testing.T) {
	c := &Client{Now: func() time.Time { return testNow }}
	cases := []struct {
		name string
		tok  CachedToken
		want bool
	}{
		{"same scopes, other order, space-separated", goodCache("t"), true},
		{"comma-separated scope", CachedToken{"t", testNow.Add(48 * time.Hour).Unix(), "read,write,initiative:write"}, true},
		{"scope missing one", CachedToken{"t", testNow.Add(48 * time.Hour).Unix(), "read write"}, false},
		{"scope has an extra", CachedToken{"t", testNow.Add(48 * time.Hour).Unix(), "read write initiative:write admin"}, false},
		{"no scope", CachedToken{"t", testNow.Add(48 * time.Hour).Unix(), ""}, false},
		{"exactly one day left", CachedToken{"t", testNow.Add(24 * time.Hour).Unix(), Scope}, false},
		{"just over a day left", CachedToken{"t", testNow.Add(24*time.Hour + time.Second).Unix(), Scope}, true},
		{"expired", CachedToken{"t", testNow.Add(-time.Hour).Unix(), Scope}, false},
		{"no expiry", CachedToken{"t", 0, Scope}, false},
		{"no token", CachedToken{"", testNow.Add(48 * time.Hour).Unix(), Scope}, false},
	}
	for _, tc := range cases {
		if got := c.fresh(tc.tok); got != tc.want {
			t.Errorf("%s: fresh = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestTokenFetchesAndCaches(t *testing.T) {
	isolate(t)
	f, c := newFake(t)
	saveTestCreds(t, "acme")

	tok, err := c.Token(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "test-token-fresh" {
		t.Errorf("token = %q", tok)
	}
	assertMode(t, CacheDir(), 0o700)
	assertMode(t, CachePath("acme"), 0o600)
	cached, ok := readCache("acme")
	if !ok || cached.AccessToken != tok || cached.ExpiresAt != testNow.Unix()+2591999 || cached.Scope != "write read initiative:write" {
		t.Errorf("cache = %+v", cached)
	}

	// The second call uses the cache after one viewer check.
	if _, err := c.Token(context.Background(), "acme"); err != nil {
		t.Fatal(err)
	}
	if f.tokenCalls != 1 || f.viewerCalls != 1 {
		t.Errorf("token calls %d, viewer calls %d; want 1 and 1", f.tokenCalls, f.viewerCalls)
	}
}

func TestCacheDirFallsBackToHome(t *testing.T) {
	home := isolate(t)
	t.Setenv("XDG_CACHE_HOME", "")
	if got, want := CachePath("acme"), home+"/.cache/ctdev/linear/acme.json"; got != want {
		t.Errorf("CachePath = %q, want %q", got, want)
	}
}

func TestTokenRefetchesWhenStale(t *testing.T) {
	cases := map[string]CachedToken{
		"scope mismatch": {"test-token-old", testNow.Add(10 * 24 * time.Hour).Unix(), "read write"},
		"near expiry":    {"test-token-old", testNow.Add(time.Hour).Unix(), Scope},
	}
	for name, stale := range cases {
		t.Run(name, func(t *testing.T) {
			isolate(t)
			f, c := newFake(t)
			saveTestCreds(t, "acme")
			seedCache(t, "acme", stale)
			tok, err := c.Token(context.Background(), "acme")
			if err != nil {
				t.Fatal(err)
			}
			if tok != "test-token-fresh" || f.tokenCalls != 1 {
				t.Errorf("token %q after %d token calls; want a refetch", tok, f.tokenCalls)
			}
			if f.viewerCalls != 0 {
				t.Errorf("a stale cache should not be checked with Linear (viewer calls %d)", f.viewerCalls)
			}
		})
	}
}

func TestTokenRefetchesWhenCachedTokenRejected(t *testing.T) {
	for _, how := range []string{"401", "autherr"} {
		t.Run(how, func(t *testing.T) {
			isolate(t)
			f, c := newFake(t)
			saveTestCreds(t, "acme")
			seedCache(t, "acme", goodCache("test-token-revoked"))
			f.rejected["test-token-revoked"] = how

			tok, err := c.Token(context.Background(), "acme")
			if err != nil {
				t.Fatal(err)
			}
			if tok != "test-token-fresh" || f.tokenCalls != 1 || f.viewerCalls != 1 {
				t.Errorf("token %q, token calls %d, viewer calls %d", tok, f.tokenCalls, f.viewerCalls)
			}
			if cached, _ := readCache("acme"); cached.AccessToken != "test-token-fresh" {
				t.Errorf("cache still holds %q", cached.AccessToken)
			}
		})
	}
}

func TestTokenKeepsCacheOnNetworkFailure(t *testing.T) {
	isolate(t)
	_, c := newFake(t)
	saveTestCreds(t, "acme")
	seedCache(t, "acme", goodCache("test-token-cached"))
	// Nothing listens here: the viewer check fails to connect.
	c.GraphQLURL = "http://127.0.0.1:1/graphql"
	c.TokenURL = "http://127.0.0.1:1/oauth/token"

	tok, err := c.Token(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "test-token-cached" {
		t.Errorf("token = %q, want the cached one", tok)
	}
}

func TestFetchTokenErrorCarriesLinearTextNotSecret(t *testing.T) {
	isolate(t)
	f, c := newFake(t)
	f.tokenStatus = http.StatusBadRequest
	f.tokenBody = `{"error":"invalid_scope","error_description":"Requested scope is invalid"}`

	_, err := c.FetchToken(context.Background(), Credentials{testClientID, testSecret})
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	for _, want := range []string{"HTTP 400", "invalid_scope", "Requested scope is invalid"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q should contain %q", msg, want)
		}
	}
	if strings.Contains(msg, testSecret) {
		t.Error("error contains the secret")
	}
}

func TestFetchTokenBadCredentials(t *testing.T) {
	isolate(t)
	_, c := newFake(t)
	_, err := c.FetchToken(context.Background(), Credentials{testClientID, "test-wrong-secret"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") || !strings.Contains(err.Error(), "invalid_client") {
		t.Errorf("got %v", err)
	}
	if strings.Contains(err.Error(), "test-wrong-secret") {
		t.Error("error contains the secret")
	}
}

func TestFetchTokenMalformedSuccess(t *testing.T) {
	isolate(t)
	f, c := newFake(t)
	f.tokenStatus = http.StatusOK
	f.tokenBody = `{"token_type":"Bearer"}`
	if _, err := c.FetchToken(context.Background(), Credentials{testClientID, testSecret}); err == nil ||
		!strings.Contains(err.Error(), "HTTP 200") {
		t.Errorf("a 200 with no access_token should fail, got %v", err)
	}
}

func TestFetchTokenNetworkFailureHidesSecret(t *testing.T) {
	c := &Client{TokenURL: "http://127.0.0.1:1/oauth/token", HTTP: http.DefaultClient, Now: time.Now}
	_, err := c.FetchToken(context.Background(), Credentials{testClientID, testSecret})
	if err == nil || !strings.Contains(err.Error(), "could not reach") {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), testSecret) {
		t.Error("error contains the secret")
	}
}

func TestTokenWithoutCredentials(t *testing.T) {
	isolate(t)
	_, c := newFake(t)
	if _, err := c.Token(context.Background(), "acme"); err == nil {
		t.Error("expected a missing-credentials error")
	}
}

func TestGraphQLSendsQueryAndVariables(t *testing.T) {
	isolate(t)
	f, c := newFake(t)
	saveTestCreds(t, "acme")
	body, status, err := c.GraphQL(context.Background(), "acme", "query($id: String!) { issue(id: $id) { id } }", json.RawMessage(`{"id":"CTD-1"}`))
	if err != nil || status != 200 || len(body) == 0 {
		t.Fatalf("status %d, err %v", status, err)
	}
	vars, _ := f.lastGraphQL["variables"].(map[string]any)
	if vars["id"] != "CTD-1" {
		t.Errorf("variables sent = %v", f.lastGraphQL["variables"])
	}

	if _, _, err := c.GraphQL(context.Background(), "acme", "{ viewer { id } }", json.RawMessage(`{nope`)); err == nil ||
		!strings.Contains(err.Error(), "VARIABLES_JSON") {
		t.Errorf("bad variables: got %v", err)
	}
}

func TestClearCache(t *testing.T) {
	isolate(t)
	seedCache(t, "acme", goodCache("test-token"))
	if err := ClearCache("acme"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(CachePath("acme")); !os.IsNotExist(err) {
		t.Error("cache file still there")
	}
	if err := ClearCache("acme"); err != nil {
		t.Errorf("clearing a missing cache should succeed: %v", err)
	}
}

func TestCheckStatus(t *testing.T) {
	isolate(t)
	_, c := newFake(t)

	st := c.CheckStatus(context.Background(), "acme")
	if st.HasCredentials || st.Connected || st.Err == nil {
		t.Errorf("no credentials: %+v", st)
	}

	saveTestCreds(t, "acme")
	st = c.CheckStatus(context.Background(), "acme")
	if !st.HasCredentials || !st.Connected || st.Err != nil {
		t.Fatalf("connected: %+v", st)
	}
	if st.Actor != "Claude Code (testhost)" || strings.Join(st.TeamKeys(), ",") != "ENG,OPS" {
		t.Errorf("actor %q, teams %v", st.Actor, st.TeamKeys())
	}
}

func TestCheckStatusReportsGraphQLErrors(t *testing.T) {
	isolate(t)
	f, c := newFake(t)
	saveTestCreds(t, "acme")
	f.graphQLStatus = http.StatusBadRequest
	f.graphQLBody = `{"errors":[{"message":"Example failure"}]}`
	st := c.CheckStatus(context.Background(), "acme")
	if st.Connected || st.Err == nil || !strings.Contains(st.Err.Error(), "Example failure") {
		t.Errorf("got %+v", st)
	}
}
