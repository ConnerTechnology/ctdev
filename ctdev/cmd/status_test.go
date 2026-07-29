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

// Memory/disk/uptime rendering moved to tui/info and platform; their tests
// live alongside them now.

func TestParseDiskPressure(t *testing.T) {
	// Capacity column drives the decision; 84% must stay quiet, 85% must not.
	df := "Filesystem 1024-blocks Used Available Capacity Mounted on\n" +
		"/dev/sda1  1917499848 1800000000  117499848      94% /\n" +
		"/dev/sdb1  1917499848  100000000 1817499848       6% /data\n" +
		"/dev/sdc1   500000000  420000000   80000000      84% /var\n"
	got := parseDiskPressure(df)
	if len(got) != 1 {
		t.Fatalf("got %d over-threshold mounts, want 1: %v", len(got), got)
	}
	if !strings.Contains(got[0], "/ at 94%") || !strings.Contains(got[0], "free") {
		t.Errorf("line = %q, want the mount, percent and free space", got[0])
	}
	if len(parseDiskPressure("garbage")) != 0 {
		t.Error("unparseable df output should yield no warnings")
	}
}
