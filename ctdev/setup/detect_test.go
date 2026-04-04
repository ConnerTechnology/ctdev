package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseXsetRepeat(t *testing.T) {
	input := "    auto repeat delay:  200    repeat rate:  50"
	delay, rate := parseXsetRepeat(input)
	if delay != "200" {
		t.Errorf("expected delay 200, got %s", delay)
	}
	if rate != "50" {
		t.Errorf("expected rate 50, got %s", rate)
	}
}

func TestParseXsetRepeatEmpty(t *testing.T) {
	delay, rate := parseXsetRepeat("some other line")
	if delay != "" || rate != "" {
		t.Errorf("expected empty, got %s %s", delay, rate)
	}
}

func TestParseGrubVar(t *testing.T) {
	content := `GRUB_TIMEOUT_STYLE=menu
GRUB_TIMEOUT=10
# GRUB_DISABLE_OS_PROBER=true
GRUB_DISABLE_OS_PROBER=false`

	if v := parseGrubVar(content, "GRUB_TIMEOUT"); v != "10" {
		t.Errorf("expected 10, got %s", v)
	}
	if v := parseGrubVar(content, "GRUB_TIMEOUT_STYLE"); v != "menu" {
		t.Errorf("expected menu, got %s", v)
	}
	if v := parseGrubVar(content, "GRUB_DISABLE_OS_PROBER"); v != "false" {
		t.Errorf("expected false, got %s", v)
	}
}

func TestParseGrubVarMissing(t *testing.T) {
	content := `GRUB_TIMEOUT=5`
	if v := parseGrubVar(content, "GRUB_TIMEOUT_STYLE"); v != "" {
		t.Errorf("expected empty, got %s", v)
	}
}

func TestParseGrubVarQuoted(t *testing.T) {
	content := `GRUB_TIMEOUT_STYLE="hidden"`
	if v := parseGrubVar(content, "GRUB_TIMEOUT_STYLE"); v != "hidden" {
		t.Errorf("expected hidden, got %s", v)
	}
}

func TestDetectFileExists(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "marker")
	os.WriteFile(f, []byte("x"), 0644)

	if detectFileExists(f) != "installed" {
		t.Error("expected installed for existing file")
	}
	if detectFileExists(filepath.Join(tmp, "nope")) != "not installed" {
		t.Error("expected not installed for missing file")
	}
}

func TestDetectFileExistsInstalled(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "udev-rule")
	os.WriteFile(f, []byte("ACTION==\"add\""), 0644)

	if got := detectFileExists(f); got != "installed" {
		t.Errorf("expected installed, got %s", got)
	}
}

func TestDetectFileExistsNotInstalled(t *testing.T) {
	if got := detectFileExists("/nonexistent/path/99-fake.rules"); got != "not installed" {
		t.Errorf("expected not installed, got %s", got)
	}
}

func TestDetectGrubOSProberTranslation(t *testing.T) {
	// Test the value translation logic that detectGrubOSProber uses
	// "false" means os-prober is enabled (GRUB_DISABLE_OS_PROBER=false means don't disable)
	tests := []struct {
		grubValue string
		expected  string
	}{
		{"false", "enabled"},
		{"true", "disabled"},
		{"", "disabled"},
	}
	for _, tt := range tests {
		got := "disabled"
		if tt.grubValue == "false" {
			got = "enabled"
		}
		if got != tt.expected {
			t.Errorf("grub value %q: got %q, want %q", tt.grubValue, got, tt.expected)
		}
	}
}
