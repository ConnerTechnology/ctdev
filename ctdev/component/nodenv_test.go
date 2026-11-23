package component

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
)

// The nodenv component installs nodenv and node-build only. Which Node runs is
// the user's call, so it never installs a version or sets the global one.
func TestNodenvInstallLeavesNodeVersionsAlone(t *testing.T) {
	useHome(t)
	var out bytes.Buffer
	if err := nodenvInstall(context.Background(), ExecOpts{DryRun: true, Force: true, Stdout: &out}); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	got := out.String()
	if platform.Detect().PackageManager == "brew" {
		if !strings.Contains(got, "brew install nodenv node-build") {
			t.Errorf("expected the Homebrew install; got:\n%s", got)
		}
	} else if !strings.Contains(got, "nodenv/nodenv.git") || !strings.Contains(got, "nodenv/node-build.git") {
		t.Errorf("expected both clones; got:\n%s", got)
	}
	for _, never := range []string{" install --skip-existing", " global "} {
		if strings.Contains(got, never) {
			t.Errorf("nodenv install must not run %q; got:\n%s", never, got)
		}
	}
}
