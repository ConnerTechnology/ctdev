package component

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// --skip-existing makes `ctdev install --force` idempotent when the target
// Ruby version is already installed.
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
