package component

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

func TestParseEntireVersion(t *testing.T) {
	cases := map[string]string{
		"Entire CLI 0.13.4\nGo version: go1.27.4\nOS/Arch: linux/amd64\n": "0.13.4",
		"Entire CLI v0.12.2\n":      "0.12.2",
		"  Entire CLI 1.0.0  ":      "1.0.0",
		"entire: command not found": "",
		"":                          "",
	}
	for in, want := range cases {
		if got := parseEntireVersion(in); got != want {
			t.Errorf("parseEntireVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEntireArchiveName(t *testing.T) {
	for arch, want := range map[string]string{"amd64": "entire_linux_amd64.tar.gz", "arm64": "entire_linux_arm64.tar.gz"} {
		got, err := entireArchiveName(arch)
		if err != nil || got != want {
			t.Errorf("entireArchiveName(%q) = %q, %v; want %q", arch, got, err, want)
		}
	}
	if _, err := entireArchiveName("386"); !errors.Is(err, ErrUnsupportedOS) {
		t.Errorf("unsupported arch must wrap ErrUnsupportedOS so it reports as skipped, got %v", err)
	}
}

func TestEntireChecksumFor(t *testing.T) {
	sums := "aaa  entire_darwin_arm64.tar.gz\nBBB *entire_linux_amd64.tar.gz\nccc  git-remote-entire-linux-amd64.tar.gz\n"
	if got := entireChecksumFor(sums, "entire_linux_amd64.tar.gz"); got != "BBB" {
		t.Errorf("got %q, want BBB", got)
	}
	if got := entireChecksumFor(sums, "entire_linux_arm64.tar.gz"); got != "" {
		t.Errorf("got %q for an archive the file doesn't list", got)
	}
}

func serve(t *testing.T, status int, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestEntireChannelVersion(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		want    string
		wantErr bool
	}{
		{"tag wins", 200, `{"channel":"stable","tag":"v0.13.4","version":"0.13.3"}`, "0.13.4", false},
		{"version when no tag", 200, `{"channel":"stable","version":"0.13.4"}`, "0.13.4", false},
		{"nightly channel", 200, `{"channel":"nightly","tag":"v0.14.0-nightly.1"}`, "", true},
		{"not a release number", 200, `{"channel":"stable","tag":"v../../evil"}`, "", true},
		{"prerelease on stable", 200, `{"channel":"stable","tag":"v0.14.0-rc1"}`, "", true},
		{"not json", 200, `<html>`, "", true},
		{"http error", 503, `down`, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := entireChannelVersion(context.Background(), serve(t, c.status, c.body))
			if (err != nil) != c.wantErr || got != c.want {
				t.Errorf("got %q, %v; want %q (error %v)", got, err, c.want, c.wantErr)
			}
		})
	}
}

// entireTestArchive builds a release archive holding the named files, each
// containing its own name, as Entire's does (plus a README that isn't a binary).
func entireTestArchive(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, n := range append([]string{"README.md"}, names...) {
		body := []byte("#!/bin/sh\necho " + n + "\n")
		if err := tw.WriteHeader(&tar.Header{Name: n, Mode: 0o755, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		tw.Write(body)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// entireReleaseServer serves files under /v<ver>/ and counts its requests.
func entireReleaseServer(t *testing.T, files map[string][]byte) (string, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &hits
}

const entireTestArchiveName = "entire_linux_amd64.tar.gz"

func TestFetchEntireRelease_FallsBackWhenAnOriginIsDown(t *testing.T) {
	archive := entireTestArchive(t, "entire", "git-remote-entire")
	down, _ := entireReleaseServer(t, nil)
	mirror, _ := entireReleaseServer(t, map[string][]byte{
		"/v0.13.4/" + entireTestArchiveName: archive,
		"/v0.13.4/checksums.txt":            []byte(sha(archive) + "  " + entireTestArchiveName + "\n"),
	})
	origins := []entireOrigin{{"GitHub", down}, {"install.entire.com", mirror}}

	var out bytes.Buffer
	path, err := fetchEntireRelease(context.Background(), &out, origins, "0.13.4", entireTestArchiveName, t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, archive) {
		t.Error("returned path doesn't hold the mirror's archive")
	}
	if !strings.Contains(out.String(), "from GitHub, trying the next source") {
		t.Errorf("the fallback should be said; got %q", out.String())
	}
}

func TestFetchEntireRelease_MismatchStopsWithoutTryingTheMirror(t *testing.T) {
	archive := entireTestArchive(t, "entire", "git-remote-entire")
	bad, _ := entireReleaseServer(t, map[string][]byte{
		"/v0.13.4/" + entireTestArchiveName: archive,
		"/v0.13.4/checksums.txt":            []byte(strings.Repeat("0", 64) + "  " + entireTestArchiveName + "\n"),
	})
	mirror, mirrorHits := entireReleaseServer(t, map[string][]byte{
		"/v0.13.4/" + entireTestArchiveName: archive,
		"/v0.13.4/checksums.txt":            []byte(sha(archive) + "  " + entireTestArchiveName + "\n"),
	})
	origins := []entireOrigin{{"GitHub", bad}, {"install.entire.com", mirror}}

	_, err := fetchEntireRelease(context.Background(), &bytes.Buffer{}, origins, "0.13.4", entireTestArchiveName, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("want a checksum mismatch error, got %v", err)
	}
	if mirrorHits.Load() != 0 {
		t.Error("a mismatch must stop the install, not be retried against the mirror")
	}
}

func TestFetchEntireRelease_UnlistedArchiveAdvancesAndAllDownFails(t *testing.T) {
	archive := entireTestArchive(t, "entire", "git-remote-entire")
	unlisted, _ := entireReleaseServer(t, map[string][]byte{
		"/v0.13.4/" + entireTestArchiveName: archive,
		"/v0.13.4/checksums.txt":            []byte("abc  something_else.tar.gz\n"),
	})
	down, _ := entireReleaseServer(t, nil)
	origins := []entireOrigin{{"GitHub", unlisted}, {"install.entire.com", down}}

	_, err := fetchEntireRelease(context.Background(), &bytes.Buffer{}, origins, "0.13.4", entireTestArchiveName, t.TempDir())
	if err == nil {
		t.Fatal("no origin could serve a checked archive, so the fetch must fail")
	}
	for _, want := range []string{"does not list", "install.entire.com"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

// linuxOnly skips on macOS, where the cask is the install.
func linuxOnly(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the ~/.local/bin install is Linux's")
	}
}

func writeExec(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestEntireInstalled_RecognizesAHandInstall(t *testing.T) {
	linuxOnly(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := filepath.Join(home, ".local", "bin")

	if entireInstalled() {
		t.Fatal("nothing is installed yet")
	}
	writeExec(t, filepath.Join(bin, "entire"), "x")
	if entireInstalled() {
		t.Error("entire without git-remote-entire isn't a whole install")
	}
	writeExec(t, filepath.Join(bin, "git-remote-entire"), "x")
	if !entireInstalled() {
		t.Error("both binaries in ~/.local/bin, from a hand install or ctdev, count as installed")
	}
}

// The whole Linux install path against a fake origin: a hand install already
// in ~/.local/bin is replaced in place, and no second copy is left behind.
func TestInstallEntireRelease_ReplacesAHandInstallInPlace(t *testing.T) {
	linuxOnly(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := filepath.Join(home, ".local", "bin")
	writeExec(t, filepath.Join(bin, "entire"), "old")
	writeExec(t, filepath.Join(bin, "git-remote-entire"), "old")

	archive := entireTestArchive(t, "entire", "git-remote-entire")
	url, _ := entireReleaseServer(t, map[string][]byte{
		"/v0.13.4/" + entireTestArchiveName: archive,
		"/v0.13.4/checksums.txt":            []byte(sha(archive) + "  " + entireTestArchiveName + "\n"),
	})
	saved := entireOrigins
	entireOrigins = []entireOrigin{{"GitHub", url}}
	t.Cleanup(func() { entireOrigins = saved })

	o := sysutil.Opts{Stdout: &bytes.Buffer{}}
	if err := installEntireRelease(context.Background(), o, "amd64", "0.13.4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, b := range entireBinaries {
		got, err := os.ReadFile(filepath.Join(bin, b))
		if err != nil || !strings.Contains(string(got), "echo "+b) {
			t.Errorf("%s was not replaced with the release's: %q, %v", b, got, err)
		}
	}
	entries, _ := os.ReadDir(bin)
	if len(entries) != 2 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("~/.local/bin should hold exactly the two binaries, got %v", names)
	}
}

func TestInstallEntireRelease_RefusesAnArchiveMissingTheHelper(t *testing.T) {
	linuxOnly(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := filepath.Join(home, ".local", "bin")
	writeExec(t, filepath.Join(bin, "entire"), "old")

	archive := entireTestArchive(t, "entire")
	url, _ := entireReleaseServer(t, map[string][]byte{
		"/v0.13.4/" + entireTestArchiveName: archive,
		"/v0.13.4/checksums.txt":            []byte(sha(archive) + "  " + entireTestArchiveName + "\n"),
	})
	saved := entireOrigins
	entireOrigins = []entireOrigin{{"GitHub", url}}
	t.Cleanup(func() { entireOrigins = saved })

	o := sysutil.Opts{Stdout: &bytes.Buffer{}}
	if err := installEntireRelease(context.Background(), o, "amd64", "0.13.4"); err == nil {
		t.Fatal("an archive without git-remote-entire must fail the install")
	}
	if got, _ := os.ReadFile(filepath.Join(bin, "entire")); string(got) != "old" {
		t.Error("a failed install must leave the existing entire as it was")
	}
}

func TestInstallEntireRelease_RefusesANonReleaseVersion(t *testing.T) {
	o := sysutil.Opts{DryRun: true, Stdout: &bytes.Buffer{}}
	for _, ver := range []string{"", "0.14.0-nightly.1", "../0.13.4"} {
		if err := installEntireRelease(context.Background(), o, "amd64", ver); err == nil {
			t.Errorf("version %q should be refused before it reaches a URL", ver)
		}
	}
}

func readEntireConfig(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("config.json isn't JSON: %v\n%s", err, data)
	}
	return m
}

func TestDisableEntireAutoUpdate(t *testing.T) {
	t.Run("creates the file", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "entire")
		t.Setenv("ENTIRE_CONFIG_DIR", dir)
		if err := disableEntireAutoUpdate(sysutil.Opts{Stdout: &bytes.Buffer{}}); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "config.json")
		if m := readEntireConfig(t, path); m["no_auto_update"] != true {
			t.Errorf("got %v", m)
		}
		if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v; the directory holds credentials, so the file stays private", fi.Mode().Perm())
		}
	})

	t.Run("keeps the other keys", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("ENTIRE_CONFIG_DIR", dir)
		path := filepath.Join(dir, "config.json")
		os.WriteFile(path, []byte(`{"no_auto_update": false, "example_key": {"nested": 1}}`), 0o644)
		if err := disableEntireAutoUpdate(sysutil.Opts{Stdout: &bytes.Buffer{}}); err != nil {
			t.Fatal(err)
		}
		m := readEntireConfig(t, path)
		if m["no_auto_update"] != true || m["example_key"] == nil {
			t.Errorf("got %v", m)
		}
		if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o644 {
			t.Errorf("mode = %v, want the file's own 0644 kept", fi.Mode().Perm())
		}
	})

	t.Run("already off is left untouched", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("ENTIRE_CONFIG_DIR", dir)
		path := filepath.Join(dir, "config.json")
		orig := []byte(`{"no_auto_update":true}`)
		os.WriteFile(path, orig, 0o600)
		var out bytes.Buffer
		if err := disableEntireAutoUpdate(sysutil.Opts{Stdout: &out}); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(path); !bytes.Equal(got, orig) || out.Len() != 0 {
			t.Errorf("file = %s, output = %q; want both unchanged", got, out.String())
		}
	})

	t.Run("invalid JSON is left alone", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("ENTIRE_CONFIG_DIR", dir)
		path := filepath.Join(dir, "config.json")
		orig := []byte(`{"no_auto_update": tru`)
		os.WriteFile(path, orig, 0o600)
		var out bytes.Buffer
		if err := disableEntireAutoUpdate(sysutil.Opts{Stdout: &out}); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(path); !bytes.Equal(got, orig) {
			t.Errorf("a file that isn't JSON must not be rewritten; got %s", got)
		}
		if !strings.Contains(out.String(), "isn't valid JSON") {
			t.Errorf("should say why it was skipped; got %q", out.String())
		}
	})

	t.Run("dry run writes nothing", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("ENTIRE_CONFIG_DIR", dir)
		var out bytes.Buffer
		if err := disableEntireAutoUpdate(sysutil.Opts{DryRun: true, Stdout: &out}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, "config.json")); err == nil {
			t.Error("dry run created config.json")
		}
		if !strings.Contains(out.String(), "no_auto_update") {
			t.Errorf("dry run should say what it would set; got %q", out.String())
		}
	})
}

