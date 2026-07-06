package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show this machine's health at a glance",
	Long: "One screen of operational health: disk headroom, Tailscale connectivity, homelab " +
		"containers, backup freshness (last run, result, integrity check), and pending apt " +
		"updates. Everything is read locally — no network calls — so it's fast enough to run " +
		"on every login. Sections appear only when they apply to this machine.",
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx := cmdContext(cmd)
	host, _ := os.Hostname()

	fmt.Println(styles.Title.Render(fmt.Sprintf("ctdev status — %s", host)) + styles.Dimmed.Render("  (ctdev "+version+")"))
	fmt.Println()
	label := styles.Label(14)

	// System: uptime · load · memory · disk.
	var sys []string
	if up := readUptime(); up != "" {
		sys = append(sys, "up "+up)
	}
	if load := readLoadAvg(); load != "" {
		if n := runtime.NumCPU(); n > 0 {
			load += fmt.Sprintf(" (%d cores)", n)
		}
		sys = append(sys, "load "+load)
	}
	if mem := readMemUsage(); mem != "" {
		sys = append(sys, "mem "+mem)
	}
	if disk := readDiskUsage("/"); disk != "" {
		sys = append(sys, "disk / "+disk)
	}
	if len(sys) > 0 {
		fmt.Printf("  %s %s\n", label.Render("System"), styles.Value.Render(strings.Join(sys, " · ")))
	}

	// Health: reboot pending, failed units, disk SMART — the "something's
	// quietly wrong" catch-alls. Linux only; each shows only when relevant.
	if runtime.GOOS == "linux" {
		if rebootRequired() {
			fmt.Printf("  %s %s\n", label.Render("Reboot"),
				styles.Warning.Render("required")+styles.Dimmed.Render(" — a kernel/security update needs a reboot"))
		}
		if n := failedUnitCount(); n > 0 {
			fmt.Printf("  %s %s\n", label.Render("Failed units"),
				styles.Error.Render(fmt.Sprintf("%d", n))+styles.Dimmed.Render(" — systemctl --failed"))
		}
		if disks := diskHealth(ctx); disks != "" {
			fmt.Printf("  %s %s\n", label.Render("Disk health"), disks)
		}
		if t := cpuThermal(); t != "" {
			fmt.Printf("  %s %s\n", label.Render("Temp"), t)
		}
	}

	// Tailscale connectivity.
	if sysutil.CommandExists("tailscale") {
		if ip := commandOutput(ctx, "tailscale", "ip", "-4"); ip != "" {
			fmt.Printf("  %s %s\n", label.Render("Tailscale"), styles.Success.Render("connected")+styles.Dimmed.Render(" ("+ip+")"))
		} else {
			fmt.Printf("  %s %s\n", label.Render("Tailscale"), styles.Warning.Render("not connected")+styles.Dimmed.Render(" — sudo tailscale up"))
		}
	}

	// Homelab containers (only the stacks installed on this node).
	var containers []string
	for _, name := range []string{"pihole", "caddy", "portainer", "beszel"} {
		if !compInstalled(name) {
			continue
		}
		if sysutil.ContainerRunning(name) {
			containers = append(containers, styles.Success.Render("✓")+" "+name)
		} else {
			containers = append(containers, styles.Error.Render("✗")+" "+name)
		}
	}
	if len(containers) > 0 {
		fmt.Printf("  %s %s\n", label.Render("Containers"), strings.Join(containers, "  "))
	}

	// Backups: freshness and result, read from systemd — no repo round-trip.
	if runtime.GOOS == "linux" && pathExists(resticBackupScript) {
		fmt.Printf("  %s %s\n", label.Render("Backups"), timerStatusLine("restic-backup", 26*time.Hour))
		// The integrity sub-line is only meaningful when backups actually run;
		// suppress it when the backup timer is off. When the check timer was
		// never deployed (pre-v12.3.1), say so plainly instead of "not-found".
		if unitEnabled("restic-backup.timer") {
			var integrity string
			if !pathExists("/etc/systemd/system/restic-check.timer") {
				integrity = styles.Dimmed.Render("not deployed — re-run 'ctdev install restic'")
			} else {
				integrity = timerStatusLine("restic-check", 32*24*time.Hour)
			}
			fmt.Printf("  %s %s\n", label.Render(""), styles.Dimmed.Render("integrity: ")+integrity)
		}
	}

	// Pending apt updates — local read of the existing index (no refresh).
	if _, err := exec.LookPath("apt"); err == nil {
		items, err := scanAPT(ctx)
		switch {
		case err != nil:
			fmt.Printf("  %s %s\n", label.Render("Updates"), styles.Dimmed.Render("apt scan failed: "+err.Error()))
		case len(items) == 0:
			fmt.Printf("  %s %s\n", label.Render("Updates"), styles.Success.Render("apt up to date")+styles.Dimmed.Render(" (as of last index refresh)"))
		default:
			kernels := 0
			for _, it := range items {
				if it.IsKernel {
					kernels++
				}
			}
			line := fmt.Sprintf("%d apt package(s) upgradable", len(items))
			if kernels > 0 {
				line += fmt.Sprintf(" (%d kernel)", kernels)
			}
			fmt.Printf("  %s %s\n", label.Render("Updates"), styles.Warning.Render(line)+styles.Dimmed.Render(" — ctdev update"))
		}
	}

	fmt.Println()
	return nil
}

