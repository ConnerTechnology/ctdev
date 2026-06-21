package component

import "testing"

func TestRegistryHas42Components(t *testing.T) {
	if len(Registry) != 42 {
		t.Errorf("expected 42 components, got %d", len(Registry))
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
