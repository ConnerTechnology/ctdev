//go:build unix

package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// b2Standin sets m up with a stand-in restic for a B2 destination: its key
// list reports key c0ffee as the one that opened the repository, unless
// B2_ACCOUNT_KEY is example-revoked, which the backend refuses, or
// example-other-repo, where a different key opens it.
func b2Standin(t *testing.T, m *Machine) *Destination {
	t.Helper()
	m.Restic = filepath.Join(t.TempDir(), "restic")
	script := `#!/bin/sh
case "$1" in
version) echo "restic 0.19.1 compiled with go1.26.4 on linux/amd64" ;;
key)
  case "$B2_ACCOUNT_KEY" in
  example-revoked) echo "Fatal: unable to open repository: b2: unauthorized" >&2; exit 1 ;;
  example-other-repo) echo '[{"id":"0ther","current":true,"userName":"example","hostName":"example-laptop"}]' ;;
  *) echo '[{"id":"c0ffee","current":true,"userName":"example","hostName":"example-laptop"}]' ;;
  esac ;;
esac
`
	if err := os.WriteFile(m.Restic, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	d := &Destination{
		Name: "example-backups", Address: "b2:example-bucket:website", m: m,
		credentials: map[string]string{"B2_ACCOUNT_ID": "example-old-id", "B2_ACCOUNT_KEY": "example-old-key"},
		secretKey:   newSecretKey().String(), computerKey: "example-computer-key", computerKeyID: "c0ffee",
	}
	if err := m.save(d); err != nil {
		t.Fatal(err)
	}
	return d
}

// New backend credentials are saved only once this computer's own key opens
// the repository with them.
func TestSetCredentialsChecksThemBeforeSaving(t *testing.T) {
	m := testMachine(t, "example-laptop")
	d := b2Standin(t, m)
	a := &Account{d: d}
	ctx := context.Background()
	saved := func() string {
		data, err := os.ReadFile(m.destinationFile(d.Name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	before := saved()
	for _, key := range []string{"example-revoked", "example-other-repo"} {
		if err := a.SetCredentials(ctx, map[string]string{"B2_ACCOUNT_ID": "example-new-id", "B2_ACCOUNT_KEY": key}); err == nil {
			t.Errorf("%s: credentials this computer's key can't open the repository with were accepted", key)
		}
		if saved() != before {
			t.Errorf("%s: the destination file changed", key)
		}
	}
	if err := a.SetCredentials(ctx, map[string]string{"B2_ACCOUNT_ID": "example-new-id", "B2_ACCOUNT_KEY": "example-new-key"}); err != nil {
		t.Fatal(err)
	}
	if got := saved(); !strings.Contains(got, "B2_ACCOUNT_KEY=example-new-key") || strings.Contains(got, "example-old-key") {
		t.Errorf("the new credentials weren't saved:\n%s", got)
	}
}
