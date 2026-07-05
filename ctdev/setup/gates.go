package setup

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Gates are visibility predicates for Setting.HardwareFn. FilterByHardware
// runs every gate on every wizard/--show/diff pass, so they must stay cheap:
// env vars, os.Stat, and PATH lookups only — no command execution.

// gateLinux reports whether this host runs Linux.
func gateLinux() bool { return runtime.GOOS == "linux" }

// gateMacOS reports whether this host runs macOS.
func gateMacOS() bool { return runtime.GOOS == "darwin" }

// gateGrub reports whether this Linux host boots via GRUB (Raspberry Pi
// firmware boot, for example, has no /etc/default/grub).
func gateGrub() bool {
	if !gateLinux() {
		return false
	}
	_, err := os.Stat("/etc/default/grub")
	return err == nil
}

// gateDesktop reports whether a graphical session is plausible on this Linux
// host (a headless node exports none of these).
func gateDesktop() bool {
	if !gateLinux() {
		return false
	}
	return os.Getenv("XDG_CURRENT_DESKTOP") != "" ||
		os.Getenv("DISPLAY") != "" ||
		os.Getenv("WAYLAND_DISPLAY") != ""
}

// gateCinnamon reports whether the current desktop session is Cinnamon
// (XDG_CURRENT_DESKTOP is "X-Cinnamon" on Mint; matched case-insensitively so
// /org/cinnamon and /org/nemo dconf keys aren't written inertly on GNOME etc.).
func gateCinnamon() bool {
	if !gateDesktop() {
		return false
	}
	return strings.Contains(strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP")), "cinnamon")
}

// gateDconfDesktop reports whether a desktop session with the dconf tool is
// present (for desktop-agnostic /org/gnome/desktop/peripherals keys).
func gateDconfDesktop() bool {
	if !gateDesktop() {
		return false
	}
	_, err := exec.LookPath("dconf")
	return err == nil
}

// gateNetworkManager reports whether NetworkManager manages this Linux host.
func gateNetworkManager() bool {
	if !gateLinux() {
		return false
	}
	_, err := os.Stat("/etc/NetworkManager")
	return err == nil
}

// gatePowerProfiles reports whether power-profiles-daemon's CLI is available
// on this Linux host (absent on stock Raspberry Pi OS and most servers).
func gatePowerProfiles() bool {
	if !gateLinux() {
		return false
	}
	_, err := exec.LookPath("powerprofilesctl")
	return err == nil
}

// allOf composes gates: the returned gate passes only when every fn passes.
func allOf(fns ...func() bool) func() bool {
	return func() bool {
		for _, fn := range fns {
			if !fn() {
				return false
			}
		}
		return true
	}
}
