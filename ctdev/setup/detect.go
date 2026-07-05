package setup

import (
	"context"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

// parseXsetRepeat extracts delay and rate from an xset output line like:
// "    auto repeat delay:  200    repeat rate:  50"
func parseXsetRepeat(line string) (delay, rate string) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "auto repeat delay:") {
		return "", ""
	}
	parts := strings.Fields(line)
	for i, p := range parts {
		if p == "delay:" && i+1 < len(parts) {
			delay = parts[i+1]
		}
		if p == "rate:" && i+1 < len(parts) {
			rate = parts[i+1]
		}
	}
	return
}

// parseGrubVar extracts a variable value from GRUB config content.
// Skips commented lines. Strips surrounding quotes.
func parseGrubVar(content, varName string) string {
	prefix := varName + "="
	var result string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, prefix) {
			result = strings.TrimPrefix(line, prefix)
			result = strings.Trim(result, "\"")
		}
	}
	return result
}

// readGrubFile reads /etc/default/grub and returns its content.
func readGrubFile() string {
	data, err := os.ReadFile("/etc/default/grub")
	if err != nil {
		return ""
	}
	return string(data)
}

func detectNvidiaLoaded() bool {
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

func detectMT7925E() bool {
	if out, err := exec.Command("lspci", "-d", "14c3:0717").Output(); err == nil && len(out) > 0 {
		return true
	}
	if out, err := exec.Command("lsmod").Output(); err == nil {
		return strings.Contains(string(out), "mt7925e")
	}
	return false
}

func detectGrubTimeout(_ context.Context) string {
	content := readGrubFile()
	if content == "" {
		return "unknown"
	}
	v := parseGrubVar(content, "GRUB_TIMEOUT")
	if v == "" {
		return "unknown"
	}
	return v
}

func detectGrubStyle(_ context.Context) string {
	content := readGrubFile()
	if content == "" {
		return "unknown"
	}
	v := parseGrubVar(content, "GRUB_TIMEOUT_STYLE")
	if v == "" {
		return "unknown"
	}
	return v
}

func detectGrubOSProber(_ context.Context) string {
	content := readGrubFile()
	if content == "" {
		return "unknown"
	}
	v := parseGrubVar(content, "GRUB_DISABLE_OS_PROBER")
	if v == "false" {
		return "enabled"
	}
	return "disabled"
}

func detectPowerProfile(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "powerprofilesctl", "get").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func detectDconfInt(ctx context.Context, path string) string {
	out, err := exec.CommandContext(ctx, "dconf", "read", path).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func detectDconfBool(ctx context.Context, path string) string {
	out, err := exec.CommandContext(ctx, "dconf", "read", path).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func detectDconfString(ctx context.Context, path string) string {
	out, err := exec.CommandContext(ctx, "dconf", "read", path).Output()
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(out))
	// dconf wraps strings in single quotes
	v = strings.Trim(v, "'")
	return v
}

func detectKeyRepeatDelay(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "xset", "q").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		delay, _ := parseXsetRepeat(line)
		if delay != "" {
			return delay
		}
	}
	return ""
}

func detectKeyRepeatRate(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "xset", "q").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		_, rate := parseXsetRepeat(line)
		if rate != "" {
			return rate
		}
	}
	return ""
}

func detectModuleSigned(ctx context.Context) string {
	if !detectNvidiaLoaded() {
		return "no nvidia module"
	}
	out, err := exec.CommandContext(ctx, "modinfo", "-F", "sig_id", "nvidia").Output()
	if err != nil {
		return "unknown"
	}
	sig := strings.TrimSpace(string(out))
	if sig != "" {
		return "signed"
	}
	return "unsigned"
}

func detectNvidiaSuspendServices(ctx context.Context) string {
	services := []string{
		"nvidia-suspend.service",
		"nvidia-resume.service",
		"nvidia-hibernate.service",
	}
	var ready []string
	for _, svc := range services {
		out, err := exec.CommandContext(ctx, "systemctl", "is-enabled", svc).Output()
		if err != nil {
			continue
		}
		status := strings.TrimSpace(string(out))
		// "enabled" means explicitly enabled; "static" means the service has
		// no [Install] section and is activated by dependency (sleep targets),
		// which is the normal state for NVIDIA suspend services.
		if status == "enabled" || status == "static" {
			ready = append(ready, svc)
		}
	}
	if len(ready) == len(services) {
		return "enabled"
	}
	if len(ready) == 0 {
		return "disabled"
	}
	return "partial"
}

func detectSystemdService(ctx context.Context, name string) string {
	out, err := exec.CommandContext(ctx, "systemctl", "is-active", name).Output()
	if err != nil {
		return "inactive"
	}
	return strings.TrimSpace(string(out))
}

func detectPackageInstalled(ctx context.Context, pkg string) bool {
	err := exec.CommandContext(ctx, "dpkg", "-s", pkg).Run()
	return err == nil
}

func detectFileExists(path string) string {
	if _, err := os.Stat(path); err == nil {
		return "installed"
	}
	return "not installed"
}

func detectLogitechBolt() bool {
	out, err := exec.Command("lsusb").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "046d:c548")
}

// detectUTF8Locale reports "installed" if an en_US UTF-8 locale is generated.
func detectUTF8Locale(ctx context.Context) string {
	out, _ := exec.CommandContext(ctx, "locale", "-a").Output()
	low := strings.ToLower(string(out))
	if strings.Contains(low, "en_us.utf-8") || strings.Contains(low, "en_us.utf8") {
		return "installed"
	}
	return "not installed"
}

// detectSuspendMasked reports "enabled" when all sleep targets are masked.
// `systemctl is-enabled` prints "masked" with a non-zero exit, so the captured
// stdout is checked rather than the error.
func detectSuspendMasked(ctx context.Context) string {
	for _, t := range sleepTargets {
		out, _ := exec.CommandContext(ctx, "systemctl", "is-enabled", t).Output()
		if strings.TrimSpace(string(out)) != "masked" {
			return "disabled"
		}
	}
	return "enabled"
}

// detectLinger reports whether systemd lingering is enabled for the current user.
func detectLinger(ctx context.Context) string {
	u, err := user.Current()
	if err != nil {
		return "disabled"
	}
	out, err := exec.CommandContext(ctx, "loginctl", "show-user", u.Username, "--property=Linger").Output()
	if err != nil {
		return "disabled"
	}
	if strings.TrimSpace(string(out)) == "Linger=yes" {
		return "enabled"
	}
	return "disabled"
}

// detectCodeTunnelService reports whether the VS Code tunnel user service exists.
func detectCodeTunnelService(_ context.Context) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "not installed"
	}
	return detectFileExists(filepath.Join(home, ".config", "systemd", "user", "code-tunnel.service"))
}

func detectMouseSpeed(ctx context.Context) string {
	return detectDconfString(ctx, "/org/gnome/desktop/peripherals/mouse/speed")
}

func detectNaturalScroll(ctx context.Context) string {
	return detectDconfBool(ctx, "/org/gnome/desktop/peripherals/mouse/natural-scroll")
}
