package setup

import (
	"os"
	"os/exec"
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

func detectGrubTimeout() string {
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

func detectGrubStyle() string {
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

func detectGrubOSProber() string {
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

func detectPowerProfile() string {
	out, err := exec.Command("powerprofilesctl", "get").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func detectDconfInt(path string) string {
	out, err := exec.Command("dconf", "read", path).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func detectDconfBool(path string) string {
	out, err := exec.Command("dconf", "read", path).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func detectDconfString(path string) string {
	out, err := exec.Command("dconf", "read", path).Output()
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(out))
	// dconf wraps strings in single quotes
	v = strings.Trim(v, "'")
	return v
}

func detectGsettingsString(schema, key string) string {
	out, err := exec.Command("gsettings", "get", schema, key).Output()
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(out))
	v = strings.Trim(v, "'")
	return v
}

func detectKeyRepeatDelay() string {
	out, err := exec.Command("xset", "q").Output()
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

func detectKeyRepeatRate() string {
	out, err := exec.Command("xset", "q").Output()
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

func detectModuleSigned() string {
	if !detectNvidiaLoaded() {
		return "no nvidia module"
	}
	out, err := exec.Command("modinfo", "-F", "sig_id", "nvidia").Output()
	if err != nil {
		return "unknown"
	}
	sig := strings.TrimSpace(string(out))
	if sig != "" {
		return "signed"
	}
	return "unsigned"
}

func detectNvidiaSuspendServices() string {
	services := []string{
		"nvidia-suspend.service",
		"nvidia-resume.service",
		"nvidia-hibernate.service",
	}
	var active []string
	for _, svc := range services {
		if detectSystemdService(svc) == "active" {
			active = append(active, svc)
		}
	}
	if len(active) == len(services) {
		return "enabled"
	}
	if len(active) == 0 {
		return "disabled"
	}
	return "partial"
}

func detectSystemdService(name string) string {
	out, err := exec.Command("systemctl", "is-active", name).Output()
	if err != nil {
		return "inactive"
	}
	return strings.TrimSpace(string(out))
}

func detectPackageInstalled(pkg string) bool {
	err := exec.Command("dpkg", "-s", pkg).Run()
	return err == nil
}

func detectFileExists(path string) string {
	if _, err := os.Stat(path); err == nil {
		return "installed"
	}
	return "not installed"
}

func detectMouseSpeed() string {
	return detectDconfString("/org/gnome/desktop/peripherals/mouse/speed")
}

func detectNaturalScroll() string {
	return detectDconfBool("/org/gnome/desktop/peripherals/mouse/natural-scroll")
}
