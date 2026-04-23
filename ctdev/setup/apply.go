package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/gpu"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

const wifiSleepHookScript = `#!/bin/bash
# MT7925E WiFi suspend fix — PCIe-level reset for reliable reconnect after sleep.
# The simple modprobe approach is unreliable; a full PCIe unbind/remove/rescan
# forces the hardware to re-enumerate cleanly on wake.
DRIVER_DIR="/sys/bus/pci/drivers/mt7925e"

case "$1" in
    pre)
        for dev in "$DRIVER_DIR"/0000:*; do
            [ -e "$dev" ] || continue
            addr=$(basename "$dev")
            echo "$addr" > "$DRIVER_DIR/unbind" 2>/dev/null || true
            echo 1 > "/sys/bus/pci/devices/$addr/remove" 2>/dev/null || true
        done
        ;;
    post)
        echo 1 > /sys/bus/pci/rescan
        sleep 1
        for dev in "$DRIVER_DIR"/0000:*; do
            [ -e "$dev" ] || continue
            addr=$(basename "$dev")
            echo "$addr" > "$DRIVER_DIR/bind" 2>/dev/null || true
        done
        ;;
esac
`

// grubVarArgs returns the sed command args to set a GRUB variable.
// Uses sudo sed -i to replace or append the variable in /etc/default/grub.
// Uses | as the sed delimiter to avoid conflicts with / in values.
func grubVarArgs(varName, value string) []string {
	return []string{
		"sed", "-i",
		fmt.Sprintf("s|^%s=.*|%s=%s|", sedEscape(varName), varName, value),
		"/etc/default/grub",
	}
}

