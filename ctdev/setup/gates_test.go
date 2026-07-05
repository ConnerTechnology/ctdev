package setup

import (
	"runtime"
	"testing"
)

func TestAllOf(t *testing.T) {
	yes := func() bool { return true }
	no := func() bool { return false }

	tests := []struct {
		name string
		fns  []func() bool
		want bool
	}{
		{"empty passes", nil, true},
		{"all true", []func() bool{yes, yes}, true},
		{"one false", []func() bool{yes, no, yes}, false},
		{"all false", []func() bool{no, no}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allOf(tt.fns...)(); got != tt.want {
				t.Errorf("allOf() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllOfShortCircuits(t *testing.T) {
	called := false
	no := func() bool { return false }
	spy := func() bool { called = true; return true }
	if allOf(no, spy)() {
		t.Error("expected composed gate to fail")
	}
	if called {
		t.Error("expected allOf to stop at the first failing gate")
	}
}

func TestGateOSPredicates(t *testing.T) {
	if gateLinux() != (runtime.GOOS == "linux") {
		t.Errorf("gateLinux() = %v on GOOS=%s", gateLinux(), runtime.GOOS)
	}
	if gateMacOS() != (runtime.GOOS == "darwin") {
		t.Errorf("gateMacOS() = %v on GOOS=%s", gateMacOS(), runtime.GOOS)
	}
}

// clearSessionEnv blanks the graphical-session env vars so gateDesktop sees a
// headless environment until re-set by the test.
func clearSessionEnv(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
}

func TestGateDesktop(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("gateDesktop is always false off Linux")
	}

	clearSessionEnv(t)
	if gateDesktop() {
		t.Error("expected gateDesktop=false with no session env vars")
	}

	t.Setenv("DISPLAY", ":0")
	if !gateDesktop() {
		t.Error("expected gateDesktop=true with DISPLAY set")
	}

	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	if !gateDesktop() {
		t.Error("expected gateDesktop=true with WAYLAND_DISPLAY set")
	}
}

func TestGateCinnamon(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("gateCinnamon is always false off Linux")
	}

	tests := []struct {
		desktop string
		want    bool
	}{
		{"X-Cinnamon", true},
		{"Cinnamon", true},
		{"x-cinnamon", true},
		{"GNOME", false},
		{"ubuntu:GNOME", false},
		{"KDE", false},
	}
	for _, tt := range tests {
		t.Run(tt.desktop, func(t *testing.T) {
			clearSessionEnv(t)
			t.Setenv("XDG_CURRENT_DESKTOP", tt.desktop)
			if got := gateCinnamon(); got != tt.want {
				t.Errorf("gateCinnamon() with XDG_CURRENT_DESKTOP=%q = %v, want %v", tt.desktop, got, tt.want)
			}
		})
	}

	t.Run("headless", func(t *testing.T) {
		clearSessionEnv(t)
		if gateCinnamon() {
			t.Error("expected gateCinnamon=false with no session env vars")
		}
	})
}

func TestGateDconfDesktopRequiresDesktop(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("gateDconfDesktop is always false off Linux")
	}
	clearSessionEnv(t)
	if gateDconfDesktop() {
		t.Error("expected gateDconfDesktop=false in a headless environment")
	}
}
