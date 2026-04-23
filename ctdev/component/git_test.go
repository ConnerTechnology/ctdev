package component

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitUninstall_PreservedMessageOnlyWhenLocalExists(t *testing.T) {
	t.Run("prints preserved when .gitconfig.local exists", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		if err := os.WriteFile(filepath.Join(home, ".gitconfig.local"), []byte("[user]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := gitUninstall(context.Background(), ExecOpts{Stdout: &out}); err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		if !strings.Contains(out.String(), ".gitconfig.local preserved") {
			t.Errorf("expected preserved message; got:\n%s", out.String())
		}
	})

	t.Run("omits preserved when no .gitconfig.local exists", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		var out bytes.Buffer
		if err := gitUninstall(context.Background(), ExecOpts{Stdout: &out}); err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		if strings.Contains(out.String(), ".gitconfig.local preserved") {
			t.Errorf("did not expect preserved message; got:\n%s", out.String())
		}
	})
}
