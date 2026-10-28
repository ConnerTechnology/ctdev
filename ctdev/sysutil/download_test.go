package sysutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadFile_Success(t *testing.T) {
	body := []byte("payload-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	if err := DownloadFile(context.Background(), srv.URL, dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("dest body = %q, want %q", got, body)
	}
}

func TestDownloadFile_HTTPErrorNoFileCreated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	err := DownloadFile(context.Background(), srv.URL, dest)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("expected dest to be absent after HTTP error; stat err = %v", statErr)
	}
}

func TestDownloadFile_PartialTransferCleansUp(t *testing.T) {
	// Advertise a longer Content-Length than we send so io.Copy returns
	// ErrUnexpectedEOF. DownloadFile must remove the partial file.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "short")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	err := DownloadFile(context.Background(), srv.URL, dest)
	if err == nil {
		t.Fatal("expected error from truncated response")
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("expected dest to be removed on partial transfer; stat err = %v", statErr)
	}
}

func TestVerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.bin")
	content := []byte("hello world")
	if err := os.WriteFile(file, content, 0644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(content)
	expected := hex.EncodeToString(h[:])

	if err := VerifyChecksum(file, expected); err != nil {
		t.Errorf("expected valid checksum, got: %v", err)
	}
	if err := VerifyChecksum(file, "0000badchecksum"); err == nil {
		t.Error("expected checksum mismatch error")
	}
}

func TestVerifyChecksumFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.bin")
	content := []byte("test data")
	if err := os.WriteFile(file, content, 0644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(content)
	hash := hex.EncodeToString(h[:])

	checksums := filepath.Join(dir, "SHA256SUMS")
	csContent := hash + "  test.bin\ndeadbeef  other.bin\n"
	if err := os.WriteFile(checksums, []byte(csContent), 0644); err != nil {
		t.Fatal(err)
	}

	if err := VerifyChecksumFile(file, checksums); err != nil {
		t.Errorf("expected valid checksum file match, got: %v", err)
	}
}

func TestVerifyChecksumFileFormats(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.bin")
	content := []byte("test data")
	if err := os.WriteFile(file, content, 0644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(content)
	hash := hex.EncodeToString(h[:])

	cases := map[string]string{
		"binary marker":   hash + " *test.bin\n",
		"uppercase hash":  strings.ToUpper(hash) + "  test.bin\n",
		"trailing column": hash + "  test.bin  extra\n",
	}
	for name, csContent := range cases {
		t.Run(name, func(t *testing.T) {
			checksums := filepath.Join(dir, "SHA256SUMS")
			if err := os.WriteFile(checksums, []byte(csContent), 0644); err != nil {
				t.Fatal(err)
			}
			if err := VerifyChecksumFile(file, checksums); err != nil {
				t.Errorf("expected match for %s format, got: %v", name, err)
			}
		})
	}
}

func TestVerifyChecksumFileMissing(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.bin")
	if err := os.WriteFile(file, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	checksums := filepath.Join(dir, "SHA256SUMS")
	if err := os.WriteFile(checksums, []byte("abc123  other.bin\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := VerifyChecksumFile(file, checksums)
	if err == nil {
		t.Error("expected error for missing file in checksums")
	}
}

func writeTestArchive(t *testing.T, content string) (path, sum string) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "asset.bin")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte(content))
	return path, hex.EncodeToString(h[:])
}

func TestVerifyDownload_ChecksumFuncMatch(t *testing.T) {
	path, sum := writeTestArchive(t, "binary")
	var gotVer, gotOS, gotArch string
	spec := GitHubBinarySpec{Checksum: func(_ context.Context, ver, goos, goarch string) (string, error) {
		gotVer, gotOS, gotArch = ver, goos, goarch
		return strings.ToUpper(sum), nil // manifests may publish uppercase hex
	}}
	if err := verifyDownload(context.Background(), spec, "1.2.3", "linux", "arm64", path, t.TempDir()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotVer != "1.2.3" || gotOS != "linux" || gotArch != "arm64" {
		t.Errorf("Checksum called with (%q, %q, %q)", gotVer, gotOS, gotArch)
	}
}

func TestVerifyDownload_ChecksumFuncMismatch(t *testing.T) {
	path, _ := writeTestArchive(t, "binary")
	spec := GitHubBinarySpec{Checksum: func(context.Context, string, string, string) (string, error) {
		return strings.Repeat("0", 64), nil
	}}
	err := verifyDownload(context.Background(), spec, "1.2.3", "linux", "amd64", path, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestVerifyDownload_ChecksumFuncError(t *testing.T) {
	path, _ := writeTestArchive(t, "binary")
	spec := GitHubBinarySpec{Checksum: func(context.Context, string, string, string) (string, error) {
		return "", fmt.Errorf("no checksum for 1.2.3")
	}}
	err := verifyDownload(context.Background(), spec, "1.2.3", "linux", "amd64", path, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no checksum for 1.2.3") {
		t.Fatalf("a lookup failure must fail the install, got %v", err)
	}
}

func TestVerifyDownload_NoChecksumConfigured(t *testing.T) {
	path, _ := writeTestArchive(t, "binary")
	if err := verifyDownload(context.Background(), GitHubBinarySpec{}, "1.2.3", "linux", "amd64", path, t.TempDir()); err != nil {
		t.Fatalf("specs without a checksum behave as before, got %v", err)
	}
}

func TestVerifyDownload_ChecksumURLStillWorks(t *testing.T) {
	path, sum := writeTestArchive(t, "binary")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", sum, filepath.Base(path))
	}))
	defer srv.Close()
	spec := GitHubBinarySpec{ChecksumURL: func(string, string, string) string { return srv.URL }}
	if err := verifyDownload(context.Background(), spec, "1.2.3", "linux", "amd64", path, t.TempDir()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
