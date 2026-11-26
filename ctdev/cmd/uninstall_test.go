package cmd

import (
	"bytes"
	"context"
	"strings"
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

func TestRunInstallAndUninstall_EdgeCases(t *testing.T) {
	t.Run("install unknown component", func(t *testing.T) {
		savedBatch := flagBatch
		flagBatch = true
		t.Cleanup(func() { flagBatch = savedBatch })
		err := runInstall(installCmd, []string{"no-such-component-xyz"})
		if err == nil || !strings.Contains(err.Error(), "unknown component") {
			t.Errorf("runInstall(unknown) = %v, want unknown component error", err)
		}
	})

	t.Run("install batch with no args", func(t *testing.T) {
		savedBatch := flagBatch
		flagBatch = true
		t.Cleanup(func() { flagBatch = savedBatch })
		err := runInstall(installCmd, nil)
		if err == nil || !strings.Contains(err.Error(), "no components specified") {
			t.Errorf("runInstall(batch, nil) = %v, want no components error", err)
		}
	})

	t.Run("install resolves dependencies in order", func(t *testing.T) {
		var order []string
		comp.RegisterForTest(t, comp.Component{
			Name: "test-dep-leaf", Root: comp.RootNever,
			GoInstall: func(context.Context, comp.ExecOpts) error { order = append(order, "leaf"); return nil },
		})
		comp.RegisterForTest(t, comp.Component{
			Name: "test-dep-parent", Root: comp.RootNever, Dependencies: []string{"test-dep-leaf"},
			GoInstall: func(context.Context, comp.ExecOpts) error { order = append(order, "parent"); return nil },
		})
		savedBatch, savedDryRun := flagBatch, flagDryRun
		flagBatch, flagDryRun = true, false
		t.Cleanup(func() { flagBatch, flagDryRun = savedBatch, savedDryRun })

		out := captureStdout(t, func() {
			if err := runInstall(installCmd, []string{"test-dep-parent"}); err != nil {
				t.Fatalf("runInstall: %v", err)
			}
		})
		if strings.Join(order, ",") != "leaf,parent" {
			t.Errorf("install order = %v, want [leaf parent]", order)
		}
		if !strings.Contains(out, "1 dependencies: test-dep-leaf") {
			t.Errorf("expected dependency announcement in output:\n%s", out)
		}
	})

	t.Run("uninstall unknown and batch no args", func(t *testing.T) {
		savedBatch := flagBatch
		flagBatch = true
		t.Cleanup(func() { flagBatch = savedBatch })
		if err := runUninstall(uninstallCmd, []string{"no-such-component-xyz"}); err == nil || !strings.Contains(err.Error(), "unknown component") {
			t.Errorf("runUninstall(unknown) = %v", err)
		}
		if err := runUninstall(uninstallCmd, nil); err == nil || !strings.Contains(err.Error(), "no components specified") {
			t.Errorf("runUninstall(batch, nil) = %v", err)
		}
	})

	t.Run("uninstall dry-run skips sudo even for RootAlways", func(t *testing.T) {
		ran := false
		comp.RegisterForTest(t, comp.Component{
			Name:        "test-root-comp",
			Root:        comp.RootAlways,
			Detect:      func() bool { return true },
			GoUninstall: func(context.Context, comp.ExecOpts) error { ran = true; return nil },
		})
		savedBatch, savedDryRun := flagBatch, flagDryRun
		flagBatch, flagDryRun = true, true
		t.Cleanup(func() { flagBatch, flagDryRun = savedBatch, savedDryRun })
		t.Setenv("PATH", t.TempDir()) // no sudo available on PATH

		captureStdout(t, func() {
			if err := runUninstall(uninstallCmd, []string{"test-root-comp"}); err != nil {
				t.Fatalf("runUninstall(--dry-run): %v", err)
			}
		})
		if !ran {
			t.Error("expected dry-run uninstall to invoke GoUninstall without sudo")
		}
	})
}
