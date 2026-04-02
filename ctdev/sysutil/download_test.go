package sysutil

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

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
