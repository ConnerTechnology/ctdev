package sysutil

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallUserBinaryPreservesOldBinaryOnFailure(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "tool")
	const old = "old synthetic binary"
	if err := os.WriteFile(dest, []byte(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := InstallUserBinary(context.Background(), Opts{Stdout: io.Discard}, filepath.Join(dir, "missing"), dest); err == nil {
		t.Fatal("expected copy failure")
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != old {
		t.Fatalf("existing binary changed: %q, %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "tool" {
		t.Fatalf("staging file left behind: %v, %v", entries, err)
	}
}

func TestInstallUserBinaryDryRunDoesNotCreateDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	if err := InstallUserBinary(context.Background(), Opts{Stdout: io.Discard, DryRun: true}, "synthetic-source", filepath.Join(dir, "tool")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("dry-run created destination directory: %v", err)
	}
}

func TestInstallUserBinaryReplacesAtomically(t *testing.T) {
	dir := t.TempDir()
	src, dest := filepath.Join(dir, "source"), filepath.Join(dir, "tool")
	if err := os.WriteFile(src, []byte("new synthetic binary"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("old synthetic binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	old, err := os.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if err := InstallUserBinary(context.Background(), Opts{Stdout: io.Discard}, src, dest); err != nil {
		t.Fatal(err)
	}
	if got, err := io.ReadAll(old); err != nil || string(got) != "old synthetic binary" {
		t.Fatalf("replaced binary was overwritten in place: %q, %v", got, err)
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != "new synthetic binary" {
		t.Fatalf("destination not replaced: %q, %v", got, err)
	}
}