// timerStatusLine summarizes a systemd timer/service pair: enabled?, when it
// last ran, whether that run succeeded, and a staleness warning when the last
// run is older than maxAge (e.g. a daily backup that hasn't run in >26h).
func timerStatusLine(unit string, maxAge time.Duration) string {
	enabled := unitEnabledState(unit + ".timer")
	if enabled != "enabled" {
		return styles.Warning.Render("timer " + orUnknown(enabled))
	}

	last := unitProperty(unit+".timer", "LastTriggerUSec")
	if last == "" || last == "n/a" {
		return styles.Dimmed.Render("enabled — not yet run")
	}
	t, err := time.Parse("Mon 2006-01-02 15:04:05 MST", last)
	if err != nil {
		return styles.Value.Render("last run " + last)
	}
	age := time.Since(t)

	result := unitProperty(unit+".service", "Result")
	out := fmt.Sprintf("last run %s ago", humanAge(age))
	switch {
	case result != "success" && result != "":
		out = styles.Error.Render(out+" — FAILED") + styles.Dimmed.Render(" (journalctl -u "+unit+")")
	case age > maxAge:
		out = styles.Warning.Render(out + " — overdue")
	default:
		out = styles.Success.Render(out) + styles.Dimmed.Render(" · success")
	}
	return out
}

// readMemUsage returns "used/total" memory from /proc/meminfo, computing used
// as total minus available (the number that actually matters).
func readMemUsage() string {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return ""
	}
	return parseMemUsage(string(b))
}

// parseMemUsage renders "used/total (pct)" from /proc/meminfo, where used is
// total minus available (the figure that reflects real pressure).
func parseMemUsage(meminfo string) string {
	var totalKB, availKB int64
	for _, line := range strings.Split(meminfo, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			totalKB, _ = strconv.ParseInt(fields[1], 10, 64)
		case "MemAvailable:":
			availKB, _ = strconv.ParseInt(fields[1], 10, 64)
		}
	}
	if totalKB == 0 {
		return ""
	}
	used := totalKB - availKB
	return fmt.Sprintf("%s/%s (%d%%)", humanKB(used), humanKB(totalKB), used*100/totalKB)
}

// rebootRequired reports whether a pending kernel/security update needs a
// reboot — Debian/Ubuntu (and unattended-upgrades) drop this flag file.
func rebootRequired() bool {
	return pathExists("/var/run/reboot-required") || pathExists("/run/reboot-required")
}

