package diagnose

import (
	"errors"
	"os/exec"
	"testing"
)

// macOS ships sshd_config whether or not Remote Login is on, which made every
// Mac with it off report password logins. Only a definite answer may skip.
func TestSystemdState(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want serviceState
	}{
		{"debian service up", "active\ninactive\ninactive\ninactive", serviceRunning},
		{"ubuntu socket activation", "inactive\ninactive\nactive\ninactive", serviceRunning},
		{"everything down", "inactive\ninactive\ninactive\ninactive", serviceStopped},
		{"down after a crash", "failed\ninactive\ninactive\ninactive", serviceStopped},
		{"old systemd, unit not installed", "unknown\ninactive\ninactive\ninactive", serviceUnknown},
		{"no systemd answer", "", serviceUnknown},
		{"systemd offline in a container", "offline\noffline\noffline\noffline", serviceUnknown},
	}
	for _, tt := range tests {
		if got := systemdState(tt.out, 4); got != tt.want {
			t.Errorf("%s: systemdState = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestLaunchdState(t *testing.T) {
	tests := []struct {
		name string
		code int
		want serviceState
	}{
		{"job loaded", 0, serviceRunning},
		{"could not find service", launchdNoSuchService, serviceStopped},
		{"other failure", 1, serviceUnknown},
		{"launchctl never ran", -1, serviceUnknown},
	}
	for _, tt := range tests {
		if got := launchdState(tt.code); got != tt.want {
			t.Errorf("%s: launchdState = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestExitCode(t *testing.T) {
	if got := exitCode(nil); got != 0 {
		t.Errorf("success: exitCode = %d, want 0", got)
	}
	if got := exitCode(exec.ErrNotFound); got != -1 {
		t.Errorf("never ran: exitCode = %d, want -1", got)
	}
	if got := exitCode(errors.New("boom")); got != -1 {
		t.Errorf("plain error: exitCode = %d, want -1", got)
	}
}
