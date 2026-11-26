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

func TestDetectGrubOSProber(t *testing.T) {
	// GRUB_DISABLE_OS_PROBER=false means os-prober is enabled; true or omitted
	// means disabled; a missing grub file reports unknown.
	orig := grubDefaultPath
	t.Cleanup(func() { grubDefaultPath = orig })

	dir := t.TempDir()
	tests := []struct {
		name    string
		content string
		missing bool
		want    string
	}{
		{"false means enabled", "GRUB_DISABLE_OS_PROBER=false\n", false, "enabled"},
		{"quoted false means enabled", "GRUB_DISABLE_OS_PROBER=\"false\"\n", false, "enabled"},
		{"true means disabled", "GRUB_DISABLE_OS_PROBER=true\n", false, "disabled"},
		{"omitted means disabled", "GRUB_TIMEOUT=5\n", false, "disabled"},
		{"commented false means disabled", "# GRUB_DISABLE_OS_PROBER=false\nGRUB_TIMEOUT=5\n", false, "disabled"},
		{"missing file reports unknown", "", true, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name)
			if !tt.missing {
				if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			grubDefaultPath = path
			if got := detectGrubOSProber(t.Context()); got != tt.want {
				t.Errorf("detectGrubOSProber() = %q, want %q", got, tt.want)
			}
		})
	}
}
