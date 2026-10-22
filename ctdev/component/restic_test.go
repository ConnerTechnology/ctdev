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

// restic is only its binary: a dry run of install (forced) or uninstall shows
// one line and nothing else — no systemd units, no scripts, no system config
// directory. On Linux that line is the pinned upstream release going into
// ~/.local/bin (apt's restic is too old); on macOS it is Homebrew.
func TestResticDryRunIsOnlyTheBinary(t *testing.T) {
	type step struct {
		name string
		run  func(context.Context, ExecOpts) error
		want []string // substrings the one dry-run line must contain
	}
	var steps []step
	switch platform.Detect().OS {
	case platform.Linux:
		steps = []step{
			{"install", resticInstall, []string{"download restic/restic v" + resticVersion, "/.local/bin/restic"}},
			{"uninstall", resticUninstall, []string{"rm -f", "/.local/bin/restic"}},
		}
	case platform.MacOS:
		steps = []step{
			{"install", resticInstall, []string{"brew install restic"}},
			{"uninstall", resticUninstall, []string{"brew uninstall restic"}},
		}
	default:
		t.Skip("restic installs on Linux and macOS only")
	}
	for _, s := range steps {
		var buf bytes.Buffer
		opts := ExecOpts{DryRun: true, Force: true, Stdout: &buf, Stderr: &buf}
		if err := s.run(context.Background(), opts); err != nil {
			t.Fatalf("%s dry run: %v", s.name, err)
		}
		dry := dryRunLines(buf.String())
		if len(dry) != 1 {
			t.Errorf("%s: want one dry-run line, got:\n%s", s.name, buf.String())
			continue
		}
		for _, w := range s.want {
			if !strings.Contains(dry[0], w) {
				t.Errorf("%s: dry-run line %q lacks %q", s.name, dry[0], w)
			}
		}
	}
}
