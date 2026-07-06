package cmd

import (
	"strings"
	"testing"
	"time"
)

func TestHumanAge(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Minute, "30m"},
		{time.Hour, "1h"},
		{11 * time.Hour, "11h"},
		{47 * time.Hour, "47h"},
		{48 * time.Hour, "2d"},
		{76 * time.Hour, "3d 4h"},
	}
	for _, tt := range tests {
		if got := humanAge(tt.d); got != tt.want {
			t.Errorf("humanAge(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestParseDFLine(t *testing.T) {
	df := "Filesystem 1024-blocks Used Available Capacity Mounted on\n" +
		"/dev/mapper/vgmint-root  1917499848 405340248 1414682176      23% /\n"
	if got := parseDFLine(df); got != "387G/1.8T (23%)" {
		t.Errorf("parseDFLine = %q, want %q", got, "387G/1.8T (23%)")
	}
	if got := parseDFLine("garbage"); got != "" {
		t.Errorf("parseDFLine(garbage) = %q, want empty", got)
	}
}

func TestParseMemUsage(t *testing.T) {
	// ~63 GiB total, ~55.8 GiB available → ~9.5G used, 15%.
	meminfo := "MemTotal:       65805020 kB\nMemFree:        40000000 kB\nMemAvailable:   55805020 kB\nBuffers:          100 kB\n"
	got := parseMemUsage(meminfo)
	if !strings.Contains(got, "/63G") || !strings.HasSuffix(got, "(15%)") {
		t.Errorf("parseMemUsage = %q, want ~9.5G/63G (15%%)", got)
	}
	if parseMemUsage("garbage") != "" {
		t.Error("parseMemUsage(garbage) should be empty")
	}
}

func TestHumanKB(t *testing.T) {
	tests := []struct {
		kb   int64
		want string
	}{
		{1024 * 1024, "1.0G"},            // 1 GiB
		{11 * 1024 * 1024, "11G"},        // ≥10G drops the decimal
		{2 * 1024 * 1024 * 1024, "2.0T"}, // TiB range
	}
	for _, tt := range tests {
		if got := humanKB(tt.kb); got != tt.want {
			t.Errorf("humanKB(%d) = %q, want %q", tt.kb, got, tt.want)
		}
	}
}