// sedEscape escapes regex metacharacters for use in sed patterns.
func sedEscape(s string) string {
	replacer := strings.NewReplacer(
		`.`, `\.`,
		`*`, `\*`,
		`[`, `\[`,
		`]`, `\]`,
		`^`, `\^`,
		`$`, `\$`,
		`\`, `\\`,
	)
	return replacer.Replace(s)
}

// dconfWriteArgs returns the dconf write command args for a given path and value.
func dconfWriteArgs(path, value string) []string {
	return []string{"dconf", "write", path, value}
}

// applyGrubVar sets a GRUB variable in /etc/default/grub using sed.
// If the variable doesn't exist, it appends it.
func applyGrubVar(ctx context.Context, o sysutil.Opts, varName, value string) error {
	content, err := os.ReadFile("/etc/default/grub")
	if err != nil {
		return fmt.Errorf("read /etc/default/grub: %w", err)
	}

	// Backup before modification (-n = no clobber, preserves original across multiple calls)
	if err := sysutil.SudoRun(ctx, o, "cp", "-n", "/etc/default/grub", "/etc/default/grub.ctdev-backup"); err != nil {
		return fmt.Errorf("backup /etc/default/grub: %w", err)
	}

	// Check if variable already exists — if so, replace it; otherwise append.
	if containsGrubVar(string(content), varName) {
		args := grubVarArgs(varName, value)
		return sysutil.SudoRun(ctx, o, args[0], args[1:]...)
	}

	// Append the variable
	tmpFile, err := os.CreateTemp("", "grub-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(appendGrubLine(string(content), varName, value)); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	return sysutil.SudoRun(ctx, o, "cp", tmpFile.Name(), "/etc/default/grub")
}

// appendGrubLine returns content with `varName=value` appended as a new line.
// A missing trailing newline on content is inserted first so the new variable
// lands on its own line instead of merging with the last existing line.
func appendGrubLine(content, varName, value string) string {
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + fmt.Sprintf("%s=%s\n", varName, value)
}

// containsGrubVar returns true if the content has a non-commented line starting with varName=.
func containsGrubVar(content, varName string) bool {
	prefix := varName + "="
	for _, line := range splitLines(content) {
		if len(line) > 0 && line[0] != '#' && len(line) >= len(prefix) && line[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// applyDconf writes a value to a dconf path (no quoting — suitable for int, bool, double).
func applyDconf(ctx context.Context, o sysutil.Opts, path, value string) error {
	args := dconfWriteArgs(path, value)
	return sysutil.Run(ctx, o, args[0], args[1:]...)
}

// applyDconfString writes a string value to a dconf path, wrapping it in single quotes.
func applyDconfString(ctx context.Context, o sysutil.Opts, path, value string) error {
	quoted := fmt.Sprintf("'%s'", value)
	args := dconfWriteArgs(path, quoted)
	return sysutil.Run(ctx, o, args[0], args[1:]...)
}

// applyGsettings runs gsettings set <schema> <key> <value>.
func applyGsettings(ctx context.Context, o sysutil.Opts, schema, key, value string) error {
	return sysutil.Run(ctx, o, "gsettings", "set", schema, key, value)
}

// applyPowerProfile sets the system power profile via powerprofilesctl.
func applyPowerProfile(ctx context.Context, o sysutil.Opts, value string) error {
	return sysutil.Run(ctx, o, "powerprofilesctl", "set", value)
}

// applyKeyRepeat sets the keyboard repeat rate using xset and gsettings.
// delay is in ms, rate is in characters-per-second.
func applyKeyRepeat(ctx context.Context, o sysutil.Opts, delay, rate string) error {
	if err := sysutil.Run(ctx, o, "xset", "r", "rate", delay, rate); err != nil {
		return fmt.Errorf("xset r rate: %w", err)
	}
	if err := applyGsettings(ctx, o, "org.cinnamon.desktop.peripherals.keyboard", "delay", delay); err != nil {
		return fmt.Errorf("gsettings delay: %w", err)
	}
	// gsettings repeat-interval is in milliseconds between repeats, not cps.
	interval := cpsToIntervalMs(rate)
	if err := applyGsettings(ctx, o, "org.cinnamon.desktop.peripherals.keyboard", "repeat-interval", interval); err != nil {
		return fmt.Errorf("gsettings repeat-interval: %w", err)
	}
	return nil
}

// cpsToIntervalMs converts a characters-per-second rate string to a
// milliseconds-between-repeats interval string for gsettings.
func cpsToIntervalMs(rate string) string {
	rateCps, err := strconv.Atoi(rate)
	if err != nil || rateCps <= 0 {
		return rate
	}
	return strconv.Itoa(1000 / rateCps)
}

// applySystemdEnable enables and starts a systemd service.
func applySystemdEnable(ctx context.Context, o sysutil.Opts, service string) error {
	if err := sysutil.SudoRun(ctx, o, "systemctl", "enable", service); err != nil {
		return fmt.Errorf("systemctl enable %s: %w", service, err)
	}
	if err := sysutil.SudoRun(ctx, o, "systemctl", "start", service); err != nil {
		return fmt.Errorf("systemctl start %s: %w", service, err)
	}
	return nil
}

// applyPackages installs apt packages quietly.
func applyPackages(ctx context.Context, o sysutil.Opts, packages []string) error {
	args := append([]string{"apt-get", "install", "-y", "-qq"}, packages...)
	return sysutil.SudoRun(ctx, o, args[0], args[1:]...)
}

// applyNvidiaSigning runs the GPU signing setup via the gpu package.
func applyNvidiaSigning(ctx context.Context, o sysutil.Opts) error {
	return gpu.RunSetup(ctx, gpu.Opts{
		Stdout: o.Stdout,
		Stdin:  os.Stdin,
	})
}

// applyNvidiaSuspendServices enables NVIDIA suspend-related systemd services.
func applyNvidiaSuspendServices(ctx context.Context, o sysutil.Opts) error {
	services := []string{
		"nvidia-suspend.service",
		"nvidia-resume.service",
		"nvidia-hibernate.service",
	}
	for _, svc := range services {
		// Skip services that don't exist or are static (no [Install] section).
		// (systemctl is-enabled output read plainly; no ctx because this is a read-only probe.)
		out, err := runOutput(ctx, "systemctl", "is-enabled", svc)
		if err != nil {
			continue
		}
		if strings.TrimSpace(out) == "static" {
			continue
		}
		if err := sysutil.SudoRun(ctx, o, "systemctl", "enable", svc); err != nil {
			return fmt.Errorf("enable %s: %w", svc, err)
		}
	}
	return nil
}

// applyWifiSuspendFix writes a systemd sleep hook to handle MT7925E WiFi suspend.
func applyWifiSuspendFix(ctx context.Context, o sysutil.Opts) error {
	return sysutil.SudoWriteFile(ctx, o, wifiSleepHookScript, "/usr/lib/systemd/system-sleep/wifi-mt7925")
}

// applyXbindkeys installs xbindkeys and xdotool, deploys the config, and sets up autostart.
func applyXbindkeys(ctx context.Context, o sysutil.Opts) error {
	if err := applyPackages(ctx, o, []string{"xbindkeys", "xdotool"}); err != nil {
		return fmt.Errorf("install xbindkeys/xdotool: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	configDst := filepath.Join(home, ".xbindkeysrc")

	// Use DeployFileFromFS so any pre-existing user-customized file gets
	// backed up on diff instead of silently overwritten.
	if err := sysutil.DeployFileFromFS(Configs, "configs/xbindkeys/.xbindkeysrc", configDst); err != nil {
		return fmt.Errorf("deploy .xbindkeysrc: %w", err)
	}

	autostartDir := filepath.Join(home, ".config", "autostart")
	if err := os.MkdirAll(autostartDir, 0o755); err != nil {
		return fmt.Errorf("mkdir autostart: %w", err)
	}

	desktopDst := filepath.Join(autostartDir, "xbindkeys.desktop")
	if err := sysutil.DeployFileFromFS(Configs, "configs/xbindkeys/xbindkeys.desktop", desktopDst); err != nil {
		return fmt.Errorf("deploy xbindkeys.desktop: %w", err)
	}

	// Restart xbindkeys to pick up new config; best-effort.
	_ = sysutil.Run(ctx, o, "killall", "xbindkeys")
	// Fire-and-forget: don't block on xbindkeys.
	_ = startDetached("xbindkeys")

	return nil
}

// applyWireplumberLDAC copies the WirePlumber LDAC config from the dotfiles repo.
func applyWireplumberLDAC(ctx context.Context, o sysutil.Opts) error {
	confDir := "/etc/wireplumber/wireplumber.conf.d"
	confDst := filepath.Join(confDir, "51-ldac-hq.conf")

	if err := sysutil.SudoRun(ctx, o, "mkdir", "-p", confDir); err != nil {
		return fmt.Errorf("mkdir wireplumber conf dir: %w", err)
	}

	content, err := Configs.ReadFile("configs/wireplumber/51-ldac-hq.conf")
	if err != nil {
		return fmt.Errorf("read embedded wireplumber config: %w", err)
	}
	if err := sysutil.SudoWriteFile(ctx, o, string(content), confDst); err != nil {
		return fmt.Errorf("copy wireplumber config: %w", err)
	}

	// Restart PipeWire stack to pick up new config; best-effort.
	_ = sysutil.Run(ctx, o, "systemctl", "--user", "restart",
		"pipewire", "pipewire-pulse", "wireplumber")

	return nil
}

// applyLogitechKVMFix installs a udev rule and systemd user service to restart Solaar
// when the Logi Bolt receiver reconnects after a KVM switch.
func applyLogitechKVMFix(ctx context.Context, o sysutil.Opts) error {
	// Deploy udev rule
	udevContent, err := Configs.ReadFile("configs/udev/99-logitech-kvm-fix.rules")
	if err != nil {
		return fmt.Errorf("read embedded udev rule: %w", err)
	}
	if err := sysutil.SudoWriteFile(ctx, o, string(udevContent), "/etc/udev/rules.d/99-logitech-kvm-fix.rules"); err != nil {
		return fmt.Errorf("copy udev rule: %w", err)
	}

	// Deploy systemd user service
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	serviceDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		return fmt.Errorf("mkdir systemd user dir: %w", err)
	}
	serviceContent, err := Configs.ReadFile("configs/udev/solaar-restart.service")
	if err != nil {
		return fmt.Errorf("read embedded service file: %w", err)
	}
	serviceDst := filepath.Join(serviceDir, "solaar-restart.service")
	if err := os.WriteFile(serviceDst, serviceContent, 0o644); err != nil {
		return fmt.Errorf("write service file: %w", err)
	}

	// Enable the user service and reload udev
	_ = sysutil.Run(ctx, o, "systemctl", "--user", "daemon-reload")
	_ = sysutil.Run(ctx, o, "systemctl", "--user", "enable", "solaar-restart.service")
	if err := sysutil.SudoRun(ctx, o, "udevadm", "control", "--reload-rules"); err != nil {
		return fmt.Errorf("reload udev rules: %w", err)
	}

	return nil
}

// applyHideDrives installs a udev rule to hide Windows/secondary partitions from the file manager.
func applyHideDrives(ctx context.Context, o sysutil.Opts) error {
	content, err := Configs.ReadFile("configs/udev/99-hide-drives.rules")
	if err != nil {
		return fmt.Errorf("read embedded hide-drives rule: %w", err)
	}
	if err := sysutil.SudoWriteFile(ctx, o, string(content), "/etc/udev/rules.d/99-hide-drives.rules"); err != nil {
		return fmt.Errorf("copy udev rule: %w", err)
	}
	return sysutil.SudoRun(ctx, o, "udevadm", "control", "--reload-rules")
}

// applySSDTrim enables the fstrim.timer systemd unit for periodic SSD TRIM.
func applySSDTrim(ctx context.Context, o sysutil.Opts) error {
	if err := sysutil.SudoRun(ctx, o, "systemctl", "enable", "fstrim.timer"); err != nil {
		return fmt.Errorf("enable fstrim.timer: %w", err)
	}
	if err := sysutil.SudoRun(ctx, o, "systemctl", "start", "fstrim.timer"); err != nil {
		return fmt.Errorf("start fstrim.timer: %w", err)
	}
	return nil
}

// applyUpdateGrub regenerates the GRUB configuration. Used as a post-apply hook.
func applyUpdateGrub(ctx context.Context, o sysutil.Opts) error {
	return sysutil.SudoRun(ctx, o, "update-grub")
}
