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
	for _, reason := range []string{"Secure Boot is off", "suspend is turned off"} {
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
		if err := applySettings(ctx, states, false, false, false); err == nil || !strings.Contains(err.Error(), "NVIDIA DKMS module not found") {
			t.Errorf("applySettings = %v, want the real signing failure", err)
		}
	}))
	if n := strings.Count(out, "anyway"); n != 1 {
		t.Errorf("want one 'anyway' question, got %d; output %q", n, out)
	}
	if !strings.Contains(out, "NVIDIA DKMS module not found") {
		t.Errorf("want the real signing setup to run (and stop at DKMS); got %q", out)
	}
}

// The GPU wizard runs signing once and returns the outcome of that attempt.
func TestConfigureGPU_RunsSigningOnce(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GPU setup is Linux-only")
	}
	for _, tt := range []struct {
		name               string
		dkms, rebuildFails bool
		wantErr            []string
	}{
		{name: "no DKMS driver", wantErr: []string{"NVIDIA DKMS module not found"}},
		{name: "rebuild fails and there are no modules to sign", dkms: true, rebuildFails: true,
			wantErr: []string{"DKMS rebuild failed", "manual signing"}},
		{name: "rebuild succeeds", dkms: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			uefi(t)
			bins := map[string]string{
				"mokutil":   `echo 'SecureBoot enabled'`,
				"lsmod":     `echo 'nvidia 1 0'`,
				"modinfo":   `echo`,
				"systemctl": `echo masked; exit 1`,
				"uname":     `echo example-test-kernel-with-no-modules`,
				"sudo":      `exit 0`,
			}
			if tt.dkms {
				bins["dkms"] = `echo 'nvidia/123.45, example-kernel, x86_64: installed'`
			}
			if tt.rebuildFails {
				bins["sudo"] = `if [ "$1 $2" = 'dkms install' ]; then exit 1; fi`
			}
			fakeBins(t, bins)
			oldBatch := flagBatch
			flagBatch = true
			t.Cleanup(func() { flagBatch = oldBatch })
			var err error
			out := plain(captureStdout(t, func() { err = runConfigureGPU(configureGPUCmd, nil) }))
			if n := strings.Count(out, "GPU Signing Setup"); n != 1 {
				t.Errorf("signing setup ran %d times, want once; output %q", n, out)
			}
			if tt.rebuildFails && !strings.Contains(out, "falling back to manual signing") {
				t.Errorf("didn't attempt fallback signing: %q", out)
			}
			if len(tt.wantErr) == 0 {
				if err != nil || !strings.Contains(out, "DKMS rebuild complete") {
					t.Fatalf("want successful rebuild, got %v; output %q", err, out)
				}
			}
			for _, want := range tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("configure gpu err = %v; want it to contain %q", err, want)
				}
			}
		})
	}
}

// Every --show status label fits its column, so none wraps onto a second line.
func TestGPUShow_StatusLabelsFit(t *testing.T) {
	checks := []gpu.StatusCheck{
		{Name: "Secure Boot", Pass: true, Detail: "enabled"},
		{Name: "MOK key enrolled", Pass: true, Detail: "in firmware"},
		{Name: "Module signature", Pass: false, Detail: "signed with a different key"},
	}
	lines := strings.Split(ansi.Strip(captureStdout(t, func() { printSigningStatus(checks) })), "\n")
	if len(lines) != len(checks)+1 {
		t.Fatalf("want one line per check, got %q", lines)
	}
	for i, check := range checks {
		if !strings.Contains(lines[i], check.Name+":") || !strings.Contains(lines[i], check.Detail) {
			t.Errorf("line %d = %q, want %q and its detail on it", i, lines[i], check.Name+":")
		}
	}
}
