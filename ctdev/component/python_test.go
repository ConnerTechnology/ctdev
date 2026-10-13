package component

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestPythonInstall_DryRunUsesSkipExisting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out bytes.Buffer
	err := pythonInstall(context.Background(), ExecOpts{
		DryRun: true,
		Force:  true,
		Stdout: &out,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "install --skip-existing "+pythonVersion) {
		t.Errorf("expected 'install --skip-existing %s' in output; got:\n%s", pythonVersion, got)
	}
	if !strings.Contains(got, "global "+pythonVersion) {
		t.Errorf("expected 'global %s' in output; got:\n%s", pythonVersion, got)
	}
}

func TestBrewPythonFormulae(t *testing.T) {
	got := brewPythonFormulae([]string{"git", "python@3.14", "python-tk@3.14", "python", "pythonista", "python@3.12"})
	want := []string{"python@3.14", "python", "python@3.12"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestPlanBrewPythonMigration(t *testing.T) {
	formulae := []string{"python@3.14", "python@3.12", "python@3.11"}
	dependents := map[string][]string{
		"python@3.14": {"gcloud-cli"},
		"python@3.12": {"python-tk@3.12"},
		"python@3.11": nil,
	}
	remove, keep := planBrewPythonMigration(formulae, dependents)
	if want := []string{"python@3.12", "python@3.11"}; !reflect.DeepEqual(remove, want) {
		t.Errorf("remove = %v, want %v", remove, want)
	}
	if want := map[string][]string{"python@3.14": {"gcloud-cli"}}; !reflect.DeepEqual(keep, want) {
		t.Errorf("keep = %v, want %v", keep, want)
	}
}
