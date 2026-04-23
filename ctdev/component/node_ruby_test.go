package component

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// The dry-run tests below verify that the args passed to nodenv/rbenv include
// --skip-existing, which makes `ctdev install --force` idempotent when the
// target runtime version is already installed.

func TestNodeInstall_DryRunUsesSkipExisting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out bytes.Buffer
	err := nodeInstall(context.Background(), ExecOpts{
		DryRun: true,
		Force:  true,
		Stdout: &out,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "install --skip-existing "+nodeVersion) {
		t.Errorf("expected 'install --skip-existing %s' in output; got:\n%s", nodeVersion, got)
	}
}

func TestRubyInstall_DryRunUsesSkipExisting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out bytes.Buffer
	err := rubyInstall(context.Background(), ExecOpts{
		DryRun: true,
		Force:  true,
		Stdout: &out,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "install --skip-existing "+rubyVersion) {
		t.Errorf("expected 'install --skip-existing %s' in output; got:\n%s", rubyVersion, got)
	}
}
