package setup

import (
	"os"
	"os/exec"
	"path/filepath"
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

// sysBlockDir is where the kernel lists block devices; tests point it at a
// fake tree.
var sysBlockDir = "/sys/block"

// gateSSD reports whether this Linux host has an SSD that TRIM can help.
func gateSSD() bool { return gateLinux() && hasSSD() }

// hasSSD reports whether any block device under sysBlockDir is a fixed,
// non-rotational disk that accepts discards. Loop, zram and device-mapper
// devices have no device link and are skipped; removable flash is skipped
// because fstrim.timer is about the machine's own disks. Reads sysfs files
// only, so it stays cheap enough for a gate.
func hasSSD() bool {
	entries, err := os.ReadDir(sysBlockDir)
	if err != nil {
		return false
	}
	read := func(dev, rel string) string {
		b, err := os.ReadFile(filepath.Join(sysBlockDir, dev, rel))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	for _, e := range entries {
		dev := e.Name()
		if _, err := os.Stat(filepath.Join(sysBlockDir, dev, "device")); err != nil {
			continue
		}
		if read(dev, "queue/rotational") != "0" || read(dev, "removable") == "1" {
			continue
		}
		if d := read(dev, "queue/discard_max_bytes"); d == "" || d == "0" {
			continue
		}
		return true
	}
	return false
}
