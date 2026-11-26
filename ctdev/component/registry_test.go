package component

import (
	"encoding/json"
	"testing"
)

func TestRegistryStructuralInvariants(t *testing.T) {
	if len(Registry) == 0 {
		t.Fatal("Registry is empty")
	}
	byName := make(map[string]*Component, len(Registry))
	for i := range Registry {
		c := &Registry[i]
		if c.Name == "" {
			t.Errorf("Registry[%d] has empty Name", i)
		}
		if c.Description == "" {
			t.Errorf("component %q has empty Description", c.Name)
		}
		if c.Category == "" {
			t.Errorf("component %q has empty Category", c.Name)
		}
		if len(c.SupportedOS) == 0 {
			t.Errorf("component %q has empty SupportedOS", c.Name)
		}
		if got := FindByName(c.Name); got == nil {
			t.Errorf("FindByName(%q) returned nil", c.Name)
		}
		byName[c.Name] = c
	}
	if got := FindByName("nonexistent-component-xyz"); got != nil {
		t.Errorf("FindByName(nonexistent) = %+v, want nil", got)
	}

	// Every declared dependency must exist and the dependency graph must be acyclic.
	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)
	state := make(map[string]int, len(Registry))
	var visit func(name string, path []string)
	visit = func(name string, path []string) {
		switch state[name] {
		case visited:
			return
		case visiting:
			t.Errorf("dependency cycle detected: %v -> %s", path, name)
			return
		}
		state[name] = visiting
		c := byName[name]
		for _, dep := range c.Dependencies {
			if byName[dep] == nil {
				t.Errorf("component %q depends on unknown component %q", name, dep)
				continue
			}
			visit(dep, append(path, name))
		}
		state[name] = visited
	}
	for _, c := range Registry {
		visit(c.Name, nil)
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

func TestHelmDependsOnKubectl(t *testing.T) {
	c := FindByName("helm")
	if c == nil {
		t.Fatal("expected to find helm")
	}
	if len(c.Dependencies) != 1 || c.Dependencies[0] != "kubectl" {
		t.Errorf("expected helm to depend on kubectl, got %v", c.Dependencies)
	}
}

// The npm-global components need npm, which runs under the Node nodenv manages,
// and install under $HOME without root.
func TestNpmGlobalComponentsInstallThroughNode(t *testing.T) {
	for _, name := range []string{"ccusage", "codex"} {
		c := FindByName(name)
		if c == nil {
			t.Errorf("%s component missing from registry", name)
			continue
		}
		if len(c.Dependencies) != 1 || c.Dependencies[0] != "nodenv" {
			t.Errorf("expected %s to depend on nodenv, got %v", name, c.Dependencies)
		}
		if c.Root != RootNever {
			t.Errorf("%s installs with npm under $HOME and must declare RootNever, got %v", name, c.Root)
		}
	}
}

func TestCcstatuslineInstallsThroughNodenv(t *testing.T) {
	c := FindByName("ccstatusline")
	if c == nil {
		t.Fatal("ccstatusline component missing from registry")
	}
	if len(c.Dependencies) != 1 || c.Dependencies[0] != "nodenv" {
		t.Errorf("expected ccstatusline to depend on nodenv, got %v", c.Dependencies)
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

func TestClaudeDesktopInstallsOnLinuxAndMacOS(t *testing.T) {
	c := FindByName("claude-desktop")
	if c == nil {
		t.Fatal("claude-desktop component missing from registry")
	}
	if len(c.SupportedOS) != 2 || c.SupportedOS[0] != OSMacOS || c.SupportedOS[1] != OSLinux {
		t.Errorf("expected claude-desktop on macOS and Linux, got %v", c.SupportedOS)
	}
	if c.DetectCmd != "claude-desktop" {
		t.Errorf("expected claude-desktop detected by its command on Linux, got %q", c.DetectCmd)
	}
}
