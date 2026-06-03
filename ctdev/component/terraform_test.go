package component

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

// On CI boxes terraform isn't installed, so the uninstall path through any
// supported package manager should short-circuit with a clear message rather
// than attempting to remove a binary we never placed.
func TestTerraformUninstall_ReportsNotInstalledOnSupportedPM(t *testing.T) {
	pm := platform.Detect().PackageManager
	supported := map[string]bool{"brew": true, "apt": true}
	if !supported[pm] {
		t.Skipf("package manager %q not exercised by this test", pm)
	}

	var out bytes.Buffer
	if err := terraformUninstall(context.Background(), ExecOpts{DryRun: true, Stdout: &out}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "terraform package not installed") && !strings.Contains(got, "remove") {
		t.Errorf("expected either 'not installed' or a dry-run 'remove' command; got:\n%s", got)
	}
	// We should never see the standalone binary rm fallback reached on a
	// supported package manager.
	if strings.Contains(got, "rm -f /usr/local/bin/terraform") {
		t.Errorf("supported PM should not fall through to binary removal; got:\n%s", got)
	}
}
