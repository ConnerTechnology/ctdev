package setup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestNvidiaSigningNotNeeded(t *testing.T) {
	t.Run("Secure Boot off", func(t *testing.T) {
		fakeBins(t, map[string]string{"mokutil": `echo "SecureBoot disabled"`})
		if got := NvidiaSigningNotNeeded(context.Background()); !strings.Contains(got, "Secure Boot is off") {
			t.Errorf("NvidiaSigningNotNeeded = %q, want a 'Secure Boot is off' reason", got)
		}
	})
	t.Run("Secure Boot on", func(t *testing.T) {
		fakeBins(t, map[string]string{"mokutil": `echo "SecureBoot enabled"`})
		if got := NvidiaSigningNotNeeded(context.Background()); got != "" {
			t.Errorf("NvidiaSigningNotNeeded = %q, want needed (\"\")", got)
		}
	})
	t.Run("Secure Boot state unknown", func(t *testing.T) {
		fakeBins(t, nil) // no mokutil
		if got := NvidiaSigningNotNeeded(context.Background()); got != "" {
			t.Errorf("NvidiaSigningNotNeeded = %q, want needed (\"\") when mokutil can't tell", got)
		}
	})
}

func TestNvidiaSuspendNotNeeded(t *testing.T) {
	t.Run("suspend masked", func(t *testing.T) {
		// systemctl is-enabled prints "masked" and exits non-zero for a masked unit.
		fakeBins(t, map[string]string{"systemctl": `echo masked; exit 1`})
		if got := nvidiaSuspendNotNeeded(context.Background()); !strings.Contains(got, "never suspends") {
			t.Errorf("nvidiaSuspendNotNeeded = %q, want a 'never suspends' reason", got)
		}
	})
	t.Run("suspend allowed", func(t *testing.T) {
		fakeBins(t, map[string]string{"systemctl": `echo static`})
		if got := nvidiaSuspendNotNeeded(context.Background()); got != "" {
			t.Errorf("nvidiaSuspendNotNeeded = %q, want needed (\"\")", got)
		}
	})
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
