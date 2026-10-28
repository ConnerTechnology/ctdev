package component

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

const herdrTestManifest = `{
  "version": "2.0.0",
  "sha256": {"linux-x86_64": "latestsum"},
  "releases": {
    "2.0.0": {"sha256": {"linux-x86_64": "latestsum", "linux-aarch64": "ARM2SUM"}},
    "1.9.0": {"sha256": {"linux-x86_64": "oldsum"}}
  }
}`

func herdrManifestServer(t *testing.T, status int, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestHerdrChecksum_UsesTheRequestedVersion(t *testing.T) {
	url := herdrManifestServer(t, 200, herdrTestManifest)
	got, err := herdrChecksum(context.Background(), url, "1.9.0", "linux-x86_64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "oldsum" {
		t.Errorf("got %q, want the 1.9.0 hash, not the latest one", got)
	}
}

func TestHerdrChecksum_FailsClosed(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		ver    string
		target string
	}{
		"version not in manifest": {200, herdrTestManifest, "3.0.0", "linux-x86_64"},
		"target not in release":   {200, herdrTestManifest, "1.9.0", "linux-aarch64"},
		"empty hash":              {200, `{"releases":{"1.0.0":{"sha256":{"linux-x86_64":""}}}}`, "1.0.0", "linux-x86_64"},
		"not json":                {200, "<html>", "2.0.0", "linux-x86_64"},
		"http error":              {503, "down", "2.0.0", "linux-x86_64"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			url := herdrManifestServer(t, c.status, c.body)
			got, err := herdrChecksum(context.Background(), url, c.ver, c.target)
			if err == nil {
				t.Fatalf("expected an error, got hash %q", got)
			}
		})
	}
}

func TestHerdrTarget(t *testing.T) {
	for arch, want := range map[string]string{"amd64": "linux-x86_64", "arm64": "linux-aarch64"} {
		got, err := herdrTarget(arch)
		if err != nil || got != want {
			t.Errorf("herdrTarget(%q) = %q, %v; want %q", arch, got, err, want)
		}
	}
	_, err := herdrTarget("386")
	if !errors.Is(err, ErrUnsupportedOS) {
		t.Errorf("unsupported arch must wrap ErrUnsupportedOS so it reports as skipped, got %v", err)
	}
}

func TestHerdrUninstall_DryRun(t *testing.T) {
	var out bytes.Buffer
	if err := herdrUninstall(context.Background(), ExecOpts{DryRun: true, Stdout: &out}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	want := "rm -f /usr/local/bin/herdr"
	if runtime.GOOS == "darwin" {
		want = "brew uninstall herdr"
	}
	if !strings.Contains(got, want) {
		t.Errorf("expected %q in dry-run output; got:\n%s", want, got)
	}
	if strings.Contains(got, ".config/herdr") {
		t.Errorf("uninstall must leave herdr's config and sessions alone; got:\n%s", got)
	}
}

func TestHerdrRegistered(t *testing.T) {
	c := componentByName["herdr"]
	if c == nil {
		t.Fatal("herdr is not in the registry")
	}
	if c.GoInstall == nil || c.GoUninstall == nil {
		t.Error("herdr needs both an installer and an uninstaller")
	}
}
