package component

import (
	"bytes"
	"context"
	"testing"
)

func TestSimplePackageInstallerSkipsIfInstalled(t *testing.T) {
	installer := SimplePackageInstaller("go")
	var buf bytes.Buffer
	err := installer(context.Background(), ExecOpts{
		Stdout: &buf,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("already installed")) {
		t.Errorf("expected 'already installed' message, got: %s", buf.String())
	}
}

func TestSimplePackageInstallerForceBypass(t *testing.T) {
	installer := SimplePackageInstaller("go")
	var buf bytes.Buffer
	err := installer(context.Background(), ExecOpts{
		Stdout: &buf,
		DryRun: true,
		Force:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bytes.Contains(buf.Bytes(), []byte("already installed")) {
		t.Error("expected force to bypass installed check")
	}
}
