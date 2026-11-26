package cmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/checklist"
)

func TestSelfInstallScriptURL_UsesReleaseTag(t *testing.T) {
	got := selfInstallScriptURL("v17.2.0")
	want := "https://raw.githubusercontent.com/ConnerTechnology/ctdev/v17.2.0/install.sh"
	if got != want {
		t.Errorf("url = %q, want %q", got, want)
	}
	if strings.Contains(got, "/main/") {
		t.Errorf("self-update must never fetch install.sh from main: %q", got)
	}
}

func TestCtdevUpdateStep_FetchesTagAndInstallsInPlace(t *testing.T) {
	steps := buildUpdateSteps([]checklist.UpdateItem{
		{Name: "ctdev", Source: "ctdev", CurrentVer: "17.1.0", NewVer: "17.2.0"},
	})
	if len(steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(steps))
	}

	var out bytes.Buffer
	if err := steps[0].run(context.Background(), sysutil.Opts{DryRun: true, Stdout: &out}); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := out.String()

	if !strings.Contains(got, "/ConnerTechnology/ctdev/v17.2.0/install.sh") {
		t.Errorf("dry run should fetch the v17.2.0 install.sh, got %q", got)
	}
	if strings.Contains(got, "/main/") {
		t.Errorf("dry run mentions a main URL: %q", got)
	}
	if !strings.Contains(got, "CTDEV_VERSION=v17.2.0") {
		t.Errorf("dry run should pin CTDEV_VERSION=v17.2.0, got %q", got)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if want := "INSTALL_DIR=" + filepath.Dir(exe); !strings.Contains(got, want) {
		t.Errorf("dry run should install next to the running binary (%s), got %q", want, got)
	}
}

func TestUpdateSelf_RejectsOddTag(t *testing.T) {
	for _, ver := range []string{"main", "17.2", "17.2.0/../../main", ""} {
		var out bytes.Buffer
		if err := updateSelf(context.Background(), sysutil.Opts{DryRun: true, Stdout: &out}, ver); err == nil {
			t.Errorf("updateSelf(%q) = nil, want an error", ver)
		}
	}
}

func TestSelfInstallArgs_SurviveSudo(t *testing.T) {
	got := selfInstallArgs("/usr/local/bin", "v17.2.0", "/tmp/install.sh")
	want := []string{"env", "INSTALL_DIR=/usr/local/bin", "CTDEV_VERSION=v17.2.0", "bash", "/tmp/install.sh"}
	if !slices.Equal(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
}

// withSelfInstallDir points the self-update at dir for the rest of the test.
func withSelfInstallDir(t *testing.T, dir string) {
	t.Helper()
	orig := selfInstallDir
	selfInstallDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { selfInstallDir = orig })
}

// readOnlyDir is a directory the current user can't create files in, like
// /usr/local/bin for a non-root user.
func readOnlyDir(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root can write to any directory")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	return dir
}

func TestUpdateNeedsRoot_CtdevInUnwritableDir(t *testing.T) {
	withSelfInstallDir(t, readOnlyDir(t))
	items := []checklist.UpdateItem{{Source: "ctdev", Name: "ctdev"}}
	if !updateNeedsRoot(items) {
		t.Error("updateNeedsRoot() = false for a ctdev binary in an unwritable dir, want true")
	}
}

func TestUpdateNeedsRoot_CtdevInWritableDir(t *testing.T) {
	withSelfInstallDir(t, t.TempDir())
	items := []checklist.UpdateItem{{Source: "ctdev", Name: "ctdev"}}
	if updateNeedsRoot(items) {
		t.Error("updateNeedsRoot() = true for a ctdev binary in a writable dir, want false")
	}
}

func TestUpdateSelf_DryRunSaysAsRoot(t *testing.T) {
	dir := readOnlyDir(t)
	withSelfInstallDir(t, dir)
	var out bytes.Buffer
	if err := updateSelf(context.Background(), sysutil.Opts{DryRun: true, Stdout: &out}, "17.2.0"); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "(as root)") || !strings.Contains(got, "INSTALL_DIR="+dir) {
		t.Errorf("dry run = %q, want it to say (as root) and INSTALL_DIR=%s", got, dir)
	}
}

