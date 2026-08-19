package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/ConnerTechnology/dotfiles/ctdev/diagnose"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show this machine's health at a glance",
	Long: "One screen of things that may need attention: a pending reboot, failed units, disk " +
		"SMART health, Tailscale connectivity, homelab containers, backup freshness (last run, " +
		"result, integrity check), and pending apt updates. Everything is read locally — no " +
		"network calls — so it's fast enough to run on every login. Sections appear only when " +
		"they apply to this machine. For inventory (specs, uptime, memory and disk usage, " +
		"installed components) see 'ctdev info'.",
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

	// Health comes from the shared diagnose catalog, filtered to the checks
	// that touch nothing but this machine. That keeps one implementation of
	// "is the disk full" instead of two, without breaking the promise that
	// status makes no network calls.
	facts := diagnose.GatherFacts(ctx, platform.Detect())
	results, checks := diagnose.LocalChecks(ctx, platform.Detect(), facts)
	attention := diagnose.NeedsAttention(results)

	for _, c := range checks {
		res, needs := attention[c.ID]
		if !needs {
			continue
		}
		style := styles.Warning
		if res.Severity == diagnose.Fail {
			style = styles.Error
		}
		fmt.Printf("  %s %s\n", label.Render(c.Name), style.Render(res.Detail))
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
	for _, name := range []string{"pihole", "caddy", "portainer", "beszel", "mcp-email-server"} {
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

const (
	// diskWarnPercent is where a filesystem stops being background information
	// and starts being a problem. Below it status says nothing — ctdev info
	// carries the always-on usage view, and repeating it here is noise.
	diskWarnPercent = 85
	// aptStuckAfter is how long a daily apt job may sit mid-run before we call
	// it wedged. A healthy `apt-get update` is minutes at worst, even on a Pi.
	aptStuckAfter = 30 * time.Minute
	// aptIndexStale is when the package index stops being trustworthy — two
	// missed daily runs. Until it refreshes, "up to date" means nothing.
	aptIndexStale = 48 * time.Hour
)

// aptHealth reports trouble with apt's own housekeeping. The daily jobs hold
// /var/lib/apt/lists/lock while they run and are Type=oneshot, so systemd gives
// them no start timeout by default (see 'ctdev configure autoupdate') — one
// that stalls blocks every later apt call for as long as the machine is up,
// while a stale index quietly makes "up to date" a lie. Returns "" when fine.
func aptHealth() string {
	if _, err := exec.LookPath("apt-get"); err != nil {
		return ""
	}
	var problems []string
	for _, unit := range []string{"apt-daily.service", "apt-daily-upgrade.service"} {
		if age := stuckActivating(unit); age > aptStuckAfter {
			problems = append(problems, fmt.Sprintf("%s wedged %s", strings.TrimSuffix(unit, ".service"), humanAge(age)))
		}
	}
	if age := aptIndexAge(); age > aptIndexStale {
		problems = append(problems, fmt.Sprintf("index %s old", humanAge(age)))
	}
	if len(problems) == 0 {
		return ""
	}
	hint := " — sudo systemctl stop apt-daily.service"
	if len(problems) == 1 && strings.HasPrefix(problems[0], "index") {
		hint = " — sudo apt-get update"
	}
	return styles.Warning.Render("⚠ "+strings.Join(problems, ", ")) + styles.Dimmed.Render(hint)
}

// stuckActivating returns how long a unit has been mid-start, or 0 when it
// isn't activating. Units enter "activating" on start and leave it when
// ExecStart returns, so a large age here means the job never finished.
func stuckActivating(unit string) time.Duration {
	if unitProperty(unit, "ActiveState") != "activating" {
		return 0
	}
	started := unitProperty(unit, "InactiveExitTimestamp")
	t, err := time.Parse("Mon 2006-01-02 15:04:05 MST", started)
	if err != nil {
		return 0
	}
	return time.Since(t)
}

// aptIndexAge returns how long ago the package index was last written, read
// from the newest file apt drops in its lists directory on a successful fetch.
func aptIndexAge() time.Duration {
	entries, err := os.ReadDir("/var/lib/apt/lists")
	if err != nil {
		return 0
	}
	var newest time.Time
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if fi.ModTime().After(newest) {
			newest = fi.ModTime()
		}
	}
	if newest.IsZero() {
		return 0
	}
	return time.Since(newest)
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

// commandOutput runs a command and returns its trimmed stdout ("" on error).
func commandOutput(ctx context.Context, name string, args ...string) string {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
