package sysutil

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestIsRoot(t *testing.T) {
	if got, want := IsRoot(), os.Geteuid() == 0; got != want {
		t.Errorf("IsRoot() = %v, want %v (euid %d)", got, want, os.Geteuid())
	}
}

func TestSudoNoPrompt(t *testing.T) {
	cmd := SudoNoPrompt(context.Background(), "du", "-sk", "/var/log")
	got := strings.Join(cmd.Args, " ")

	// As root the wrapper has to disappear: containers run as uid 0 and often
	// have no sudo installed at all.
	want := "sudo -n du -sk /var/log"
	if IsRoot() {
		want = "du -sk /var/log"
	}
	if !strings.HasSuffix(got, want) {
		t.Errorf("SudoNoPrompt args = %q, want suffix %q", got, want)
	}
}

func TestCheckSudoAccess_NoSudoBinary(t *testing.T) {
	if IsRoot() {
		if got := CheckSudoAccess(context.Background()); got != AlreadyRoot {
			t.Errorf("CheckSudoAccess() as root = %v, want AlreadyRoot", got)
		}
		return
	}
	// An empty PATH is the hardened-container case: no sudo to escalate with.
	t.Setenv("PATH", t.TempDir())
	if got := CheckSudoAccess(context.Background()); got != SudoUnavailable {
		t.Errorf("CheckSudoAccess() without sudo on PATH = %v, want SudoUnavailable", got)
	}
	if CanElevateQuietly(context.Background()) {
		t.Error("CanElevateQuietly() = true without sudo on PATH")
	}
}
