// Package linear holds the Linear OAuth app credentials ctdev keeps per
// workspace, the client-credentials token it gets with them, and the MCP server
// entry that points a repo's Claude Code at Linear through
// `ctdev linear mcp-headers`.
//
// A client-credentials token acts as the app, not as the person, so what a
// Claude session does in Linear shows up as the app and notifies the user. One
// app is created per computer; a workspace is one Linear organization.
//
// Nothing here logs, prints or puts into an error the token or the secret.
package linear

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/secretfile"
	"github.com/ConnerTechnology/ctdev/ctdev/state"
)

// Keys in a workspace's credentials file.
const (
	keyClientID     = "LINEAR_CLIENT_ID"
	keyClientSecret = "LINEAR_CLIENT_SECRET"
)

var workspacePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ErrNoCredentials means the workspace has no credentials file yet.
var ErrNoCredentials = errors.New("no credentials saved")

// ValidateWorkspace rejects a name that could not be a file name under the
// credentials directory: lowercase letters, digits and dashes, not starting
// with a dash.
func ValidateWorkspace(name string) error {
	if !workspacePattern.MatchString(name) {
		return fmt.Errorf("invalid workspace name %q: use lowercase letters, digits and dashes, starting with a letter or digit", name)
	}
	return nil
}

// Credentials are one Linear OAuth app's client ID and secret.
type Credentials struct {
	ClientID     string
	ClientSecret string
}

// CredentialsDir is where every workspace's credentials file lives.
func CredentialsDir() string {
	return filepath.Join(state.ConfigDir(), "linear")
}

// CredentialsPath is the credentials file for one workspace.
func CredentialsPath(workspace string) string {
	return filepath.Join(CredentialsDir(), workspace+".env")
}

// LoadCredentials reads a workspace's credentials file. The file is parsed as
// KEY=VALUE lines and never executed. A missing file is ErrNoCredentials.
func LoadCredentials(workspace string) (Credentials, error) {
	if err := ValidateWorkspace(workspace); err != nil {
		return Credentials{}, err
	}
	path := CredentialsPath(workspace)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Credentials{}, fmt.Errorf("%w (%s does not exist)", ErrNoCredentials, path)
	}
	if err != nil {
		return Credentials{}, fmt.Errorf("cannot read %s: %w", path, err)
	}
	vals := secretfile.Parse(data)
	c := Credentials{ClientID: vals[keyClientID], ClientSecret: vals[keyClientSecret]}
	if c.ClientID == "" || c.ClientSecret == "" {
		return Credentials{}, fmt.Errorf("%s does not set %s and %s", path, keyClientID, keyClientSecret)
	}
	return c, nil
}

// SaveCredentials writes a workspace's credentials file atomically, owner-only,
// in an owner-only directory.
func SaveCredentials(workspace string, c Credentials) error {
	if err := ValidateWorkspace(workspace); err != nil {
		return err
	}
	for _, v := range []string{c.ClientID, c.ClientSecret} {
		if v == "" || strings.ContainsAny(v, "\r\n") {
			return errors.New("client ID and secret must be non-empty single lines")
		}
	}
	content := fmt.Sprintf("%s=%s\n%s=%s\n", keyClientID, c.ClientID, keyClientSecret, c.ClientSecret)
	return secretfile.WritePrivate(CredentialsPath(workspace), []byte(content))
}

// Workspaces lists the workspaces that have a credentials file, sorted.
func Workspaces() ([]string, error) {
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
		if !ok || e.IsDir() || ValidateWorkspace(name) != nil {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
