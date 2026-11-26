package component

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
)

// On a supported package manager, terraformUninstall reports "not installed"
// when the package is absent and previews package removal when it is present,
// never falling through to standalone binary removal.
func TestTerraformUninstall_SupportedPM(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stubs require unix")
	}
	pm := platform.Detect().PackageManager
	removalVerb, ok := map[string]string{"apt": "remove", "brew": "uninstall"}[pm]
	if !ok {
		t.Skipf("package manager %q not exercised by this test", pm)
	}

	writePMStub := func(t *testing.T, installed bool) {
		t.Helper()
		bin := t.TempDir()
		switch pm {
		case "apt":
			body := "#!/bin/sh\nexit 1\n"
			if installed {
				body = "#!/bin/sh\necho 'installed 1.9.0'\nexit 0\n"
			}
			if err := os.WriteFile(filepath.Join(bin, "dpkg-query"), []byte(body), 0o755); err != nil {
				t.Fatal(err)
			}
		case "brew":
			body := "#!/bin/sh\nexit 1\n"
			if installed {
				body = "#!/bin/sh\nexit 0\n"
			}
			if err := os.WriteFile(filepath.Join(bin, "brew"), []byte(body), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	}

	t.Run("not installed", func(t *testing.T) {
		writePMStub(t, false)
		var out bytes.Buffer
		if err := terraformUninstall(context.Background(), ExecOpts{DryRun: true, Stdout: &out}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := out.String()
		if !strings.Contains(got, "terraform package not installed") {
			t.Errorf("expected 'terraform package not installed', got:\n%s", got)
		}
		if strings.Contains(got, removalVerb+" -y terraform") || strings.Contains(got, "brew uninstall terraform") {
			t.Errorf("should not remove package when not installed; got:\n%s", got)
		}
		if strings.Contains(got, "rm -f /usr/local/bin/terraform") {
			t.Errorf("supported PM should not fall through to binary removal; got:\n%s", got)
		}
	})

	t.Run("installed", func(t *testing.T) {
		writePMStub(t, true)
		var out bytes.Buffer
		if err := terraformUninstall(context.Background(), ExecOpts{DryRun: true, Stdout: &out}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := out.String()
		if !strings.Contains(got, removalVerb) {
			t.Errorf("expected dry-run %q command; got:\n%s", removalVerb, got)
		}
		if strings.Contains(got, "terraform package not installed") {
			t.Errorf("should not claim not installed; got:\n%s", got)
		}
		if strings.Contains(got, "rm -f /usr/local/bin/terraform") {
			t.Errorf("supported PM should not fall through to binary removal; got:\n%s", got)
		}
	})
}
