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

func TestSudoArgv(t *testing.T) {
	tests := []struct {
		name     string
		noPrompt bool
		want     []string
	}{
		{"prompting wrapper", false, []string{"sudo", "apt", "install", "jq"}},
		{"non-prompting wrapper", true, []string{"sudo", "-n", "apt", "install", "jq"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.want
			// As root the wrapper has to disappear entirely: containers run as
			// uid 0 and often have no sudo installed at all.
			if IsRoot() {
				want = []string{"apt", "install", "jq"}
			}
			got := sudoArgv(tt.noPrompt, "apt", []string{"install", "jq"})
			if strings.Join(got, " ") != strings.Join(want, " ") {
				t.Errorf("sudoArgv(%v) = %v, want %v", tt.noPrompt, got, want)
			}
		})
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
