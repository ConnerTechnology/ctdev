//go:build unix

package component

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAntigravityInstallReportsDownloadFailure(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, curl string
		ok         bool
	}{
		{"cannot download installer", `send 'printf example > "$HOME/.local/bin/agy"'; exit 22`, false},
		{"installer fails", `send 'exit 1'`, false},
		{"installer succeeds", `send 'printf example > "$HOME/.local/bin/agy"'`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bin, home := t.TempDir(), t.TempDir()
			t.Setenv("PATH", bin)
			t.Setenv("HOME", home)
			if err := os.Symlink(bash, filepath.Join(bin, "bash")); err != nil {
				t.Fatal(err)
			}
			curl := `#!/bin/sh
out=/dev/stdout
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then out="$2"; shift; fi
  shift
done
send() { printf '%s\n' "$1" > "$out"; }
` + tt.curl + "\n"
			if err := os.WriteFile(filepath.Join(bin, "curl"), []byte(curl), 0o755); err != nil {
				t.Fatal(err)
			}
			installDir := filepath.Join(home, ".local", "bin")
			if err := os.MkdirAll(installDir, 0o755); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			res := Install(context.Background(), FindByName("antigravity-cli"), ExecOpts{Stdout: &out, Stderr: &out})
			if (res.Err == nil) != tt.ok || res.Skipped {
				t.Errorf("install = %+v, want success %t; output %q", res, tt.ok, &out)
			}
			if _, err := os.Stat(filepath.Join(installDir, "agy")); tt.ok && err != nil || !tt.ok && !os.IsNotExist(err) {
				t.Errorf("installed agy: stat error %v, want present %t", err, tt.ok)
			}
		})
	}
}
