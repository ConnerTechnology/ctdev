package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Scope is the one scope set every caller asks for. Asking Linear for a
// different set revokes every token the app already has, so it must never
// vary.
const Scope = "read,write,initiative:write"

// refreshMargin: tokens last 30 days. Refresh a day early so a long session
// never holds an expired one.
const refreshMargin = 24 * time.Hour

const (
	defaultTokenURL   = "https://api.linear.app/oauth/token"
	defaultGraphQLURL = "https://api.linear.app/graphql"
)

// Per-request deadlines, the same ones the bash helper used.
const (
	tokenTimeout   = 6 * time.Second
	viewerTimeout  = 3 * time.Second
	graphQLTimeout = 8 * time.Second
)

// Client talks to Linear. The zero value is not usable; call NewClient. Tests
// point TokenURL and GraphQLURL at httptest servers.
type Client struct {
	TokenURL   string
	GraphQLURL string
	HTTP       *http.Client
	Now        func() time.Time
}

// NewClient returns a Client for the real Linear API.
func NewClient() *Client {
	return &Client{
		TokenURL:   defaultTokenURL,
		GraphQLURL: defaultGraphQLURL,
		HTTP:       http.DefaultClient,
		Now:        time.Now,
	}
}

// CachedToken is what the token cache holds for one workspace.
type CachedToken struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   int64  `json:"expires_at"`
	Scope       string `json:"scope"`
}

// CacheDir is $XDG_CACHE_HOME/ctdev/linear, or ~/.cache/ctdev/linear.
func CacheDir() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, "ctdev", "linear")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".cache", "ctdev", "linear")
}

// CachePath is one workspace's token cache file.
func CachePath(workspace string) string {
	return filepath.Join(CacheDir(), workspace+".json")
}

// ClearCache deletes a workspace's cached token, so the next request fetches
// one with the current credentials.
func ClearCache(workspace string) error {
	if err := ValidateWorkspace(workspace); err != nil {
		return err
	}
	err := os.Remove(CachePath(workspace))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func readCache(workspace string) (CachedToken, bool) {
	data, err := os.ReadFile(CachePath(workspace))
	if err != nil {
		return CachedToken{}, false
	}
	var t CachedToken
	if json.Unmarshal(data, &t) != nil {
		return CachedToken{}, false
	}
	return t, true
}

func writeCache(workspace string, t CachedToken) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return writePrivate(CachePath(workspace), data)
}

