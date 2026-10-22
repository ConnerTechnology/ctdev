package component

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
)

// A dry run of `ctdev install restic --force` shows the package install and
// nothing else: no systemd units, no scripts, no /etc/restic.
func TestResticInstallDryRunIsOnlyThePackage(t *testing.T) {
	pm := platform.Detect().PackageManager
	if pm != "apt" && pm != "brew" {
		t.Skipf("restic installs via apt or Homebrew; this host uses %q", pm)
	}
	var buf bytes.Buffer
	opts := ExecOpts{DryRun: true, Force: true, Stdout: &buf, Stderr: &buf}
	if err := resticInstall(context.Background(), opts); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	var dry []string
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(line, "[dry-run]") {
			dry = append(dry, line)
		}
	}
	if len(dry) != 1 || !strings.Contains(dry[0], "install") || !strings.HasSuffix(dry[0], " restic") {
		t.Errorf("want one package-install line, got:\n%s", buf.String())
	}
}

func TestResticUninstallDryRunIsOnlyThePackage(t *testing.T) {
	pm := platform.Detect().PackageManager
	if pm != "apt" && pm != "brew" {
		t.Skipf("restic installs via apt or Homebrew; this host uses %q", pm)
	}
	var buf bytes.Buffer
	opts := ExecOpts{DryRun: true, Stdout: &buf, Stderr: &buf}
	if err := resticUninstall(context.Background(), opts); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	var dry []string
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(line, "[dry-run]") {
			dry = append(dry, line)
		}
	}
	if len(dry) != 1 || !strings.HasSuffix(dry[0], " restic") {
		t.Errorf("want one package-remove line, got:\n%s", buf.String())
	}
}
