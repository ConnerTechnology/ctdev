package sysutil

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHTTPClientCheckRedirect(t *testing.T) {
	check := HTTPClient().CheckRedirect
	if check == nil {
		t.Fatal("HTTPClient().CheckRedirect is nil")
	}

	// Refuse HTTPS -> HTTP downgrade on non-loopback host.
	req, _ := http.NewRequest(http.MethodGet, "http://example.invalid/file", nil)
	if err := check(req, []*http.Request{req}); err == nil {
		t.Error("expected error on non-https redirect to public host")
	}

	// Allow HTTP redirect on loopback (for httptest).
	loopReq, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:8080/file", nil)
	if err := check(loopReq, []*http.Request{loopReq}); err != nil {
		t.Errorf("loopback redirect should be allowed, got: %v", err)
	}

	// Refuse >= 10 hops even over HTTPS.
	httpsReq, _ := http.NewRequest(http.MethodGet, "https://example.invalid/hop", nil)
	via := make([]*http.Request, 10)
	if err := check(httpsReq, via); err == nil {
		t.Error("expected error after 10 redirects")
	}
}

func TestDownloadGitHubBinary_TarGzZipAndUnknownFormat(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX install binary test")
	}

	buildTarGz := func(name, content string) []byte {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))})
		_, _ = tw.Write([]byte(content))
		_ = tw.Close()
		_ = gz.Close()
		return buf.Bytes()
	}
	buildZip := func(name, content string) []byte {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		fw, _ := zw.Create(name)
		_, _ = fw.Write([]byte(content))
		_ = zw.Close()
		return buf.Bytes()
	}

	tgz := buildTarGz("demo-tool", "#!/bin/sh\necho tar\n")
	zp := buildZip("demo-tool", "#!/bin/sh\necho zip\n")
	shaHex := func(b []byte) string {
		s := sha256.Sum256(b)
		return hex.EncodeToString(s[:])
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tool.tar.gz":
			_, _ = w.Write(tgz)
		case "/tool.zip":
			_, _ = w.Write(zp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// tar.gz with UserInstall + Checksum callback
	destTar := filepath.Join(t.TempDir(), "bin", "demo-tar")
	ver, err := DownloadGitHubBinary(context.Background(), Opts{Stdout: &bytes.Buffer{}}, GitHubBinarySpec{
		Repo:        "example/demo",
		Version:     "1.0.0",
		ArchiveURL:  func(string, string, string) string { return srv.URL + "/tool.tar.gz" },
		Checksum:    func(context.Context, string, string, string) (string, error) { return shaHex(tgz), nil },
		BinaryPath:  func(string, string) string { return "demo-tool" },
		InstallDest: destTar,
		ArchFormat:  "tar.gz",
		UserInstall: true,
	})
	if err != nil || ver != "1.0.0" {
		t.Fatalf("DownloadGitHubBinary(tar.gz) = %q, %v", ver, err)
	}
	if data, err := os.ReadFile(destTar); err != nil || !strings.Contains(string(data), "echo tar") {
		t.Errorf("installed tar binary = %q, %v", data, err)
	}

	// zip (if unzip is installed)
	if CommandExists("unzip") {
		destZip := filepath.Join(t.TempDir(), "bin", "demo-zip")
		_, err := DownloadGitHubBinary(context.Background(), Opts{Stdout: &bytes.Buffer{}}, GitHubBinarySpec{
			Repo:        "example/demo",
			Version:     "1.0.0",
			ArchiveURL:  func(string, string, string) string { return srv.URL + "/tool.zip" },
			Checksum:    func(context.Context, string, string, string) (string, error) { return shaHex(zp), nil },
			BinaryPath:  func(string, string) string { return "demo-tool" },
			InstallDest: destZip,
			ArchFormat:  "zip",
			UserInstall: true,
		})
		if err != nil {
			t.Fatalf("DownloadGitHubBinary(zip): %v", err)
		}
		if data, err := os.ReadFile(destZip); err != nil || !strings.Contains(string(data), "echo zip") {
			t.Errorf("installed zip binary = %q, %v", data, err)
		}
	}

	// Unknown ArchFormat returns error rather than silently falling through.
	_, err = DownloadGitHubBinary(context.Background(), Opts{Stdout: &bytes.Buffer{}}, GitHubBinarySpec{
		Repo:        "example/demo",
		Version:     "1.0.0",
		ArchiveURL:  func(string, string, string) string { return srv.URL + "/tool.tar.gz" },
		BinaryPath:  func(string, string) string { return "demo-tool" },
		InstallDest: filepath.Join(t.TempDir(), "bin", "demo-bad"),
		ArchFormat:  "7z",
		UserInstall: true,
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported archive format") {
		t.Errorf("expected unsupported archive format error, got %v", err)
	}
}

func TestDeployFile_ReplacesSymlinkWithoutMutatingTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX symlink test")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	dst := filepath.Join(dir, "dst")
	_ = os.WriteFile(target, []byte("original-target\n"), 0o644)
	if err := os.Symlink(target, dst); err != nil {
		t.Fatal(err)
	}

	if err := DeployFile([]byte("new-content\n"), dst); err != nil {
		t.Fatalf("DeployFile over symlink: %v", err)
	}
	gotDst, _ := os.ReadFile(dst)
	if string(gotDst) != "new-content\n" {
		t.Errorf("dst = %q, want new-content", gotDst)
	}
	fi, err := os.Lstat(dst)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("dst should be a regular file after DeployFile, mode=%v err=%v", fi.Mode(), err)
	}
	gotTarget, _ := os.ReadFile(target)
	if string(gotTarget) != "original-target\n" {
		t.Errorf("symlink target was mutated: %q", gotTarget)
	}
}

func TestPiholeAndShellHelpers(t *testing.T) {
	ctx := context.Background()
	if err := PiholeRun(ctx, Opts{DryRun: true, Stdout: &bytes.Buffer{}}); err == nil {
		t.Error("PiholeRun with empty args should return error")
	}
	if _, err := PiholeCapture(ctx); err == nil {
		t.Error("PiholeCapture with empty args should return error")
	}

	// Native dnsmasq drop-in must use 0644 so non-root dnsmasq can read it.
	t.Setenv("PATH", t.TempDir()) // no docker -> native path
	var out bytes.Buffer
	if err := PiholeWriteDnsmasq(ctx, Opts{DryRun: true, Stdout: &out}, "03-tailnet.conf", "server=/example/100.100.100.100\n"); err != nil {
		t.Fatalf("PiholeWriteDnsmasq dry-run: %v", err)
	}
	if !strings.Contains(out.String(), "/etc/dnsmasq.d/03-tailnet.conf") {
		t.Errorf("expected native dnsmasq path in dry-run output:\n%s", out.String())
	}

	// LoginShell falls back to $SHELL when getent is unavailable.
	t.Setenv("SHELL", "/usr/bin/example-shell")
	if got := LoginShell(ctx); got != "/usr/bin/example-shell" {
		t.Errorf("LoginShell() fallback = %q, want /usr/bin/example-shell", got)
	}

	// Sanity check unused import avoidance for url in case extended later.
	_ = url.Values{}
}
