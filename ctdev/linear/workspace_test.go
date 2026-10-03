package linear

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// isolate points the config and cache directories at a temp dir.
func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	return dir
}

func TestValidateWorkspace(t *testing.T) {
	for _, ok := range []string{"acme", "a", "0", "acme-2", "my-org-x"} {
		if err := ValidateWorkspace(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "-acme", "Acme", "acme_x", "acme.x", "../etc", "a/b", "acme ", " acme"} {
		if err := ValidateWorkspace(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestCredentialsRoundTripAndModes(t *testing.T) {
	isolate(t)
	want := Credentials{ClientID: "test-client-id", ClientSecret: "test-secret"}
	if err := SaveCredentials("acme", want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCredentials("acme")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("round trip: got %+v, want %+v", got, want)
	}

	assertMode(t, CredentialsDir(), 0o700)
	assertMode(t, CredentialsPath("acme"), 0o600)

	// Overwriting keeps the mode and leaves no temp files behind.
	if err := SaveCredentials("acme", Credentials{"test-client-id-2", "test-secret-2"}); err != nil {
		t.Fatal(err)
	}
	assertMode(t, CredentialsPath("acme"), 0o600)
	entries, _ := os.ReadDir(CredentialsDir())
	if len(entries) != 1 {
		t.Errorf("expected only acme.env in the directory, got %d entries", len(entries))
	}
}

func TestCredentialsTightenLooseDirectory(t *testing.T) {
	isolate(t)
	if err := os.MkdirAll(CredentialsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SaveCredentials("acme", Credentials{"test-client-id", "test-secret"}); err != nil {
		t.Fatal(err)
	}
	assertMode(t, CredentialsDir(), 0o700)
}

func TestLoadCredentialsParsesWithoutExecuting(t *testing.T) {
	isolate(t)
	if err := os.MkdirAll(CredentialsDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "ran")
	content := "# a comment\n\nexport LINEAR_CLIENT_ID=\"test-client-id\"\n" +
		"LINEAR_CLIENT_SECRET=test-secret\n$(touch " + marker + ")\n"
	if err := os.WriteFile(CredentialsPath("acme"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCredentials("acme")
	if err != nil {
		t.Fatal(err)
	}
	if got.ClientID != "test-client-id" || got.ClientSecret != "test-secret" {
		t.Errorf("got %+v", got)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the credentials file was executed")
	}
}

func TestLoadCredentialsMissing(t *testing.T) {
	isolate(t)
	if _, err := LoadCredentials("acme"); !errors.Is(err, ErrNoCredentials) {
		t.Errorf("expected ErrNoCredentials, got %v", err)
	}
}

func TestLoadCredentialsIncomplete(t *testing.T) {
	isolate(t)
	_ = os.MkdirAll(CredentialsDir(), 0o700)
	_ = os.WriteFile(CredentialsPath("acme"), []byte("LINEAR_CLIENT_ID=test-client-id\n"), 0o600)
	_, err := LoadCredentials("acme")
	if err == nil || errors.Is(err, ErrNoCredentials) {
		t.Errorf("expected an incomplete-file error, got %v", err)
	}
}

func TestSaveCredentialsRejectsMultiline(t *testing.T) {
	isolate(t)
	if err := SaveCredentials("acme", Credentials{"test-client-id", "test\nLINEAR_CLIENT_ID=x"}); err == nil {
		t.Error("a secret with a newline should be rejected")
	}
}

func TestWorkspacesLists(t *testing.T) {
	isolate(t)
	if ws, err := Workspaces(); err != nil || len(ws) != 0 {
		t.Fatalf("no directory: got %v, %v", ws, err)
	}
	for _, w := range []string{"zeta", "acme"} {
		if err := SaveCredentials(w, Credentials{"test-client-id", "test-secret"}); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(CredentialsDir(), "notes.txt"), nil, 0o600)
	ws, err := Workspaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 2 || ws[0] != "acme" || ws[1] != "zeta" {
		t.Errorf("got %v", ws)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != want {
		t.Errorf("%s: mode %o, want %o", path, got, want)
	}
}
