package component

import "testing"

func TestRegistryHas36Components(t *testing.T) {
	if len(Registry) != 36 {
		t.Errorf("expected 36 components, got %d", len(Registry))
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

func TestRegistryAllHaveBashScripts(t *testing.T) {
	for _, c := range Registry {
		if c.BashInstall == "" {
			t.Errorf("component %s missing BashInstall", c.Name)
		}
		if c.BashUninstall == "" {
			t.Errorf("component %s missing BashUninstall", c.Name)
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
