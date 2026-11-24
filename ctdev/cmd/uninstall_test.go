package cmd

import (
	"bytes"
	"context"
	"testing"

	comp "github.com/ConnerTechnology/ctdev/ctdev/component"
)

func registerUninstallable(t *testing.T, name string, installed bool) *int {
	t.Helper()
	calls := 0
	comp.RegisterForTest(t, comp.Component{
		Name:        name,
		Root:        comp.RootNever,
		Detect:      func() bool { return installed },
		GoUninstall: func(context.Context, comp.ExecOpts) error { calls++; return nil },
	})
	return &calls
}

func runUninstallBatch(t *testing.T, force bool, args ...string) string {
	t.Helper()
	flagBatch, flagForce = true, force
	t.Cleanup(func() { flagBatch, flagForce = false, false })
	var out bytes.Buffer
	uninstallCmd.SetOut(&out)
	t.Cleanup(func() { uninstallCmd.SetOut(nil) })
	if err := runUninstall(uninstallCmd, args); err != nil {
		t.Fatalf("runUninstall: %v", err)
	}
	return out.String()
}

func TestUninstallSkipsComponentThatIsNotInstalled(t *testing.T) {
	absent := registerUninstallable(t, "test-absent", false)
	present := registerUninstallable(t, "test-present", true)

	out := runUninstallBatch(t, false, "test-absent", "test-present")

	if *absent != 0 {
		t.Errorf("uninstaller ran %d times for a component that isn't installed", *absent)
	}
	if *present != 1 {
		t.Errorf("uninstaller ran %d times for an installed component, want 1", *present)
	}
	if want := "test-absent isn't installed"; !bytes.Contains([]byte(out), []byte(want)) {
		t.Errorf("output %q doesn't say %q", out, want)
	}
}

func TestUninstallForceRunsUninstallerForComponentThatIsNotInstalled(t *testing.T) {
	absent := registerUninstallable(t, "test-absent", false)

	runUninstallBatch(t, true, "test-absent")

	if *absent != 1 {
		t.Errorf("uninstaller ran %d times with --force, want 1", *absent)
	}
}
