package component

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExecutorBashBridge(t *testing.T) {
	dir := t.TempDir()

	scriptDir := filepath.Join(dir, "components", "test")
	os.MkdirAll(scriptDir, 0755)

	installScript := filepath.Join(scriptDir, "install.sh")
	os.WriteFile(installScript, []byte("#!/usr/bin/env bash\necho 'installed test'\n"), 0755)

	exec := NewExecutor(dir)
	c := &Component{
		Name:        "test",
		BashInstall: "components/test/install.sh",
	}

	var stdout bytes.Buffer
	result := exec.Install(context.Background(), c, ExecOpts{
		Stdout: &stdout,
		Stderr: os.Stderr,
	})

	if result.Err != nil {
		t.Fatalf("install failed: %v", result.Err)
	}
	if stdout.String() != "installed test\n" {
		t.Errorf("unexpected stdout: %q", stdout.String())
	}
}

func TestExecutorExitCode2Skipped(t *testing.T) {
	dir := t.TempDir()

	scriptDir := filepath.Join(dir, "components", "test")
	os.MkdirAll(scriptDir, 0755)

	installScript := filepath.Join(scriptDir, "install.sh")
	os.WriteFile(installScript, []byte("#!/usr/bin/env bash\nexit 2\n"), 0755)

	exec := NewExecutor(dir)
	c := &Component{
		Name:        "test",
		BashInstall: "components/test/install.sh",
	}

	result := exec.Install(context.Background(), c, ExecOpts{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})

	if !result.Skipped {
		t.Error("expected result to be skipped for exit code 2")
	}
}

func TestExecutorGoInstallOverride(t *testing.T) {
	exec := NewExecutor(t.TempDir())

	called := false
	c := &Component{
		Name: "test",
		GoInstall: func(ctx context.Context, opts ExecOpts) error {
			called = true
			return nil
		},
	}

	result := exec.Install(context.Background(), c, ExecOpts{})
	if result.Err != nil {
		t.Fatalf("install failed: %v", result.Err)
	}
	if !called {
		t.Error("expected GoInstall to be called")
	}
}
