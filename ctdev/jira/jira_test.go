package jira

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
}

func TestNormalizeSite(t *testing.T) {
	for in, want := range map[string]string{
		"example":               "example",
		"Example.atlassian.net": "example",
		" https://example-2.atlassian.net/jira/ ": "example-2",
	} {
		if got, err := NormalizeSite(in); err != nil || got != want {
			t.Errorf("%q: got %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "../etc", "a/b", "-x", "example.invalid"} {
		if _, err := NormalizeSite(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestCredentialsRoundTripAndModes(t *testing.T) {
	isolate(t)
	if _, err := LoadCredentials("example"); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("missing: got %v", err)
	}
	want := Credentials{Email: "test@example.invalid", Token: "test-token"}
	if err := SaveCredentials("example", want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCredentials("example")
	if err != nil || got != want {
		t.Fatalf("got %+v, %v", got, err)
	}
	fi, _ := os.Stat(CredentialsPath("example"))
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %o", fi.Mode().Perm())
	}
	if err := SaveCredentials("example", Credentials{Email: "x", Token: "a\nb"}); err == nil {
		t.Error("a multi-line token should be rejected")
	}
	sites, err := Sites()
	if err != nil || len(sites) != 1 || sites[0] != "example" {
		t.Errorf("sites %v, %v", sites, err)
	}
}

func TestAuthHeader(t *testing.T) {
	h := Credentials{Email: "test@example.invalid", Token: "test-token"}.AuthHeader()
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(h, "Basic "))
	if err != nil || string(raw) != "test@example.invalid:test-token" {
		t.Errorf("header %q decodes to %q", h, raw)
	}
}

func TestMyself(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/myself" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if user, pass, _ := r.BasicAuth(); user != "test@example.invalid" || pass != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"accountId":"x","displayName":"Example Person"}`)
	}))
	t.Cleanup(srv.Close)
	c := &Client{HTTP: srv.Client(), BaseURL: func(string) string { return srv.URL }}
	ctx := context.Background()

	name, err := c.Myself(ctx, "example", Credentials{"test@example.invalid", "test-token"})
	if err != nil || name != "Example Person" {
		t.Errorf("got %q, %v", name, err)
	}
	_, err = c.Myself(ctx, "example", Credentials{"test@example.invalid", "test-wrong"})
	if err == nil || !strings.Contains(err.Error(), "rejected") || strings.Contains(err.Error(), "test-wrong") {
		t.Errorf("bad token: %v", err)
	}
	c.BaseURL = func(string) string { return srv.URL + "/nope" }
	if _, err := c.Myself(ctx, "example", Credentials{"test@example.invalid", "test-token"}); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("wrong site: %v", err)
	}
}
