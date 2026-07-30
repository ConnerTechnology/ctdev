package component

import (
	"os"
	"path/filepath"
	"testing"
)

// registerRootFixtures adds one component of each RootNeed kind, in both
// installed and missing states, so the gate can be exercised without depending
// on what's actually installed on the test machine.
func registerRootFixtures(t testing.TB) {
	t.Helper()
	present := filepath.Join(t.TempDir(), "marker")
	if err := os.WriteFile(present, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := "/no/such/path/ctdev-root-test"

	for _, c := range []Component{
		{Name: "test-home-only", DetectPath: present, Root: RootNever},
		{Name: "test-home-only-missing", DetectPath: missing, Root: RootNever},
		{Name: "test-always", DetectPath: present, Root: RootAlways},
		{Name: "test-pkg-present", DetectPath: present},
		{Name: "test-pkg-missing", DetectPath: missing},
	} {
		RegisterForTest(t, c)
	}
}

func TestInstallNeedsRoot(t *testing.T) {
	registerRootFixtures(t)

	tests := []struct {
		name  string
		names []string
		force bool
		want  bool
	}{
		{"home-only component never needs root", []string{"test-home-only-missing"}, false, false},
		{"already-installed package needs nothing", []string{"test-pkg-present"}, false, false},
		{"missing package needs root", []string{"test-pkg-missing"}, false, true},
		{"force re-runs the privileged install", []string{"test-pkg-present"}, true, true},
		{"force does not drag in home-only components", []string{"test-home-only"}, true, false},
		{"installed but privileged every run", []string{"test-always"}, false, true},
		{"one component needing root is enough", []string{"test-home-only", "test-pkg-missing"}, false, true},
		{"unknown names are ignored", []string{"test-nonexistent"}, false, false},
		{"nothing selected", nil, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InstallNeedsRoot(tt.names, tt.force); got != tt.want {
				t.Errorf("InstallNeedsRoot(%v, force=%v) = %v, want %v", tt.names, tt.force, got, tt.want)
			}
		})
	}
}

func TestUninstallNeedsRoot(t *testing.T) {
	registerRootFixtures(t)

	// Removal is the mirror image of install: whatever went outside $HOME has to
	// be taken back out as root, installed state notwithstanding.
	tests := []struct {
		name  string
		names []string
		want  bool
	}{
		{"home-only component", []string{"test-home-only"}, false},
		{"package that is present", []string{"test-pkg-present"}, true},
		{"package that is already gone", []string{"test-pkg-missing"}, true},
		{"privileged every run", []string{"test-always"}, true},
		{"mixed selection", []string{"test-home-only", "test-pkg-present"}, true},
		{"nothing selected", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UninstallNeedsRoot(tt.names); got != tt.want {
				t.Errorf("UninstallNeedsRoot(%v) = %v, want %v", tt.names, got, tt.want)
			}
		})
	}
}

// The devcontainer case that motivated the gate: a shell-config-only install
// must not ask for a password, while anything from a package manager still does.
func TestInstallNeedsRoot_RealRegistry(t *testing.T) {
	if InstallNeedsRoot([]string{"claude-code", "fonts", "bun", "node", "devcontainer"}, true) {
		t.Error("home-only components asked for root")
	}
	if !InstallNeedsRoot([]string{"claude-code", "docker"}, true) {
		t.Error("docker should need root")
	}
	if !InstallNeedsRoot([]string{"restic"}, false) {
		t.Error("restic redeploys /etc units on every run and should need root")
	}
}