func TestDownloadVerifiedArchive_ChecksumAndCleanup(t *testing.T) {
	payload := []byte("example-archive-bytes")
	sum := sha256.Sum256(payload)
	goodHash := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tool.tar.gz":
			_, _ = w.Write(payload)
		case "/good.sha256":
			_, _ = w.Write([]byte(goodHash + "  tool.tar.gz\n"))
		case "/bad.sha256":
			_, _ = w.Write([]byte(strings.Repeat("0", 64) + "  tool.tar.gz\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Run("checksum mismatch skips install", func(t *testing.T) {
		installed := false
		err := downloadVerifiedArchive(context.Background(), srv.URL+"/tool.tar.gz", srv.URL+"/bad.sha256",
			func(cs string) (string, error) { return strings.Fields(cs)[0], nil },
			func(tmpDir, archivePath string) error { installed = true; return nil })
		if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
			t.Fatalf("err = %v, want checksum mismatch", err)
		}
		if installed {
			t.Error("install callback ran despite checksum mismatch")
		}
	})

	t.Run("checksum match runs install and cleans temp dir", func(t *testing.T) {
		var capturedDir string
		err := downloadVerifiedArchive(context.Background(), srv.URL+"/tool.tar.gz", srv.URL+"/good.sha256",
			func(cs string) (string, error) { return strings.Fields(cs)[0], nil },
			func(tmpDir, archivePath string) error {
				capturedDir = tmpDir
				if _, err := os.Stat(archivePath); err != nil {
					t.Errorf("archive missing during install: %v", err)
				}
				return nil
			})
		if err != nil {
			t.Fatalf("downloadVerifiedArchive: %v", err)
		}
		if capturedDir == "" {
			t.Fatal("install callback did not run")
		}
		if _, err := os.Stat(capturedDir); !os.IsNotExist(err) {
			t.Errorf("temp dir %s was not cleaned up", capturedDir)
		}
	})
}

func TestBuildUpdateSteps_CoversSources(t *testing.T) {
	items := []checklist.UpdateItem{
		{Name: "curl", Source: "apt"},
		{Name: "git", Source: "apt"},
		{Name: "org.example.App", Source: "flatpak"},
		{Name: "jq", Source: "brew"},
		{Name: "visual-studio-code", Source: "brew-cask"},
		{Name: "zsh-autosuggestions", Source: "git"},
		{Name: "bun", Source: "runtime", CurrentVer: "1.0", NewVer: "1.1"},
		{Name: "go", Source: "runtime", CurrentVer: "1.24", NewVer: "1.25"},
		{Name: "helm", Source: "cli", CurrentVer: "3.15", NewVer: "3.16"},
		{Name: "pihole", Source: "docker"},
	}
	steps := buildUpdateSteps(items)
	if len(steps) != 9 {
		t.Fatalf("got %d steps, want 9 (2 apt items batched into 1): %+v", len(steps), steps)
	}
	if steps[0].name != "apt (2 packages)" {
		t.Errorf("steps[0].name = %q, want apt (2 packages)", steps[0].name)
	}
}

func TestHTTPVersionFetchers_StatusAndParsing(t *testing.T) {
	status := http.StatusOK
	body := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	origGo, origKube := goReleasesURL, kubectlStableURL
	goReleasesURL, kubectlStableURL = srv.URL, srv.URL
	t.Cleanup(func() { goReleasesURL, kubectlStableURL = origGo, origKube })

	body = `[{"version":"go1.26.1","stable":true,"files":[]}]`
	if ver, err := fetchLatestGoVersion(context.Background()); err != nil || ver != "1.26.1" {
		t.Errorf("fetchLatestGoVersion() = %q, %v; want 1.26.1", ver, err)
	}

	body = "v1.33.2\n"
	if ver, err := fetchLatestKubectlVersion(context.Background()); err != nil || ver != "1.33.2" {
		t.Errorf("fetchLatestKubectlVersion() = %q, %v; want 1.33.2", ver, err)
	}

	status = http.StatusInternalServerError
	body = "<html>error</html>"
	if _, err := fetchLatestGoVersion(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("fetchLatestGoVersion(500) = %v, want HTTP 500 error", err)
	}
	if _, err := fetchLatestKubectlVersion(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("fetchLatestKubectlVersion(500) = %v, want HTTP 500 error", err)
	}
}
