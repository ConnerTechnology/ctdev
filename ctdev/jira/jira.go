// Package jira holds the Atlassian API token ctdev keeps per Jira Cloud site,
// for the scripts that call Jira's REST API (CON-139), and the local MCP
// server entry for Atlassian's Rovo MCP server, which signs in on its own with
// OAuth and never sees the token.
//
// The token is the person's own: what a script does with it shows up as them.
// Nothing here logs, prints or puts into an error the token, except
// AuthHeader, whose output is the point.
package jira

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/secretfile"
	"github.com/ConnerTechnology/ctdev/ctdev/state"
)

// Keys in a site's credentials file.
const (
	keyEmail = "JIRA_EMAIL"
	keyToken = "JIRA_API_TOKEN"
)

// ServerName is the MCP server's name in Claude Code.
const ServerName = "atlassian"

// MCPURL is Atlassian's Rovo MCP server (Jira and Confluence Cloud).
const MCPURL = "https://mcp.atlassian.com/v2/mcp"

// TokensURL is where a person creates an Atlassian API token.
const TokensURL = "https://id.atlassian.com/manage-profile/security/api-tokens"

var sitePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ErrNoCredentials means the site has no credentials file yet.
var ErrNoCredentials = errors.New("no API token saved")

// MCPEntry is the "atlassian" server entry, as indented JSON.
func MCPEntry() json.RawMessage {
	b, _ := json.MarshalIndent(struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	}{"http", MCPURL}, "", "  ")
	return b
}

// NormalizeSite turns what a person pastes ("acme", "acme.atlassian.net",
// "https://acme.atlassian.net/jira/") into the site name, and rejects
// anything that isn't a Jira Cloud site.
func NormalizeSite(in string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(in))
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		s = u.Host
	}
	s = strings.TrimSuffix(s, ".atlassian.net")
	if err := ValidateSite(s); err != nil {
		return "", err
	}
	return s, nil
}

// ValidateSite rejects a name that could not be a site's subdomain or a file
// name under the credentials directory.
func ValidateSite(site string) error {
	if !sitePattern.MatchString(site) {
		return fmt.Errorf("invalid Jira site %q: use the name before .atlassian.net (lowercase letters, digits and dashes)", site)
	}
	return nil
}

// BaseURL is a site's REST API root.
func BaseURL(site string) string {
	return "https://" + site + ".atlassian.net"
}

// Credentials are one person's Atlassian account email and API token.
type Credentials struct {
	Email string
	Token string
}

// AuthHeader is the HTTP Authorization header value for the REST API.
func (c Credentials) AuthHeader() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(c.Email+":"+c.Token))
}

// CredentialsDir is where every site's credentials file lives.
func CredentialsDir() string {
	return filepath.Join(state.ConfigDir(), "jira")
}

// CredentialsPath is the credentials file for one site.
func CredentialsPath(site string) string {
	return filepath.Join(CredentialsDir(), site+".env")
}

// LoadCredentials reads a site's credentials file. A missing file is
// ErrNoCredentials.
func LoadCredentials(site string) (Credentials, error) {
	if err := ValidateSite(site); err != nil {
		return Credentials{}, err
	}
	path := CredentialsPath(site)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Credentials{}, fmt.Errorf("%w (%s does not exist)", ErrNoCredentials, path)
	}
	if err != nil {
		return Credentials{}, fmt.Errorf("cannot read %s: %w", path, err)
	}
	vals := secretfile.Parse(data)
	c := Credentials{Email: vals[keyEmail], Token: vals[keyToken]}
	if c.Email == "" || c.Token == "" {
		return Credentials{}, fmt.Errorf("%s does not set %s and %s", path, keyEmail, keyToken)
	}
	return c, nil
}

// SaveCredentials writes a site's credentials file atomically, owner-only, in
// an owner-only directory.
func SaveCredentials(site string, c Credentials) error {
	if err := ValidateSite(site); err != nil {
		return err
	}
	for _, v := range []string{c.Email, c.Token} {
		if v == "" || strings.ContainsAny(v, "\r\n") {
			return errors.New("email and API token must be non-empty single lines")
		}
	}
	content := fmt.Sprintf("%s=%s\n%s=%s\n", keyEmail, c.Email, keyToken, c.Token)
	return secretfile.WritePrivate(CredentialsPath(site), []byte(content))
}

// Sites lists the sites that have a credentials file, sorted.
func Sites() ([]string, error) {
	entries, err := os.ReadDir(CredentialsDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".env")
		if !ok || e.IsDir() || ValidateSite(name) != nil {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// Client checks credentials against a site.
type Client struct {
	HTTP *http.Client
	// BaseURL maps a site to its REST root; tests point it at a local server.
	BaseURL func(site string) string
}

// NewClient is a Client for the real Jira Cloud.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}, BaseURL: BaseURL}
}

// Myself asks the site who the credentials belong to, and returns that
// account's display name.
func (c *Client) Myself(ctx context.Context, site string, creds Credentials) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL(site)+"/rest/api/3/myself", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", creds.AuthHeader())
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot reach %s: %w", BaseURL(site), err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", fmt.Errorf("Jira rejected the email and API token (HTTP %d); the token may be wrong, expired or revoked", resp.StatusCode)
	case http.StatusNotFound:
		return "", fmt.Errorf("%s has no Jira (HTTP 404); check the site name", BaseURL(site))
	default:
		return "", fmt.Errorf("Jira answered HTTP %d", resp.StatusCode)
	}
	var me struct {
		DisplayName string `json:"displayName"`
	}
	if err := json.Unmarshal(body, &me); err != nil || me.DisplayName == "" {
		return "", fmt.Errorf("%s answered, but not as Jira does", BaseURL(site))
	}
	return me.DisplayName, nil
}
