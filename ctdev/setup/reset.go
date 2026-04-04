package setup

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// removeGrubCmdlineParam removes a kernel cmdline parameter from /etc/default/grub
// by stripping " <param>" from GRUB_CMDLINE_LINUX lines.
func removeGrubCmdlineParam(param string) error {
	return sudoRun("sed", "-i", fmt.Sprintf("s/ %s//", param), "/etc/default/grub")
}

// nvidiaLoaded returns true if the nvidia kernel module is currently loaded.
func nvidiaLoaded() bool {
	out, err := exec.Command("lsmod").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "nvidia ") {
			return true
		}
	}
	return false
}

// ResetLinuxDefaults resets Linux Mint system settings back to their defaults.
func ResetLinuxDefaults(w io.Writer, dryRun bool) error {
	hasNvidia := nvidiaLoaded()

	if dryRun {
		fmt.Fprintln(w, "[DRY-RUN] Would reset Power, Screensaver, Keyboard, Mouse, Sound, Nemo settings")
		fmt.Fprintln(w, "[DRY-RUN] Would reset GRUB settings (timeout=10, menu, os-prober enabled)")
		if hasNvidia {
			fmt.Fprintln(w, "[DRY-RUN] Would reset NVIDIA suspend settings (GRUB parameters, systemd services)")
		}
		fmt.Fprintln(w, "[DRY-RUN] Would disable fstrim.timer")
		fmt.Fprintln(w, "[DRY-RUN] Would remove Logitech KVM mouse fix (udev rule + systemd service)")
		fmt.Fprintln(w, "[DRY-RUN] Would remove hide-drives udev rule")
		fmt.Fprintln(w, "[DRY-RUN] Would reset xbindkeys (stop, remove autostart and config symlink)")
		fmt.Fprintln(w, "[DRY-RUN] Would remove WirePlumber LDAC config")
		return nil
	}

	// Power settings
	fmt.Fprintf(w, "Resetting Power settings...\n")
	if err := run("powerprofilesctl", "set", "balanced"); err != nil {
		return fmt.Errorf("powerprofilesctl: %w", err)
	}
	for _, key := range []string{
		"/org/cinnamon/settings-daemon/plugins/power/sleep-display-ac",
		"/org/cinnamon/settings-daemon/plugins/power/sleep-inactive-ac-timeout",
		"/org/cinnamon/settings-daemon/plugins/power/lock-on-suspend",
	} {
		if err := run("dconf", "reset", key); err != nil {
			return fmt.Errorf("dconf reset %s: %w", key, err)
		}
	}

	// Screensaver settings
	fmt.Fprintf(w, "Resetting Screensaver settings...\n")
	for _, key := range []string{
		"/org/cinnamon/desktop/session/idle-delay",
		"/org/cinnamon/desktop/screensaver/lock-enabled",
		"/org/cinnamon/desktop/screensaver/lock-delay",
	} {
		if err := run("dconf", "reset", key); err != nil {
			return fmt.Errorf("dconf reset %s: %w", key, err)
		}
	}

	// Keyboard settings
	fmt.Fprintf(w, "Resetting Keyboard settings...\n")
	schema := "org.cinnamon.desktop.peripherals.keyboard"
	for _, key := range []string{"repeat", "delay", "repeat-interval", "numlock-state"} {
		if err := run("gsettings", "reset", schema, key); err != nil {
			return fmt.Errorf("gsettings reset %s %s: %w", schema, key, err)
		}
	}
	if err := run("xset", "r", "rate"); err != nil {
		return fmt.Errorf("xset r rate: %w", err)
	}

	// Mouse settings
	fmt.Fprintf(w, "Resetting Mouse settings...\n")
	for _, key := range []string{
		"/org/gnome/desktop/peripherals/mouse/accel-profile",
		"/org/gnome/desktop/peripherals/mouse/speed",
		"/org/gnome/desktop/peripherals/mouse/natural-scroll",
	} {
		if err := run("dconf", "reset", key); err != nil {
			return fmt.Errorf("dconf reset %s: %w", key, err)
		}
	}

	// Sound settings
	fmt.Fprintf(w, "Resetting Sound settings...\n")
	if err := run("dconf", "reset", "/org/cinnamon/desktop/sound/event-sounds"); err != nil {
		return fmt.Errorf("dconf reset event-sounds: %w", err)
	}

	// Nemo settings
	fmt.Fprintf(w, "Resetting Nemo settings...\n")
	if err := run("dconf", "reset", "/org/nemo/preferences/default-folder-viewer"); err != nil {
		return fmt.Errorf("dconf reset nemo viewer: %w", err)
	}

	// WirePlumber LDAC config
	fmt.Fprintf(w, "Removing WirePlumber LDAC config...\n")
	ldacConf := "/etc/wireplumber/wireplumber.conf.d/51-ldac-hq.conf"
	if _, err := os.Stat(ldacConf); err == nil {
		if err := sudoRun("rm", ldacConf); err != nil {
			return fmt.Errorf("remove wireplumber ldac config: %w", err)
		}
		// Best-effort restart of PipeWire stack.
		_ = exec.Command("systemctl", "--user", "restart",
			"pipewire", "pipewire-pulse", "wireplumber").Run()
	}

	// xbindkeys
	fmt.Fprintf(w, "Resetting xbindkeys...\n")
	_ = exec.Command("killall", "xbindkeys").Run()
	home, _ := os.UserHomeDir()
	_ = os.Remove(home + "/.config/autostart/xbindkeys.desktop")
	info, err := os.Lstat(home + "/.xbindkeysrc")
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		_ = os.Remove(home + "/.xbindkeysrc")
	}

	// GRUB settings
	fmt.Fprintf(w, "Resetting GRUB settings...\n")
	for varName, value := range map[string]string{
		"GRUB_TIMEOUT_STYLE":    "menu",
		"GRUB_TIMEOUT":          "10",
		"GRUB_DISABLE_OS_PROBER": "false",
	} {
		if err := applyGrubVar(varName, value); err != nil {
			return fmt.Errorf("applyGrubVar %s: %w", varName, err)
		}
	}

	// Fix linuxmint grub override file if it forces os-prober off.
	mintGrubCfg := "/etc/default/grub.d/50_linuxmint.cfg"
	if data, err := os.ReadFile(mintGrubCfg); err == nil {
		if bytes.Contains(data, []byte("GRUB_DISABLE_OS_PROBER=true")) {
			if err := sudoRun("sed", "-i",
				"s/GRUB_DISABLE_OS_PROBER=true/GRUB_DISABLE_OS_PROBER=false/",
				mintGrubCfg); err != nil {
				return fmt.Errorf("fix linuxmint grub cfg: %w", err)
			}
		}
	}

	// NVIDIA suspend settings
	if hasNvidia {
		fmt.Fprintf(w, "Resetting NVIDIA suspend settings...\n")
		for _, param := range []string{
			"nvidia.NVreg_PreserveVideoMemoryAllocations=1",
			"nvidia.NVreg_EnableS0ixPowerManagement=0",
			"pcie_aspm=off",
		} {
			if err := removeGrubCmdlineParam(param); err != nil {
				return fmt.Errorf("remove grub cmdline param %s: %w", param, err)
			}
		}
		for _, svc := range []string{
			"nvidia-suspend.service",
			"nvidia-resume.service",
			"nvidia-hibernate.service",
			"nvidia-persistenced.service",
		} {
			// Best-effort: ignore errors for services that may not exist.
			_ = exec.Command("sudo", "systemctl", "disable", svc).Run()
		}
	}

	// Logitech KVM fix
	kvmRule := "/etc/udev/rules.d/99-logitech-kvm-fix.rules"
	if _, err := os.Stat(kvmRule); err == nil {
		fmt.Fprintf(w, "Removing Logitech KVM mouse fix...\n")
		if err := sudoRun("rm", "-f", kvmRule); err != nil {
			return fmt.Errorf("remove logitech kvm udev rule: %w", err)
		}
	}
	kvmService := home + "/.config/systemd/user/solaar-restart.service"
	if _, err := os.Stat(kvmService); err == nil {
		_ = exec.Command("systemctl", "--user", "disable", "solaar-restart.service").Run()
		_ = os.Remove(kvmService)
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	}

	// Hide drives udev rule
	hideDrives := "/etc/udev/rules.d/99-hide-drives.rules"
	if _, err := os.Stat(hideDrives); err == nil {
		fmt.Fprintf(w, "Removing hide-drives udev rule...\n")
		if err := sudoRun("rm", "-f", hideDrives); err != nil {
			return fmt.Errorf("remove hide-drives udev rule: %w", err)
		}
	}

	// Reload udev after removing rules
	_ = exec.Command("sudo", "udevadm", "control", "--reload-rules").Run()

	// WiFi suspend hook
	sleepHook := "/usr/lib/systemd/system-sleep/wifi-mt7925"
	if _, err := os.Stat(sleepHook); err == nil {
		fmt.Fprintf(w, "Removing WiFi suspend fix...\n")
		if err := sudoRun("rm", "-f", sleepHook); err != nil {
			return fmt.Errorf("remove wifi suspend hook: %w", err)
		}
	}

	// fstrim
	fmt.Fprintf(w, "Resetting fstrim...\n")
	_ = exec.Command("sudo", "systemctl", "stop", "fstrim.timer").Run()
	_ = exec.Command("sudo", "systemctl", "disable", "fstrim.timer").Run()

	// Regenerate GRUB config
	if err := applyUpdateGrub(); err != nil {
		return fmt.Errorf("update-grub: %w", err)
	}

	fmt.Fprintln(w, "Linux Mint defaults reset to system defaults")
	fmt.Fprintln(w, "Some settings may require logout/restart to take full effect")
	return nil
}
