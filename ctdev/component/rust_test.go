package component

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRustInstall_DryRunRunsRustupUnattended(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out bytes.Buffer
	if err := rustInstall(context.Background(), ExecOpts{DryRun: true, Stdout: &out}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "sh.rustup.rs | sh -s -- -y --no-modify-path") {
		t.Errorf("expected an unattended rustup install that leaves PATH to path.zsh; got:\n%s", got)
	}
}

func TestRustUninstall_NoopWhenRustupMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out bytes.Buffer
	if err := rustUninstall(context.Background(), ExecOpts{DryRun: true, Stdout: &out}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out.String(), "self uninstall") {
		t.Errorf("uninstall ran rustup with no rustup present:\n%s", out.String())
	}
}
