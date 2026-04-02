package sysutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeployFileCreatesNew(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "test.conf")

	err := DeployFile([]byte("new content"), dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(dest)
	if string(got) != "new content" {
		t.Errorf("expected 'new content', got %q", string(got))
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file, got %d", len(entries))
	}
}

func TestDeployFileSkipsIdentical(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "test.conf")
	os.WriteFile(dest, []byte("same"), 0644)

	err := DeployFile([]byte("same"), dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file (no backup), got %d", len(entries))
	}
}

func TestDeployFileBackupsDifferent(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "test.conf")
	os.WriteFile(dest, []byte("old content"), 0644)

	err := DeployFile([]byte("new content"), dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(dest)
	if string(got) != "new content" {
		t.Errorf("expected 'new content', got %q", string(got))
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatalf("expected 2 files (original + backup), got %d", len(entries))
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".bak") {
			backed, _ := os.ReadFile(filepath.Join(dir, e.Name()))
			if string(backed) != "old content" {
				t.Errorf("backup should have old content, got %q", string(backed))
			}
			return
		}
	}
	t.Error("no .bak file found")
}

func TestDeployFileCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "sub", "dir", "test.conf")

	err := DeployFile([]byte("content"), dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(dest)
	if string(got) != "content" {
		t.Errorf("expected 'content', got %q", string(got))
	}
}
