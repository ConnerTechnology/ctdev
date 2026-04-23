package component

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func TestInstallDebWithDepFix_DryRunSkipsVerify(t *testing.T) {
	// Dry-run should never shell out and never run IsPackageInstalled,
	// so the helper must return nil even for a nonsensical package name.
	var buf bytes.Buffer
	o := sysutil.Opts{Stdout: &buf, DryRun: true}
	err := installDebWithDepFix(context.Background(), o, "/tmp/fake.deb", "nonexistent-pkg-xyz")
	if err != nil {
		t.Fatalf("dry-run returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "[dry-run] sudo dpkg -i /tmp/fake.deb") {
		t.Errorf("expected dry-run dpkg line; got:\n%s", out)
	}
}

func TestUnsupportedPMError_WrapsSentinel(t *testing.T) {
	err := unsupportedPMError("docker", "unknown")
	if !errors.Is(err, ErrUnsupportedOS) {
		t.Error("expected errors.Is to match ErrUnsupportedOS")
	}
	msg := err.Error()
	if !strings.Contains(msg, "docker") || !strings.Contains(msg, "unknown") {
		t.Errorf("expected descriptive message with component and pm, got %q", msg)
	}
}

func TestAlreadyInstalled_UsesRegistryIsInstalled(t *testing.T) {
	tmp := t.TempDir()
	// Register a component whose DetectApps points at a path we can toggle.
	present := tmp + "/Test.app"
	RegisterForTest(t, Component{
		Name:       "rc-already-test",
		DetectApps: []string{present},
	})
	// componentByName is populated from Registry at package init, so we need
	// to refresh it for the test-registered entry to show up.
	old := componentByName
	t.Cleanup(func() { componentByName = old })
	componentByName = rebuildComponentIndex()

	if alreadyInstalled("rc-already-test") {
		t.Error("expected not-installed before path is created")
	}
	if err := os.Mkdir(present, 0o755); err != nil {
		t.Fatal(err)
	}
	if !alreadyInstalled("rc-already-test") {
		t.Error("expected installed once DetectApps path exists")
	}
}

func TestAlreadyInstalled_UnknownNameFalse(t *testing.T) {
	if alreadyInstalled("does-not-exist-xyz") {
		t.Error("expected false for name absent from registry")
	}
}

// rebuildComponentIndex re-populates componentByName from the current Registry.
// Exposed for tests that dynamically register components.
func rebuildComponentIndex() map[string]*Component {
	m := make(map[string]*Component, len(Registry))
	for i := range Registry {
		m[Registry[i].Name] = &Registry[i]
	}
	return m
}

func TestUnsupportedPMError_ExecutorMapsToSkipped(t *testing.T) {
	exec := NewExecutor()
	c := &Component{
		Name: "test-unsupported",
		GoInstall: func(ctx context.Context, opts ExecOpts) error {
			return unsupportedPMError("test-unsupported", "weirdpm")
		},
		GoUninstall: func(ctx context.Context, opts ExecOpts) error { return nil },
	}
	result := exec.Install(context.Background(), c, ExecOpts{})
	if !result.Skipped {
		t.Errorf("expected Skipped=true via ErrUnsupportedOS wrap; got Skipped=%v, Err=%v", result.Skipped, result.Err)
	}
	if result.Err != nil {
		t.Errorf("expected Err=nil when skipped, got %v", result.Err)
	}
}

func TestSimplePackageInstallerSkipsIfInstalled(t *testing.T) {
	installer := SimplePackageInstaller("go")
	var buf bytes.Buffer
	err := installer(context.Background(), ExecOpts{
		Stdout: &buf,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("already installed")) {
		t.Errorf("expected 'already installed' message, got: %s", buf.String())
	}
}

func TestSimplePackageInstallerForceBypass(t *testing.T) {
	installer := SimplePackageInstaller("go")
	var buf bytes.Buffer
	err := installer(context.Background(), ExecOpts{
		Stdout: &buf,
		DryRun: true,
		Force:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bytes.Contains(buf.Bytes(), []byte("already installed")) {
		t.Error("expected force to bypass installed check")
	}
}
