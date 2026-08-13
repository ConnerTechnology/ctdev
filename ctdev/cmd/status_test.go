package cmd

import (
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
