package component

import "testing"

func TestFilterByOS(t *testing.T) {
	comps := []Component{
		{Name: "docker", SupportedOS: []OS{OSAny}},
		{Name: "cleanmymac", SupportedOS: []OS{OSMacOS}},
		{Name: "earlyoom", SupportedOS: []OS{OSLinux}},
	}

	linux := FilterByOS(comps, OSLinux)
	if len(linux) != 2 {
		t.Errorf("expected 2 linux components, got %d", len(linux))
	}

	macos := FilterByOS(comps, OSMacOS)
	if len(macos) != 2 {
		t.Errorf("expected 2 macos components, got %d", len(macos))
	}
}

func TestResolveDependencies(t *testing.T) {
	comps := []Component{
		{Name: "helm", Dependencies: []string{"kubectl"}},
		{Name: "kubectl"},
		{Name: "docker"},
	}

	deps := ResolveDependencies(comps, []string{"helm"})
	if len(deps) != 2 {
		t.Errorf("expected 2 (helm + kubectl), got %d", len(deps))
	}
	if deps[0] != "kubectl" {
		t.Errorf("expected kubectl first (dependency), got %s", deps[0])
	}
}
