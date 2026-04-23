package sysutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	if err := DownloadFile(srv.URL, dest); err != nil {
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
	err := DownloadFile(srv.URL, dest)
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
	err := DownloadFile(srv.URL, dest)
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
