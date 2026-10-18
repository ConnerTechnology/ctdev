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
		settings[i].DetectFunc = func(context.Context) string { return "unknown" }
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
	fakeBins(t, map[string]string{
		"mokutil":   `echo "SecureBoot disabled"`,
		"systemctl": `echo masked; exit 1`,
	})
	var applied []string
	settings := gpuSettingsForTest(&applied)
	states := setup.InitStates(context.Background(), settings)
	out := plain(captureStdout(t, func() {
		for i := range states {
			showSetting(&states[i])
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

	// Answering yes applies it anyway, with no second question.
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

// `configure gpu` runs the signing flow after the settings; with Secure Boot
// off it skips it and says why, rather than asking again.
func TestRunGPUSigning_SkipsWhenSecureBootOff(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GPU setup is Linux-only")
	}
	fakeBins(t, map[string]string{"mokutil": `echo "SecureBoot disabled"`})
	var err error
	out := plain(captureStdout(t, func() { err = runGPUSigning(context.Background(), false) }))
	if err != nil {
		t.Fatalf("runGPUSigning: %v", err)
	}
	if !strings.Contains(out, "Driver signing not needed: Secure Boot is off") || strings.Contains(out, "GPU Signing Setup") {
		t.Errorf("want the skip note and no signing setup; got %q", out)
	}
}
