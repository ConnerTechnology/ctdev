package shell

import (
	"bytes"
	"context"
	"testing"
)

func TestRun(t *testing.T) {
	var stdout bytes.Buffer
	err := Run(context.Background(), "echo", []string{"hello"}, RunOpts{
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stdout.String() != "hello\n" {
		t.Errorf("unexpected stdout: %q", stdout.String())
	}
}

func TestExitCode(t *testing.T) {
	if ExitCode(nil) != 0 {
		t.Error("nil error should return 0")
	}
	err := Run(context.Background(), "bash", []string{"-c", "exit 2"}, RunOpts{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	if ExitCode(err) != 2 {
		t.Errorf("expected exit code 2, got %d", ExitCode(err))
	}
}

func TestBoolEnv(t *testing.T) {
	if BoolEnv("FOO", true) != "FOO=true" {
		t.Error("expected FOO=true")
	}
	if BoolEnv("FOO", false) != "FOO=false" {
		t.Error("expected FOO=false")
	}
}