// failedUnitCount returns how many systemd units are in the failed state.
func failedUnitCount() int {
	out, err := exec.Command("systemctl", "--failed", "--no-legend", "--plain").Output()
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

// diskHealth summarizes SMART status across disks via smartctl. Returns "" when
// smartmontools isn't installed or sudo isn't already cached — status stays
// prompt-free, so this only reports when it can do so without asking for a
// password (sudo -n).
func diskHealth(ctx context.Context) string {
	if !sysutil.CommandExists("smartctl") {
		return ""
	}
	if exec.CommandContext(ctx, "sudo", "-n", "true").Run() != nil {
		return "" // sudo not cached — don't prompt from status
	}
	devs := smartDevices(ctx)
	if len(devs) == 0 {
		return ""
	}
	var failing []string
	healthy := 0
	for _, d := range devs {
		out, _ := exec.CommandContext(ctx, "sudo", "-n", "smartctl", "-H", d).CombinedOutput()
		// smartctl prints "PASSED" (ATA) or "OK" (NVMe) on a healthy drive.
		s := string(out)
		if strings.Contains(s, "PASSED") || strings.Contains(s, "test result: OK") {
			healthy++
		} else {
			failing = append(failing, d)
		}
	}
	if len(failing) > 0 {
		return styles.Error.Render("⚠ FAILING: "+strings.Join(failing, ", ")) + styles.Dimmed.Render(" — replace soon")
	}
	return styles.Success.Render(fmt.Sprintf("%d healthy", healthy))
}

// smartDevices lists physical block devices smartctl can query (via its own
// --scan), so we don't guess device names.
func smartDevices(ctx context.Context) []string {
	out, err := exec.CommandContext(ctx, "sudo", "-n", "smartctl", "--scan").Output()
	if err != nil {
		return nil
	}
	var devs []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		// Lines look like: "/dev/sda -d scsi # ...". Take the device path.
		if f := strings.Fields(line); len(f) > 0 && strings.HasPrefix(f[0], "/dev/") {
			devs = append(devs, f[0])
		}
	}
	return devs
}

// cpuThermal returns the CPU temperature and a throttling warning where the
// kernel exposes them (Raspberry Pi, most laptops). Returns "" when there's no
// thermal zone (e.g. many desktops).
func cpuThermal() string {
	b, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return ""
	}
	milli, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || milli <= 0 {
		return ""
	}
	c := milli / 1000
	line := fmt.Sprintf("%d°C", c)
	if c >= 80 {
		return styles.Warning.Render(line + " — hot")
	}
	return styles.Value.Render(line)
}

// unitProperty reads one systemd unit property value.
func unitProperty(unit, prop string) string {
	out, _ := exec.Command("systemctl", "show", unit, "-p", prop, "--value").Output()
	return strings.TrimSpace(string(out))
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

// humanAge renders a duration the way a person reads it: "11h", "3d 4h".
func humanAge(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 48*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if hours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd %dh", days, hours)
}

// readUptime returns the machine uptime from /proc (Linux; "" elsewhere).
func readUptime() string {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return ""
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return ""
	}
	return humanAge(time.Duration(secs) * time.Second)
}

// readLoadAvg returns the 1-minute load average (Linux; "" elsewhere).
func readLoadAvg() string {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// readDiskUsage returns "used/total (pct)" for a mount via df.
func readDiskUsage(mount string) string {
	out, err := exec.Command("df", "-Pk", mount).Output()
	if err != nil {
		return ""
	}
	return parseDFLine(string(out))
}

// parseDFLine extracts "used/total (pct)" from `df -Pk` output.
func parseDFLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return ""
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 5 {
		return ""
	}
	totalKB, err1 := strconv.ParseInt(fields[1], 10, 64)
	usedKB, err2 := strconv.ParseInt(fields[2], 10, 64)
	if err1 != nil || err2 != nil {
		return ""
	}
	return fmt.Sprintf("%s/%s (%s)", humanKB(usedKB), humanKB(totalKB), fields[4])
}

// humanKB renders a size in 1K blocks as G/T with one decimal where useful.
func humanKB(kb int64) string {
	gb := float64(kb) / (1024 * 1024)
	if gb >= 1024 {
		return fmt.Sprintf("%.1fT", gb/1024)
	}
	if gb >= 10 {
		return fmt.Sprintf("%.0fG", gb)
	}
	return fmt.Sprintf("%.1fG", gb)
}

// commandOutput runs a command and returns its trimmed stdout ("" on error).
func commandOutput(ctx context.Context, name string, args ...string) string {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
