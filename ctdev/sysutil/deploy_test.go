package sysutil

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

func TestConcurrentDeploymentsKeepAllVersions(t *testing.T) {
	const count = 16
	path := filepath.Join(t.TempDir(), "example.conf")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range count {
		wg.Go(func() {
			<-start
			if err := DeployFile([]byte(strconv.Itoa(i)), path); err != nil {
				t.Errorf("deploy %d: %v", i, err)
			}
		})
	}
	close(start)
	wg.Wait()
	backups, err := filepath.Glob(path + ".*.bak")
	if err != nil || len(backups) != count {
		t.Fatalf("backups = %d, err %v", len(backups), err)
	}
	versions := map[string]bool{}
	for _, name := range append(backups, path) {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		versions[string(data)] = true
		fi, err := os.Stat(name)
		if err != nil || fi.Mode().Perm() != 0600 {
			t.Fatalf("lost private mode for %s", name)
		}
	}
	if len(versions) != count+1 || !versions["original"] {
		t.Errorf("versions = %v", versions)
	}
}

func TestUnwritableDeploymentKeepsOriginal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write through permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "example.conf")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	if err := DeployFile([]byte("replacement"), path); err == nil {
		t.Fatal("unwritable deployment succeeded")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "original" {
		t.Fatalf("live file = %q (%v)", got, err)
	}
}
