package setup

import "testing"

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
