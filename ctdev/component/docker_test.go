package component

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// requireGPG skips where the fingerprint check can't run. The apt path is
// Linux-only; macOS installs Docker Desktop through Homebrew.
func requireGPG(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the docker apt key is checked only on Linux")
	}
	if _, err := exec.LookPath("gpg"); err != nil {
		t.Skip("gpg not installed")
	}
}

// testdata/docker.asc is the key download.docker.com served on 2026-11-05.
// If Docker rotates its key, this is the test that says the pin is stale.
func TestDockerKeyFingerprintMatchesDocker(t *testing.T) {
	requireGPG(t)
	if err := requireKeyFingerprint(context.Background(), "testdata/docker.asc", dockerKeyFingerprint); err != nil {
		t.Fatal(err)
	}
}

// A key server handing out some other key must stop the install before the key
// reaches /etc/apt/keyrings.
func TestDockerAddKeyRejectsWrongKey(t *testing.T) {
	requireGPG(t)
	wrongKey, err := Configs.ReadFile(awsPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(wrongKey)
	}))
	defer srv.Close()

	keyring := filepath.Join(t.TempDir(), "docker.asc")
	var out bytes.Buffer
	err = dockerAddKey(context.Background(), sysutil.Opts{Stdout: &out}, srv.URL, keyring)
	if err == nil || !strings.Contains(err.Error(), "not Docker's signing key") || !strings.Contains(err.Error(), "FB5DB77FD5C118B80511ADA8A6310ACC4672475C") {
		t.Fatalf("expected a fingerprint failure, got %v", err)
	}
	if _, statErr := os.Stat(keyring); statErr == nil {
		t.Error("a key with the wrong fingerprint was installed")
	}
}

// Two keys in one file fail even when one of them is Docker's: apt would
// trust both.
func TestRequireKeyFingerprintRejectsExtraKey(t *testing.T) {
	requireGPG(t)
	docker, err := os.ReadFile("testdata/docker.asc")
	if err != nil {
		t.Fatal(err)
	}
	other, err := Configs.ReadFile(awsPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	both := filepath.Join(t.TempDir(), "both.asc")
	if err := os.WriteFile(both, append(append(docker, '\n'), other...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := requireKeyFingerprint(context.Background(), both, dockerKeyFingerprint); err == nil {
		t.Error("a file with a second key alongside Docker's should fail")
	}
}

func TestDockerWriteDaemonConfigWritesWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "docker", "daemon.json")
	var out bytes.Buffer
	wrote, err := dockerWriteDaemonConfig(context.Background(), sysutil.Opts{Stdout: &out, DryRun: true}, path)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote || !strings.Contains(out.String(), "write "+path) {
		t.Errorf("a missing daemon.json should be written, got wrote=%v:\n%s", wrote, out.String())
	}
}

func TestDockerWriteDaemonConfigKeepsExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.json")
	const owners = `{"data-root": "/srv/docker"}`
	if err := os.WriteFile(path, []byte(owners), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	wrote, err := dockerWriteDaemonConfig(context.Background(), sysutil.Opts{Stdout: &out, DryRun: true}, path)
	if err != nil {
		t.Fatal(err)
	}
	if wrote || out.Len() != 0 {
		t.Errorf("an existing daemon.json must be left alone, got wrote=%v:\n%s", wrote, out.String())
	}
	if got, _ := os.ReadFile(path); string(got) != owners {
		t.Errorf("daemon.json changed: %s", got)
	}
}

func TestDockerDaemonJSONRotatesLogs(t *testing.T) {
	var got struct {
		LogDriver string            `json:"log-driver"`
		LogOpts   map[string]string `json:"log-opts"`
	}
	if err := json.Unmarshal([]byte(dockerDaemonJSON), &got); err != nil {
		t.Fatalf("daemon.json isn't valid JSON: %v", err)
	}
	if got.LogDriver != "json-file" || got.LogOpts["max-size"] != "10m" || got.LogOpts["max-file"] != "3" || len(got.LogOpts) != 2 {
		t.Errorf("unexpected daemon.json: %+v", got)
	}
}

// Only a daemon that was already running when daemon.json was written needs a
// restart; a fresh install's daemon starts with it.
func TestDockerStartService(t *testing.T) {
	for _, tc := range []struct {
		restart bool
		want    string
	}{
		{false, "systemctl start docker.service"},
		{true, "systemctl restart docker.service"},
	} {
		var out bytes.Buffer
		if err := dockerStartService(context.Background(), sysutil.Opts{Stdout: &out, DryRun: true}, tc.restart); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Errorf("restart=%v: want %q, got:\n%s", tc.restart, tc.want, out.String())
		}
	}
}
