package cmd

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/ConnerTechnology/ctdev/ctdev/gpu"
	"github.com/ConnerTechnology/ctdev/ctdev/setup"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// fakeBins puts shell-script stand-ins for system commands in a fresh
// directory and makes it the whole PATH, so a test sees only these commands.
func fakeBins(t *testing.T, scripts map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range scripts {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

// uefi makes this test's "machine" boot through UEFI, whatever the host
// running the test does, so mokutil's answer is what counts.
func uefi(t *testing.T) {
	t.Helper()
	gpu.SetEFIDirForTest(t, t.TempDir())
}

// plain strips styling and folds wrapped lines, so a test can look for a
// phrase however the wizard laid it out.
func plain(out string) string {
	return strings.Join(strings.Fields(ansi.Strip(out)), " ")
}

// gpuSettingsForTest returns the registry's gpu settings with detection fixed
// at "not applied yet" and ApplyFunc recording into applied, so nothing touches
// this machine. NotNeededFn is the real one.
func gpuSettingsForTest(applied *[]string) []setup.Setting {
	settings := setup.FilterBySlug(setup.Registry, "gpu")
	for i := range settings {
		name := settings[i].Name
		settings[i].HardwareFn = nil
		settings[i].DetectFunc = func(context.Context) string { return "off" }
		settings[i].ApplyFunc = func(context.Context, sysutil.Opts, string) error {
			*applied = append(*applied, name)
			return nil
		}
	}
	return settings
}

// With Secure Boot off and suspend masked, what --batch runs (InitStates then
// applySettings) applies neither gpu setting, even with --force.
func TestGPUBatch_SkipsSettingsThisMachineDoesNotNeed(t *testing.T) {
	uefi(t)
	fakeBins(t, map[string]string{
		"mokutil":   `echo "SecureBoot disabled"`,
		"systemctl": `echo masked; exit 1`,
	})
	var applied []string
	settings := gpuSettingsForTest(&applied)
	ctx := context.Background()
	for _, force := range []bool{false, true} {
		states := setup.InitStates(ctx, settings)
		captureStdout(t, func() {
			if err := applySettings(ctx, states, force, false, false); err != nil {
				t.Fatal(err)
			}
		})
	}
	if len(applied) != 0 {
		t.Errorf("applied %v, want nothing", applied)
	}
}

// With Secure Boot on and suspend allowed, --batch applies both.
func TestGPUBatch_AppliesSettingsThisMachineNeeds(t *testing.T) {
	uefi(t)
	fakeBins(t, map[string]string{
		"mokutil":   `echo "SecureBoot enabled"`,
		"systemctl": `echo static`,
	})
	var applied []string
	ctx := context.Background()
	states := setup.InitStates(ctx, gpuSettingsForTest(&applied))
	captureStdout(t, func() {
		if err := applySettings(ctx, states, false, false, false); err != nil {
			t.Fatal(err)
		}
	})
	if len(applied) != 2 {
		t.Errorf("applied %v, want both gpu settings", applied)
	}
}

func TestGPUShow_SaysWhyAndWhetherNeeded(t *testing.T) {
	uefi(t)
	fakeBins(t, map[string]string{
		"mokutil":   `echo "SecureBoot disabled"`,
		"systemctl": `echo masked; exit 1`,
	})
	var applied []string
	settings := gpuSettingsForTest(&applied)
	states := setup.InitStates(context.Background(), settings)
	out := plain(captureStdout(t, func() {
		for i := range states {
			showSetting(&states[i], wizardLabelWidth)
		}
	}))
	for _, s := range settings {
		if !strings.Contains(out, s.Name) || !strings.Contains(out, s.Description) {
			t.Errorf("--show should print %q and its why; got %q", s.Name, out)
		}
	}
	for _, reason := range []string{"Secure Boot is off", "never suspends"} {
		if !strings.Contains(out, reason) {
			t.Errorf("--show should say %q; got %q", reason, out)
		}
	}
	if strings.Contains(out, "recommended") {
		t.Errorf("--show shouldn't recommend a setting this machine doesn't need; got %q", out)
	}
}

func TestGPUWizard_NotNeededDefaultsToSkip(t *testing.T) {
	uefi(t)
	fakeBins(t, map[string]string{"mokutil": `echo "SecureBoot disabled"`})
	var applied []string
	settings := gpuSettingsForTest(&applied)[:1]
	ctx := context.Background()
	st := &setup.InitStates(ctx, settings)[0]

	stdinScanner = bufio.NewScanner(strings.NewReader("\n"))
	var changed bool
	out := plain(captureStdout(t, func() {
		var err error
		changed, err = promptSetting(ctx, st)
		if err != nil {
			t.Errorf("promptSetting: %v", err)
		}
	}))
	if changed || st.Enabled {
		t.Errorf("Enter on a not-needed setting: changed=%v enabled=%v, want false/false", changed, st.Enabled)
	}
	for _, want := range []string{settings[0].Description, "Not needed on this machine", "Secure Boot is off", settings[0].NoOffSwitch} {
		if !strings.Contains(out, want) {
			t.Errorf("wizard should print %q; got %q", want, out)
		}
	}

	// Answering yes queues it; TestGPUWizard_ApplyAnywayAsksOnce follows it
	// through the real apply.
	stdinScanner = bufio.NewScanner(strings.NewReader("y\n"))
	st = &setup.InitStates(ctx, settings)[0]
	captureStdout(t, func() {
		var err error
		changed, err = promptSetting(ctx, st)
		if err != nil {
			t.Errorf("promptSetting: %v", err)
		}
	})
	if !changed || !st.Enabled || st.DesiredValue != settings[0].Default {
		t.Errorf("yes on a not-needed setting: changed=%v enabled=%v desired=%q, want true/true/%s", changed, st.Enabled, st.DesiredValue, settings[0].Default)
	}
}

func TestGPUWizard_NeededSaysWhyNoOffSwitch(t *testing.T) {
	uefi(t)
	fakeBins(t, map[string]string{"mokutil": `echo "SecureBoot enabled"`})
	var applied []string
	settings := gpuSettingsForTest(&applied)[:1]
	ctx := context.Background()
	st := &setup.InitStates(ctx, settings)[0]

	stdinScanner = bufio.NewScanner(strings.NewReader("\n"))
	out := plain(captureStdout(t, func() {
		if _, err := promptSetting(ctx, st); err != nil {
			t.Errorf("promptSetting: %v", err)
		}
	}))
	if strings.Contains(out, "Not needed") {
		t.Errorf("a needed setting shouldn't say not needed; got %q", out)
	}
	if !strings.Contains(out, settings[0].NoOffSwitch) {
		t.Errorf("wizard should say why there's no off switch; got %q", out)
	}
}

// Saying yes to "Apply anyway?" runs the real signing setup without it asking
// about Secure Boot again. Nothing but a fake mokutil is on PATH, so setup
// stops at its DKMS check before touching anything.
func TestGPUWizard_ApplyAnywayAsksOnce(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GPU setup is Linux-only")
	}
	uefi(t)
	fakeBins(t, map[string]string{"mokutil": `echo "SecureBoot disabled"`})
	settings := setup.FilterBySlug(setup.Registry, "gpu")[:1]
	settings[0].HardwareFn = nil
	settings[0].DetectFunc = func(context.Context) string { return "unsigned" }
	ctx := context.Background()
	states := setup.InitStates(ctx, settings)

	// Were RunSetup to ask again, it would read this "no" from os.Stdin.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.WriteString("n\n")
	w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })

	stdinScanner = bufio.NewScanner(strings.NewReader("y\n"))
	out := plain(captureStdout(t, func() {
		if _, err := promptSetting(ctx, &states[0]); err != nil {
			t.Errorf("promptSetting: %v", err)
		}
		if err := applySettings(ctx, states, false, false, false); err != nil {
			t.Errorf("applySettings: %v", err)
		}
	}))
	if n := strings.Count(out, "anyway"); n != 1 {
		t.Errorf("want one 'anyway' question, got %d; output %q", n, out)
	}
	if !strings.Contains(out, "NVIDIA DKMS module not found") {
		t.Errorf("want the real signing setup to run (and stop at DKMS); got %q", out)
	}
}

// After the settings, `configure gpu` runs the signing flow only when it's
// needed. --batch says why it skipped; the wizard already said so.
func TestGPUSigningNeeded(t *testing.T) {
	ctx := context.Background()
	uefi(t)
	fakeBins(t, map[string]string{"mokutil": `echo "SecureBoot disabled"`})
	var needed bool
	out := plain(captureStdout(t, func() { needed = gpuSigningNeeded(ctx, true) }))
	if needed || !strings.Contains(out, "Driver signing not needed: Secure Boot is off") {
		t.Errorf("batch, Secure Boot off: needed=%v out=%q, want false and the reason", needed, out)
	}
	out = captureStdout(t, func() { needed = gpuSigningNeeded(ctx, false) })
	if needed || out != "" {
		t.Errorf("wizard, Secure Boot off: needed=%v out=%q, want false and silence", needed, out)
	}

	fakeBins(t, map[string]string{"mokutil": `echo "SecureBoot enabled"`})
	if !gpuSigningNeeded(ctx, true) {
		t.Error("Secure Boot on: signing should be needed")
	}
}