// sortedScopes normalizes a scope list, comma- or space-separated, to a sorted
// space-joined string. Linear reports scopes space-separated and in its own
// order, while requests send them comma-separated.
func sortedScopes(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

// fresh reports whether a cached token was granted exactly Scope and has more
// than refreshMargin left. A token from before a scope change still passes
// Linear's viewer check but lacks the new scope, so the scope is compared here.
func (c *Client) fresh(t CachedToken) bool {
	if t.AccessToken == "" || sortedScopes(t.Scope) != sortedScopes(Scope) {
		return false
	}
	return time.Unix(t.ExpiresAt, 0).Sub(c.Now()) > refreshMargin
}

// accepted is true unless Linear says the token is no longer accepted. This
// costs one small request per connect, and it is what makes Claude Code's
// re-run of the helper after a 401 useful: a token revoked early (a scope
// change or a rotated secret revokes them all) is replaced, not handed back. A
// network failure keeps the token, because fetching a new one would fail the
// same way.
func (c *Client) accepted(ctx context.Context, token string) bool {
	ctx, cancel := context.WithTimeout(ctx, viewerTimeout)
	defer cancel()
	body, status, err := c.post(ctx, token, []byte(`{"query":"{ viewer { id } }"}`))
	if err != nil {
		return true
	}
	return status != http.StatusUnauthorized && !bytes.Contains(body, []byte("AUTHENTICATION_ERROR"))
}

// FetchToken asks Linear for a new client-credentials token. It does not touch
// the cache. Errors carry Linear's error and error_description when it sends
// them, and never the secret.
func (c *Client) FetchToken(ctx context.Context, creds Credentials) (CachedToken, error) {
	ctx, cancel := context.WithTimeout(ctx, tokenTimeout)
	defer cancel()

	form := url.Values{"grant_type": {"client_credentials"}, "scope": {Scope}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return CachedToken{}, fmt.Errorf("could not build the token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(creds.ClientID, creds.ClientSecret)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return CachedToken{}, fmt.Errorf("could not reach %s: %s", c.TokenURL, transportReason(err))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var parsed struct {
		AccessToken      *string          `json:"access_token"`
		ExpiresIn        *float64         `json:"expires_in"`
		Scope            json.RawMessage  `json:"scope"`
		Error            *json.RawMessage `json:"error"`
		ErrorDescription *json.RawMessage `json:"error_description"`
	}
	jsonErr := json.Unmarshal(body, &parsed)
	if resp.StatusCode != http.StatusOK || jsonErr != nil || parsed.AccessToken == nil || parsed.ExpiresIn == nil {
		msg := fmt.Sprintf("token request failed with HTTP %d", resp.StatusCode)
		if detail := linearError(parsed.Error, parsed.ErrorDescription); detail != "" {
			msg += " (" + detail + ")"
		}
		return CachedToken{}, errors.New(msg)
	}
	return CachedToken{
		AccessToken: *parsed.AccessToken,
		ExpiresAt:   c.Now().Unix() + int64(*parsed.ExpiresIn),
		Scope:       scopeString(parsed.Scope),
	}, nil
}

// linearError joins Linear's error and error_description, the way the bash
// helper's `[.error, .error_description] | join(": ")` did.
func linearError(fields ...*json.RawMessage) string {
	var parts []string
	for _, f := range fields {
		if f == nil || string(*f) == "null" {
			continue
		}
		var s string
		if json.Unmarshal(*f, &s) == nil {
			parts = append(parts, s)
		} else {
			parts = append(parts, string(*f))
		}
	}
	return strings.Join(parts, ": ")
}

// scopeString accepts the scope Linear grants as either a string or a list.
func scopeString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return strings.Join(list, " ")
	}
	return ""
}

// transportReason is the cause of a failed request without the request line,
// which is all a url.Error adds.
func transportReason(err error) string {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return uerr.Err.Error()
	}
	return err.Error()
}

// Token returns a usable token for a workspace: the cached one while it is
// fresh and Linear still accepts it, otherwise a new one, which it caches.
func (c *Client) Token(ctx context.Context, workspace string) (string, error) {
	if err := ValidateWorkspace(workspace); err != nil {
		return "", err
	}
	if t, ok := readCache(workspace); ok && c.fresh(t) && c.accepted(ctx, t.AccessToken) {
		return t.AccessToken, nil
	}
	creds, err := LoadCredentials(workspace)
	if err != nil {
		return "", err
	}
	t, err := c.FetchToken(ctx, creds)
	if err != nil {
		return "", err
	}
	if err := writeCache(workspace, t); err != nil {
		return "", fmt.Errorf("could not cache the token: %w", err)
	}
	return t.AccessToken, nil
}

// GraphQL sends one request as the app and returns the response body and the
// HTTP status. variables may be nil.
func (c *Client) GraphQL(ctx context.Context, workspace, query string, variables json.RawMessage) ([]byte, int, error) {
	token, err := c.Token(ctx, workspace)
	if err != nil {
		return nil, 0, err
	}
	return c.graphQLWithToken(ctx, token, query, variables)
}

func (c *Client) graphQLWithToken(ctx context.Context, token, query string, variables json.RawMessage) ([]byte, int, error) {
	if variables == nil {
		variables = json.RawMessage("null")
	}
	if !json.Valid(variables) {
		return nil, 0, errors.New("VARIABLES_JSON is not valid JSON")
	}
	payload, err := json.Marshal(struct {
		Query     string          `json:"query"`
		Variables json.RawMessage `json:"variables"`
	}{query, variables})
	if err != nil {
		return nil, 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, graphQLTimeout)
	defer cancel()
	body, status, err := c.post(ctx, token, payload)
	if err != nil {
		return nil, 0, fmt.Errorf("could not reach %s: %s", c.GraphQLURL, transportReason(err))
	}
	return body, status, nil
}

// post sends a GraphQL body with the token as a bearer header.
func (c *Client) post(ctx context.Context, token string, payload []byte) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.GraphQLURL, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, 0, err
	}
	return body, resp.StatusCode, nil
}
