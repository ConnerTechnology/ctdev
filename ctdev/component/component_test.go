package component

import (
	"os"
	"testing"
)

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

func TestIsInstalled_DetectPath(t *testing.T) {
	t.Run("DetectPath exists", func(t *testing.T) {
		tmp := t.TempDir()
		f := tmp + "/marker"
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		c := Component{Name: "nonexistent_command_xyz", DetectPath: f}
		if !c.IsInstalled() {
			t.Error("expected installed when DetectPath file exists")
		}
	})

	t.Run("DetectPath missing does not fall through", func(t *testing.T) {
		c := Component{Name: "go", DetectPath: "/no/such/path"}
		if c.IsInstalled() {
			t.Error("expected not installed when DetectPath is set but missing")
		}
	})
}

func TestIsInstalled_DetectApps(t *testing.T) {
	t.Run("matching app path", func(t *testing.T) {
		tmp := t.TempDir()
		appDir := tmp + "/Test.app"
		if err := os.Mkdir(appDir, 0o755); err != nil {
			t.Fatal(err)
		}
		c := Component{Name: "nonexistent_command_xyz", DetectApps: []string{appDir}}
		if !c.IsInstalled() {
			t.Error("expected installed when DetectApps path exists")
		}
	})

	t.Run("no matching app falls through to command check", func(t *testing.T) {
		c := Component{Name: "go", DetectApps: []string{"/no/such/App.app"}}
		if !c.IsInstalled() {
			t.Error("expected installed via command fallback for 'go'")
		}
	})

	t.Run("any of multiple paths matches", func(t *testing.T) {
		tmp := t.TempDir()
		present := tmp + "/present.ttf"
		if err := os.WriteFile(present, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		c := Component{
			Name:       "nonexistent_command_xyz",
			DetectApps: []string{"/no/such/absent.ttf", present},
		}
		if !c.IsInstalled() {
			t.Error("expected installed when any DetectApps path exists")
		}
	})
}

func TestFontsRegistry_NoExclusiveDetectPath(t *testing.T) {
	// DetectPath is exclusive (see IsInstalled) so a cross-platform component
	// like fonts must list all candidate paths in DetectApps, not DetectPath.
	c := FindByName("fonts")
	if c == nil {
		t.Fatal("fonts component missing from registry")
	}
	if c.DetectPath != "" {
		t.Errorf("fonts should not use DetectPath (exclusive); got %q", c.DetectPath)
	}
	if len(c.DetectApps) < 2 {
		t.Errorf("fonts should list Linux and macOS font paths in DetectApps; got %v", c.DetectApps)
	}
}

func TestIsInstalled_DetectCmd(t *testing.T) {
	t.Run("DetectCmd set", func(t *testing.T) {
		c := Component{Name: "nonexistent_command_xyz", DetectCmd: "go"}
		if !c.IsInstalled() {
			t.Error("expected installed when DetectCmd points to existing command")
		}
	})

	t.Run("DetectCmd empty uses Name", func(t *testing.T) {
		c := Component{Name: "go"}
		if !c.IsInstalled() {
			t.Error("expected installed when Name is an existing command")
		}

		c2 := Component{Name: "nonexistent_command_xyz"}
		if c2.IsInstalled() {
			t.Error("expected not installed for nonexistent command")
		}
	})
}

func TestGroupByCategory(t *testing.T) {
	t.Run("multiple categories", func(t *testing.T) {
		comps := []Component{
			{Name: "a", Category: CategoryCLI},
			{Name: "b", Category: CategoryDesktop},
			{Name: "c", Category: CategoryCLI},
		}
		groups := GroupByCategory(comps)
		if len(groups) != 2 {
			t.Errorf("expected 2 groups, got %d", len(groups))
		}
		if len(groups[CategoryCLI]) != 2 {
			t.Errorf("expected 2 CLI components, got %d", len(groups[CategoryCLI]))
		}
		if len(groups[CategoryDesktop]) != 1 {
			t.Errorf("expected 1 Desktop component, got %d", len(groups[CategoryDesktop]))
		}
	})

	t.Run("single category", func(t *testing.T) {
		comps := []Component{
			{Name: "a", Category: CategorySystem},
			{Name: "b", Category: CategorySystem},
		}
		groups := GroupByCategory(comps)
		if len(groups) != 1 {
			t.Errorf("expected 1 group, got %d", len(groups))
		}
	})
}

func TestAllNames(t *testing.T) {
	names := AllNames()
	if len(names) != len(Registry) {
		t.Errorf("expected %d names, got %d", len(Registry), len(names))
	}
	for i, name := range names {
		if name != Registry[i].Name {
			t.Errorf("name[%d] = %q, want %q", i, name, Registry[i].Name)
		}
	}
}

func TestSupportsOS(t *testing.T) {
	tests := []struct {
		name        string
		supportedOS []OS
		queryOS     OS
		want        bool
	}{
		{"OSAny matches linux", []OS{OSAny}, OSLinux, true},
		{"OSAny matches macos", []OS{OSAny}, OSMacOS, true},
		{"OSLinux matches linux", []OS{OSLinux}, OSLinux, true},
		{"OSLinux does not match macos", []OS{OSLinux}, OSMacOS, false},
		{"empty SupportedOS", []OS{}, OSLinux, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Component{SupportedOS: tt.supportedOS}
			if got := c.SupportsOS(tt.queryOS); got != tt.want {
				t.Errorf("SupportsOS(%q) = %v, want %v", tt.queryOS, got, tt.want)
			}
		})
	}
}
