package sysutil

import (
	"bytes"
	"context"
	"testing"
)

func TestInstallPackageDryRun(t *testing.T) {
	var buf bytes.Buffer
	o := Opts{Stdout: &buf, DryRun: true}
	err := InstallPackage(context.Background(), o, "testpkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if out == "" {
		t.Error("expected dry-run output, got empty")
	}
}

func TestRemovePackageDryRun(t *testing.T) {
	var buf bytes.Buffer
	o := Opts{Stdout: &buf, DryRun: true}
	err := RemovePackage(context.Background(), o, "testpkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if out == "" {
		t.Error("expected dry-run output, got empty")
	}
}

func TestIsPackageInstalledKnown(t *testing.T) {
	// "go" should be installed since we're running Go tests
	if !CommandExists("go") {
		t.Skip("go not on PATH")
	}
}

func TestIsPackageInstalledUnknown(t *testing.T) {
	if CommandExists("nonexistent-package-xyz-12345") {
		t.Error("expected nonexistent command to not exist")
	}
}
