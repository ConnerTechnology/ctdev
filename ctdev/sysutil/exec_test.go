package sysutil

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestRun_DryRun(t *testing.T) {
	var buf bytes.Buffer
	o := Opts{Stdout: &buf, DryRun: true}

	if err := Run(context.Background(), o, "echo", "hello", "world"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := buf.String()
	want := "[dry-run] echo hello world\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Cancelling ctx must terminate a long-running child process so Ctrl-C
// propagates from the CLI down into whatever command is currently running.
func TestRun_CancelInterruptsChild(t *testing.T) {
	var buf bytes.Buffer
	o := Opts{Stdout: &buf}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, o, "sleep", "30")
	}()
	// Yield briefly so the child starts, then cancel.
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected error from cancelled Run")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
}

func TestSudoRun_DryRunIncludesSudo(t *testing.T) {
	var buf bytes.Buffer
	o := Opts{Stdout: &buf, DryRun: true}
	if err := SudoRun(context.Background(), o, "apt-get", "update"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "[dry-run] sudo apt-get update\n"
	if buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}

