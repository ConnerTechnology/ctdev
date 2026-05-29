package setup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/user"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// removeGrubCmdlineParam removes a kernel cmdline parameter from /etc/default/grub
// by stripping " <param>" from GRUB_CMDLINE_LINUX lines.
func removeGrubCmdlineParam(ctx context.Context, o sysutil.Opts, param string) error {
	return sysutil.SudoRun(ctx, o, "sed", "-i", fmt.Sprintf("s| %s||", sedEscape(param)), "/etc/default/grub")
}

// ResetLinuxDefaults resets Linux Mint system settings back to their defaults.
// Honors o.DryRun (prints the plan) and o.Stdout (log destination).
func ResetLinuxDefaults(ctx context.Context, o sysutil.Opts) error {
	w := o.Stdout
	hasNvidia := detectNvidiaLoaded()

	if o.DryRun {
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
		fmt.Fprintln(w, "[DRY-RUN] Would reset remote access (unmask sleep targets, remove sshd/NM drop-ins, disable UFW + linger, uninstall VS Code tunnel)")
		return nil
	}

	// Power settings
	fmt.Fprintf(w, "Resetting Power settings...\n")
	if err := sysutil.Run(ctx, o, "powerprofilesctl", "set", "balanced"); err != nil {
		return fmt.Errorf("powerprofilesctl: %w", err)
	}
	for _, key := range []string{
		"/org/cinnamon/settings-daemon/plugins/power/sleep-display-ac",
		"/org/cinnamon/settings-daemon/plugins/power/sleep-inactive-ac-timeout",
		"/org/cinnamon/settings-daemon/plugins/power/lock-on-suspend",
	} {
		if err := sysutil.Run(ctx, o, "dconf", "reset", key); err != nil {
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
		if err := sysutil.Run(ctx, o, "dconf", "reset", key); err != nil {
			return fmt.Errorf("dconf reset %s: %w", key, err)
		}
	}

	// Keyboard settings
	fmt.Fprintf(w, "Resetting Keyboard settings...\n")
	schema := "org.cinnamon.desktop.peripherals.keyboard"
	for _, key := range []string{"repeat", "delay", "repeat-interval", "numlock-state"} {
		if err := sysutil.Run(ctx, o, "gsettings", "reset", schema, key); err != nil {
			return fmt.Errorf("gsettings reset %s %s: %w", schema, key, err)
		}
	}
	if err := sysutil.Run(ctx, o, "xset", "r", "rate"); err != nil {
		return fmt.Errorf("xset r rate: %w", err)
	}

	// Mouse settings
	fmt.Fprintf(w, "Resetting Mouse settings...\n")
	for _, key := range []string{
		"/org/gnome/desktop/peripherals/mouse/accel-profile",
		"/org/gnome/desktop/peripherals/mouse/speed",
		"/org/gnome/desktop/peripherals/mouse/natural-scroll",
	} {
		if err := sysutil.Run(ctx, o, "dconf", "reset", key); err != nil {
			return fmt.Errorf("dconf reset %s: %w", key, err)
		}
	}

	// Sound settings
	fmt.Fprintf(w, "Resetting Sound settings...\n")
	if err := sysutil.Run(ctx, o, "dconf", "reset", "/org/cinnamon/desktop/sound/event-sounds"); err != nil {
		return fmt.Errorf("dconf reset event-sounds: %w", err)
	}

	// Nemo settings
	fmt.Fprintf(w, "Resetting Nemo settings...\n")
	if err := sysutil.Run(ctx, o, "dconf", "reset", "/org/nemo/preferences/default-folder-viewer"); err != nil {
		return fmt.Errorf("dconf reset nemo viewer: %w", err)
	}

	// WirePlumber LDAC config
	fmt.Fprintf(w, "Removing WirePlumber LDAC config...\n")
	ldacConf := "/etc/wireplumber/wireplumber.conf.d/51-ldac-hq.conf"
	if _, err := os.Stat(ldacConf); err == nil {
		if err := sysutil.SudoRun(ctx, o, "rm", ldacConf); err != nil {
			return fmt.Errorf("remove wireplumber ldac config: %w", err)
		}
		// Best-effort restart of PipeWire stack.
		_ = sysutil.Run(ctx, o, "systemctl", "--user", "restart",
			"pipewire", "pipewire-pulse", "wireplumber")
	}

	// xbindkeys
	fmt.Fprintf(w, "Resetting xbindkeys...\n")
	_ = sysutil.Run(ctx, o, "killall", "xbindkeys")
	home, _ := os.UserHomeDir()
	_ = os.Remove(home + "/.config/autostart/xbindkeys.desktop")
	info, err := os.Lstat(home + "/.xbindkeysrc")
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		_ = os.Remove(home + "/.xbindkeysrc")
	}

	// GRUB settings
	fmt.Fprintf(w, "Resetting GRUB settings...\n")
	for varName, value := range map[string]string{
		"GRUB_TIMEOUT_STYLE":     "menu",
		"GRUB_TIMEOUT":           "10",
		"GRUB_DISABLE_OS_PROBER": "false",
	} {
		if err := applyGrubVar(ctx, o, varName, value); err != nil {
			return fmt.Errorf("applyGrubVar %s: %w", varName, err)
		}
	}

	// Fix linuxmint grub override file if it forces os-prober off.
	mintGrubCfg := "/etc/default/grub.d/50_linuxmint.cfg"
	if data, err := os.ReadFile(mintGrubCfg); err == nil {
		if bytes.Contains(data, []byte("GRUB_DISABLE_OS_PROBER=true")) {
			if err := sysutil.SudoRun(ctx, o, "sed", "-i",
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
			if err := removeGrubCmdlineParam(ctx, o, param); err != nil {
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
			_ = sysutil.SudoRun(ctx, o, "systemctl", "disable", svc)
		}
	}

	// Logitech KVM fix
	kvmRule := "/etc/udev/rules.d/99-logitech-kvm-fix.rules"
	if _, err := os.Stat(kvmRule); err == nil {
		fmt.Fprintf(w, "Removing Logitech KVM mouse fix...\n")
		if err := sysutil.SudoRun(ctx, o, "rm", "-f", kvmRule); err != nil {
			return fmt.Errorf("remove logitech kvm udev rule: %w", err)
		}
	}
	kvmService := home + "/.config/systemd/user/solaar-restart.service"
	if _, err := os.Stat(kvmService); err == nil {
		_ = sysutil.Run(ctx, o, "systemctl", "--user", "disable", "solaar-restart.service")
		_ = os.Remove(kvmService)
		_ = sysutil.Run(ctx, o, "systemctl", "--user", "daemon-reload")
	}

	// Hide drives udev rule
	hideDrives := "/etc/udev/rules.d/99-hide-drives.rules"
	if _, err := os.Stat(hideDrives); err == nil {
		fmt.Fprintf(w, "Removing hide-drives udev rule...\n")
		if err := sysutil.SudoRun(ctx, o, "rm", "-f", hideDrives); err != nil {
			return fmt.Errorf("remove hide-drives udev rule: %w", err)
		}
	}

	// Reload udev after removing rules
	_ = sysutil.SudoRun(ctx, o, "udevadm", "control", "--reload-rules")

	// WiFi suspend hook
	sleepHook := "/usr/lib/systemd/system-sleep/wifi-mt7925"
	if _, err := os.Stat(sleepHook); err == nil {
		fmt.Fprintf(w, "Removing WiFi suspend fix...\n")
		if err := sysutil.SudoRun(ctx, o, "rm", "-f", sleepHook); err != nil {
			return fmt.Errorf("remove wifi suspend hook: %w", err)
		}
	}

	// fstrim
	fmt.Fprintf(w, "Resetting fstrim...\n")
	_ = sysutil.SudoRun(ctx, o, "systemctl", "stop", "fstrim.timer")
	_ = sysutil.SudoRun(ctx, o, "systemctl", "disable", "fstrim.timer")

	// Remote access settings (best-effort — all reversible tweaks)
	fmt.Fprintf(w, "Resetting remote access settings...\n")
	_ = sysutil.SudoRun(ctx, o, "systemctl", append([]string{"unmask"}, sleepTargets...)...)
	if _, err := os.Stat(sshdDropIn); err == nil {
		_ = sysutil.SudoRun(ctx, o, "rm", "-f", sshdDropIn)
		_ = sysutil.SudoRun(ctx, o, "systemctl", "reload", "ssh")
	}
	nmConf := "/etc/NetworkManager/conf.d/wifi-powersave-off.conf"
	if _, err := os.Stat(nmConf); err == nil {
		_ = sysutil.SudoRun(ctx, o, "rm", "-f", nmConf)
	}
	if sysutil.CommandExists("ufw") {
		_ = sysutil.SudoRun(ctx, o, "ufw", "--force", "disable")
	}
	if u, err := user.Current(); err == nil {
		_ = sysutil.SudoRun(ctx, o, "loginctl", "disable-linger", u.Username)
	}
	if sysutil.CommandExists("code") {
		_ = sysutil.Run(ctx, o, "code", "tunnel", "service", "uninstall")
	}

	// Regenerate GRUB config
	if err := applyUpdateGrub(ctx, o); err != nil {
		return fmt.Errorf("update-grub: %w", err)
	}

	fmt.Fprintln(w, "Linux Mint defaults reset to system defaults")
	fmt.Fprintln(w, "Some settings may require logout/restart to take full effect")
	return nil
}
