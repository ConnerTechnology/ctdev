package setup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/gpu"
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

// uefi makes this test's "machine" boot through UEFI (or not), whatever the
// host running the test does.
func uefi(t *testing.T, booted bool) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "efi")
	if booted {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	gpu.SetEFIDirForTest(t, dir)
}

func TestNvidiaSigningNotNeeded(t *testing.T) {
	tests := []struct {
		name    string
		uefi    bool
		mokutil string // script body; "" means no mokutil on PATH
		needed  bool
	}{
		{"Secure Boot on", true, `echo "SecureBoot enabled"`, true},
		{"Secure Boot off", true, `echo "SecureBoot disabled"`, false},
		// mokutil writes these to stderr, with a non-zero exit.
		{"legacy BIOS, mokutil says no EFI variables", true, `echo "EFI variables are not supported on this system" >&2; exit 255`, false},
		{"firmware without Secure Boot", true, `echo "This system doesn't support Secure Boot" >&2; exit 255`, false},
		{"legacy BIOS, no /sys/firmware/efi", false, `echo "SecureBoot enabled"`, false},
		{"mokutil can't tell", true, `echo "Cannot determine secure boot state." >&2; exit 255`, true},
		{"mokutil missing", true, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uefi(t, tt.uefi)
			bins := map[string]string{}
			if tt.mokutil != "" {
				bins["mokutil"] = tt.mokutil
			}
			fakeBins(t, bins)
			got := NvidiaSigningNotNeeded(context.Background())
			if tt.needed && got != "" {
				t.Errorf("NvidiaSigningNotNeeded = %q, want needed (\"\")", got)
			}
			if !tt.needed && !strings.Contains(got, "Secure Boot is off") {
				t.Errorf("NvidiaSigningNotNeeded = %q, want a 'Secure Boot is off' reason", got)
			}
		})
	}
}

// TestGPUSettingsExplainThemselves pins the gpu category to CTD-90 decision 9:
// a plain name, a one-line why, on-only with the reason said, and a check
// whether this machine needs it.
func TestGPUSettingsExplainThemselves(t *testing.T) {
	want := []string{"NVIDIA driver works with Secure Boot", "NVIDIA GPU wakes from sleep cleanly"}
	gpuSettings := FilterBySlug(Registry, "gpu")
	if len(gpuSettings) != len(want) {
		t.Fatalf("gpu settings = %d, want %d", len(gpuSettings), len(want))
	}
	for i, s := range gpuSettings {
		if s.Name != want[i] {
			t.Errorf("gpu[%d].Name = %q, want %q", i, s.Name, want[i])
		}
		if s.Description == "" || strings.Contains(s.Description, "\n") {
			t.Errorf("%q: why must be one line, got %q", s.Name, s.Description)
		}
		if !s.OneWay {
			t.Errorf("%q: must stay OneWay (on-only)", s.Name)
		}
		if s.NoOffSwitch == "" {
			t.Errorf("%q: on-only setting must say why it has no off switch", s.Name)
		}
		if s.NotNeededFn == nil {
			t.Errorf("%q: must check whether this machine needs it", s.Name)
		}
	}
}

// TestGPUSettingsNotNeeded runs the registry's gpu settings through InitStates
// on a machine with Secure Boot off and suspend masked: both show as not
// needed and neither is applied, even with --force.
func TestGPUSettingsNotNeeded(t *testing.T) {
	uefi(t, true)
	fakeBins(t, map[string]string{
		"mokutil":   `echo "SecureBoot disabled"`,
		"systemctl": `echo masked; exit 1`,
	})
	gpuSettings := FilterBySlug(Registry, "gpu")
	for i := range gpuSettings {
		gpuSettings[i].DetectFunc = nil // no lsmod or modinfo here
	}
	for _, st := range InitStates(context.Background(), gpuSettings) {
		if st.NotNeeded == "" {
			t.Errorf("%q: want not needed", st.Setting.Name)
		}
		if st.NeedsApply(false) || st.NeedsApply(true) {
			t.Errorf("%q: a setting this machine doesn't need must not be applied", st.Setting.Name)
		}
	}
}