func TestOtherEntireOnPath(t *testing.T) {
	ours := filepath.Join(t.TempDir(), "entire")
	writeExec(t, ours, "x")
	other := filepath.Join(t.TempDir(), "entire")
	writeExec(t, other, "x")

	t.Setenv("PATH", filepath.Dir(ours))
	if got := otherEntireOnPath([]string{ours}); got != "" {
		t.Errorf("only ours is on PATH, got %q", got)
	}
	t.Setenv("PATH", filepath.Dir(ours)+string(os.PathListSeparator)+filepath.Dir(other))
	if got := otherEntireOnPath([]string{ours}); got != other {
		t.Errorf("got %q, want %q", got, other)
	}
}

func TestEntireUninstall_DryRunLeavesUserData(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var out bytes.Buffer
	if err := entireUninstall(context.Background(), ExecOpts{DryRun: true, Stdout: &out}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	bin := filepath.Join(home, ".local", "bin")
	wants := []string{"rm -f " + filepath.Join(bin, "entire") + "\n", "rm -f " + filepath.Join(bin, "git-remote-entire") + "\n"}
	if runtime.GOOS == "darwin" {
		wants = []string{"brew uninstall --cask entireio/tap/entire"}
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in dry-run output; got:\n%s", want, got)
		}
	}
	if strings.Contains(got, ".config/entire") {
		t.Errorf("uninstall must leave login contexts and credentials alone; got:\n%s", got)
	}
}

func TestEntireRegistered(t *testing.T) {
	c := componentByName["entire"]
	if c == nil {
		t.Fatal("entire is not in the registry")
	}
	if c.GoInstall == nil || c.GoUninstall == nil || c.Detect == nil {
		t.Error("entire needs an installer, an uninstaller and its own Detect")
	}
	if c.Root != RootNever {
		t.Error("entire installs into ~/.local/bin or through Homebrew, so it never needs root")
	}
	if c.Category != CategoryCLI {
		t.Errorf("category = %v, want CLI", c.Category)
	}
}

// ctdev installs Entire and nothing more: signing in and each repo's Entire
// settings belong to that repo, so the component never runs those commands.
func TestEntireNeverSignsInOrTouchesARepo(t *testing.T) {
	src, err := os.ReadFile("entire.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"enable", "login", "configure", "auth", "disable", "agent"} {
		if strings.Contains(string(src), `"`+sub+`"`) {
			t.Errorf("entire.go passes %q as an argument; the component must not run `entire %s`", sub, sub)
		}
	}
}
