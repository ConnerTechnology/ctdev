package sysutil

import (
	"bytes"
	"context"
	"os"
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

// Cancelling ctx must terminate an already-running child process so Ctrl-C
// propagates from the CLI down into whatever command is currently running.
// Uses a sync-point (a write to stdout before the long sleep) to confirm the
// child has actually started before we cancel — otherwise the test could pass
// because exec.CommandContext short-circuits on an already-cancelled ctx
// without ever launching the process.
func TestRun_CancelInterruptsChild(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	o := Opts{Stdout: pw}

	ctx, cancel := context.WithCancel(context.Background())
	started := time.Now()

	runErr := make(chan error, 1)
	go func() {
		// Script: announce start, then sleep long enough that the cancel
		// has to do the killing — not the shell's own exit.
		runErr <- Run(ctx, o, "sh", "-c", "echo started; sleep 30")
		pw.Close()
	}()

	// Wait for the child's "started" line to prove the process is live.
	ready := make(chan struct{})
	go func() {
		buf := make([]byte, 16)
		pr.Read(buf)
		close(ready)
	}()
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("child never printed 'started' — process didn't launch")
	}

	// Now cancel a running child. The sleep would keep it alive for 30s
	// without the ctx-kill path.
	cancel()
	select {
	case err := <-runErr:
		if err == nil {
			t.Error("expected error from cancelled Run")
		}
		if elapsed := time.Since(started); elapsed > 5*time.Second {
			t.Errorf("Run returned after %v — cancel did not interrupt sleep", elapsed)
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
