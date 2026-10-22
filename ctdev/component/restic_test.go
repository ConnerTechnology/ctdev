package component

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
)

// dryRunLines returns the "[dry-run]" lines of a dry run's output.
func dryRunLines(out string) []string {
	var dry []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "[dry-run]") {
			dry = append(dry, line)
		}
	}
	return dry
}

// restic is only its package: a dry run of install (forced) or uninstall shows
// one package-manager line and nothing else — no systemd units, no scripts, no
// system config directory.
func TestResticDryRunIsOnlyThePackage(t *testing.T) {
	pm := platform.Detect().PackageManager
	if pm != "apt" && pm != "brew" {
		t.Skipf("restic installs via apt or Homebrew; this host uses %q", pm)
	}
	c := FindByName("restic")
	if c == nil {
		t.Fatal("restic is not in the registry")
	}
	// apt-get says install/remove; brew says install/uninstall.
	for _, tc := range []struct {
		name string
		run  func(context.Context, ExecOpts) error
		verb string
	}{
		{"install", c.GoInstall, " install "},
		{"uninstall", c.GoUninstall, map[string]string{"apt": " remove ", "brew": " uninstall "}[pm]},
	} {
		var buf bytes.Buffer
		opts := ExecOpts{DryRun: true, Force: true, Stdout: &buf, Stderr: &buf}
		if err := tc.run(context.Background(), opts); err != nil {
			t.Fatalf("%s dry run: %v", tc.name, err)
		}
		dry := dryRunLines(buf.String())
		if len(dry) != 1 || !strings.Contains(dry[0], tc.verb) || !strings.HasSuffix(dry[0], " restic") {
			t.Errorf("%s: want one package%sline, got:\n%s", tc.name, tc.verb, buf.String())
		}
	}
}
