package sysutil

import (
	"bytes"
	"testing"
)

func TestRun_DryRun(t *testing.T) {
	var buf bytes.Buffer
	o := Opts{Stdout: &buf, DryRun: true}

	if err := Run(o, "echo", "hello", "world"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := buf.String()
	want := "[dry-run] echo hello world\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

