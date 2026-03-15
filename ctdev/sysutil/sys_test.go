package sysutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommandExistsTrue(t *testing.T) {
	if !CommandExists("go") {
		t.Error("expected 'go' to exist on PATH")
	}
}

func TestCommandExistsFalse(t *testing.T) {
	if CommandExists("nonexistent-cmd-xyz-99999") {
		t.Error("expected nonexistent command to return false")
	}
}

func TestSafeSymlinkCreates(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	dst := filepath.Join(dir, "link.txt")
	os.WriteFile(src, []byte("hello"), 0644)

	if err := SafeSymlink(src, dst); err != nil {
		t.Fatalf("SafeSymlink failed: %v", err)
	}

	target, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("Readlink failed: %v", err)
	}
	if target != src {
		t.Errorf("expected link to %s, got %s", src, target)
	}
}

func TestSafeSymlinkReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	dst := filepath.Join(dir, "link.txt")
	os.WriteFile(src, []byte("hello"), 0644)
	os.WriteFile(dst, []byte("old"), 0644) // existing file at dst

	if err := SafeSymlink(src, dst); err != nil {
		t.Fatalf("SafeSymlink failed: %v", err)
	}

	target, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("Readlink failed: %v", err)
	}
	if target != src {
		t.Errorf("expected link to %s, got %s", src, target)
	}
}
