package component

import (
	"encoding/json"
	"testing"
)

func TestRegistryHas54Components(t *testing.T) {
	if len(Registry) != 54 {
		t.Errorf("expected 54 components, got %d", len(Registry))
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
