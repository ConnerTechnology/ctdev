package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/checklist"
)

// A trimmed `pyenv install --list`: other minors, pre-releases, free-threaded
// builds and other interpreters all sit beside the patches we want.
const pyenvList = `Available versions:
  3.13.9
  3.14.0
  3.14.8
  3.14.10
  3.14.11t
  3.15.0
  3.15.1a1
  pypy3.14-7.3.20
  miniconda3-3.14-25.1.0
`

func TestLatestPythonPatch(t *testing.T) {
	tests := []struct {
		current, want string
	}{
		{"3.14.8", "3.14.10"}, // numeric compare, and 3.14.11t is free-threaded
		{"3.13.2", "3.13.9"},
		{"3.15.0", "3.15.0"}, // the alpha is skipped
		{"3.12.1", ""},       // no definitions for this minor
		{"3", ""},
	}
	for _, tt := range tests {
		if got := latestPythonPatch(pyenvList, tt.current); got != tt.want {
			t.Errorf("latestPythonPatch(%q) = %q, want %q", tt.current, got, tt.want)
		}
	}
}

func TestRuntimeUpdateStep_PythonInstallsThenSetsGlobal(t *testing.T) {
	step, ok := runtimeUpdateStep(checklist.UpdateItem{Source: "runtime", Name: "python (pyenv)", NewVer: "3.14.10"})
	if !ok {
		t.Fatal("python (pyenv) has no update step")
	}
	var out bytes.Buffer
	if err := step.run(context.Background(), sysutil.Opts{DryRun: true, Stdout: &out}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	install := strings.Index(got, "pyenv install --skip-existing 3.14.10")
	global := strings.Index(got, "pyenv global 3.14.10")
	if install < 0 || global < install {
		t.Errorf("expected install then global; got:\n%s", got)
	}
}
