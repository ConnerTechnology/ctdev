package component

import (
	"encoding/json"
	"testing"
)

func TestRegistryHas55Components(t *testing.T) {
	if len(Registry) != 55 {
		t.Errorf("expected 55 components, got %d", len(Registry))
	}
}

func TestRegistryNoDuplicates(t *testing.T) {
	seen := make(map[string]bool)
	for _, c := range Registry {
		if seen[c.Name] {
			t.Errorf("duplicate component: %s", c.Name)
		}
		seen[c.Name] = true
	}
}

func TestRegistryAllHaveInstallMethod(t *testing.T) {
	for _, c := range Registry {
		if c.GoInstall == nil || c.GoUninstall == nil {
			t.Errorf("component %s missing GoInstall or GoUninstall", c.Name)
		}
	}
}

func TestFindByName(t *testing.T) {
	c := FindByName("docker")
	if c == nil {
		t.Fatal("expected to find docker")
	}
	if c.Category != CategoryCLI {
		t.Errorf("expected CLI Tools, got %s", c.Category)
	}
}

func TestHelmDependsOnKubectl(t *testing.T) {
	c := FindByName("helm")
	if c == nil {
		t.Fatal("expected to find helm")
	}
	if len(c.Dependencies) != 1 || c.Dependencies[0] != "kubectl" {
		t.Errorf("expected helm to depend on kubectl, got %v", c.Dependencies)
	}
}

func TestCcusageInstallsThroughNode(t *testing.T) {
	c := FindByName("ccusage")
	if c == nil {
		t.Fatal("ccusage component missing from registry")
	}
	if len(c.Dependencies) != 1 || c.Dependencies[0] != "node" {
		t.Errorf("expected ccusage to depend on node, got %v", c.Dependencies)
	}
	if c.Root != RootNever {
		t.Errorf("ccusage installs with npm under $HOME and must declare RootNever, got %v", c.Root)
	}
}

func TestCcstatuslineInstallsThroughNode(t *testing.T) {
	c := FindByName("ccstatusline")
	if c == nil {
		t.Fatal("ccstatusline component missing from registry")
	}
	if len(c.Dependencies) != 1 || c.Dependencies[0] != "node" {
		t.Errorf("expected ccstatusline to depend on node, got %v", c.Dependencies)
	}
	if c.Root != RootNever {
		t.Errorf("ccstatusline installs with npm under $HOME and must declare RootNever, got %v", c.Root)
	}
}

func TestCcstatuslineLayoutIsEmbedded(t *testing.T) {
	data, err := Configs.ReadFile("configs/ccstatusline/settings.json")
	if err != nil {
		t.Fatalf("ccstatusline layout not embedded: %v", err)
	}
	if !json.Valid(data) {
		t.Error("ccstatusline layout is not valid JSON")
	}
}

func TestGoDetectionUsesPath(t *testing.T) {
	// Regression: DetectPath is exclusive in IsInstalled, so pinning it to the
	// tarball location reported a distro-packaged Go as missing — disagreeing
	// with goInstall, which treats any `go` on PATH as installed.
	c := FindByName("go")
	if c == nil {
		t.Fatal("go component missing from registry")
	}
	if c.DetectPath != "" {
		t.Errorf("go must not set DetectPath (got %q) — detection falls through to a PATH lookup", c.DetectPath)
	}
}
