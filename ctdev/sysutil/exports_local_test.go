package sysutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetLineInFileAppends(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "exports.local.zsh")
	os.WriteFile(f, []byte("# existing content\n"), 0644)

	err := SetLineInFile(f, "AWS_PROFILE", "export AWS_PROFILE=test-profile")
	if err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(f)
	if !strings.Contains(string(got), "export AWS_PROFILE=test-profile") {
		t.Errorf("expected AWS_PROFILE line, got: %s", string(got))
	}
	if !strings.Contains(string(got), "# existing content") {
		t.Error("existing content should be preserved")
	}
}

func TestSetLineInFileReplaces(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "exports.local.zsh")
	os.WriteFile(f, []byte("# header\nexport AWS_PROFILE=old-value\n# footer\n"), 0644)

	err := SetLineInFile(f, "AWS_PROFILE", "export AWS_PROFILE=new-value")
	if err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(f)
	content := string(got)
	if !strings.Contains(content, "export AWS_PROFILE=new-value") {
		t.Errorf("expected new value, got: %s", content)
	}
	if strings.Contains(content, "old-value") {
		t.Error("old value should be replaced")
	}
}

func TestSetLineInFileCreatesFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "new.zsh")

	err := SetLineInFile(f, "AWS_PROFILE", "export AWS_PROFILE=test")
	if err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(f)
	if !strings.Contains(string(got), "export AWS_PROFILE=test") {
		t.Errorf("expected line in new file, got: %s", string(got))
	}
}

func TestAppendLineIfMissing(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.zsh")
	os.WriteFile(f, []byte("# stuff\n"), 0644)

	added, _ := AppendLineIfMissing(f, "alias lc='colorls -lA --sd'")
	if !added {
		t.Error("expected line to be added")
	}

	added, _ = AppendLineIfMissing(f, "alias lc='colorls -lA --sd'")
	if added {
		t.Error("expected line to be skipped (already present)")
	}
}
