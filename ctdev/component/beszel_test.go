package component

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func beszelCompose(t *testing.T) string {
	t.Helper()
	b, err := Configs.ReadFile("configs/beszel/docker-compose.yml")
	if err != nil {
		t.Fatalf("read embedded compose file: %v", err)
	}
	return string(b)
}

// The hub defaults its link base to http://localhost:8090, so every alert's
// "View Beszel" button opened localhost on the phone it was pushed to.
func TestBeszelComposePassesAppURLToHub(t *testing.T) {
	compose := beszelCompose(t)
	if !regexp.MustCompile(`(?m)^\s+APP_URL:\s+\$\{BESZEL_APP_URL`).MatchString(compose) {
		t.Error("hub service must set APP_URL from BESZEL_APP_URL in ~/beszel/.env")
	}
}

func TestBeszelAppURL(t *testing.T) {
	if got := beszelAppURL("home.example.com"); got != "https://beszel.home.example.com" {
		t.Errorf("beszelAppURL = %q", got)
	}
	// No Caddy domain means no Caddy name to link to; the compose default stands.
	if got := beszelAppURL(""); got != "" {
		t.Errorf("beszelAppURL(\"\") = %q, want empty", got)
	}
}

// The KEY/TOKEN are hand-pasted by the admin. Writing the app URL on a re-run
// of the install must neither drop them nor loosen the file's mode.
func TestBeszelSetEnvKeepsCredentialsAndMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(beszelStack.dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(beszelEnvPath(), []byte("# pasted\nBESZEL_KEY=k\nBESZEL_TOKEN=t\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := beszelSetEnv(map[string]string{"BESZEL_APP_URL": "https://beszel.home.example.com"}); err != nil {
		t.Fatalf("set env: %v", err)
	}

	env := beszelReadEnv()
	for k, want := range map[string]string{"BESZEL_KEY": "k", "BESZEL_TOKEN": "t", "BESZEL_APP_URL": "https://beszel.home.example.com"} {
		if env[k] != want {
			t.Errorf("%s = %q, want %q", k, env[k], want)
		}
	}
	info, err := os.Stat(beszelEnvPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 0600", info.Mode().Perm())
	}
}

// Before the admin has pasted credentials there is no file at all; the app URL
// must still land so the hub's first start already links correctly.
func TestBeszelSetEnvCreatesFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := beszelSetEnv(map[string]string{"BESZEL_APP_URL": "https://beszel.home.example.com"}); err != nil {
		t.Fatalf("set env: %v", err)
	}
	if !strings.Contains(beszelReadEnv()["BESZEL_APP_URL"], "beszel.home.example.com") {
		t.Error("BESZEL_APP_URL not written")
	}
	if beszelReadEnv()["BESZEL_KEY"] != "" {
		t.Error("BESZEL_KEY should stay absent until pasted")
	}
}
