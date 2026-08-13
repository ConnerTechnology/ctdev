package diagnose

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

// checkRebootPending catches the machine that has been patched but never
// restarted — running a kernel and libraries that no longer exist on disk,
// which produces failures nobody can reproduce after the next reboot.
func checkRebootPending(ctx context.Context, f Facts) Result {
	switch f.Platform.OS {
	case platform.Linux:
		if !pathExists("/var/run/reboot-required") {
			return okf("no restart pending")
		}
		pkgs := lines(readFileString("/var/run/reboot-required.pkgs"))
		if len(pkgs) > 0 {
			return warnf("Restart when convenient — until then the machine is running code that's already been replaced on disk.",
				"a restart is pending for %d updated package(s)", len(pkgs))
		}
		return warnf("Restart when convenient.", "a restart is pending from a recent update")

	case platform.Windows:
		out := powershell(ctx,
			`if (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending') `+
				`{'pending'} else {'clear'}`)
		if strings.Contains(out, "pending") {
			return warnf("Restart to finish installing updates.", "a restart is pending from Windows Update")
		}
		return okf("no restart pending")
	}
	return skipf("not tracked on this platform")
}

// checkFailedUnits reports services that gave up. On a machine someone hands
// you, a failed unit is often the whole story — a crashed VPN, a dead
// NetworkManager, a backup that stopped weeks ago.
func checkFailedUnits(ctx context.Context, f Facts) Result {
	if f.Platform.OS != platform.Linux || !commandExists("systemctl") {
		return skipf("no systemd on this machine")
	}
	out := capture(ctx, "systemctl", "--failed", "--no-legend", "--plain")
	names := parseFailedUnits(out)
	switch {
	case len(names) == 0:
		return okf("no failed services")
	case len(names) <= 3:
		return warnf("Check what each one does before dismissing it — 'systemctl status <name>' explains why it stopped.",
			"%d failed: %s", len(names), strings.Join(names, ", "))
	default:
		return warnf("Check what each one does before dismissing it — 'systemctl status <name>' explains why it stopped.",
			"%d failed, including %s", len(names), strings.Join(names[:3], ", "))
	}
}

// parseFailedUnits pulls unit names out of `systemctl --failed --no-legend`.
func parseFailedUnits(out string) []string {
	var names []string
	for _, line := range lines(out) {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		// The bullet in "● name.service" is a separate field when present.
		name := fields[0]
		if name == "●" || name == "*" {
			if len(fields) < 2 {
				continue
			}
			name = fields[1]
		}
		if strings.Contains(name, ".") {
			names = append(names, name)
		}
	}
	return names
}

// checkOOM looks for the kernel having killed something to stay alive. It's
// the explanation behind "the app just disappears" and "the server restarts
// on its own", and it leaves no trace anywhere the user would think to look.
func checkOOM(ctx context.Context, f Facts) Result {
	if f.Platform.OS != platform.Linux || !commandExists("journalctl") {
		return skipf("no journal to read")
	}
	// Boot-scoped: an OOM kill from six weeks ago is history, not a finding.
	out := capture(ctx, "journalctl", "-b", "-k", "--no-pager", "-g", "Out of memory|oom-kill", "-o", "cat")
	victims := parseOOMVictims(out)
	if len(victims) == 0 {
		return okf("nothing killed for memory since boot")
	}
	return warnf("Add memory, or run less at once. Anything killed this way vanished without an error message.",
		"the kernel killed %d process(es) to reclaim memory: %s", len(victims), strings.Join(victims, ", "))
}

// parseOOMVictims extracts process names from kernel OOM lines, which read
// like "Out of memory: Killed process 1234 (chrome) total-vm:...".
func parseOOMVictims(out string) []string {
	var victims []string
	seen := make(map[string]bool)
	for _, line := range lines(out) {
		_, rest, found := strings.Cut(line, "Killed process ")
		if !found {
			continue
		}
		open := strings.IndexByte(rest, '(')
		close := strings.IndexByte(rest, ')')
		if open < 0 || close <= open {
			continue
		}
		name := rest[open+1 : close]
		if !seen[name] {
			seen[name] = true
			victims = append(victims, name)
		}
	}
	return victims
}

// checkPrintQueue catches a stuck print job. It's unglamorous and it's one of
// the most common calls there is — and the queue holds the answer, which
// nobody thinks to check.
func checkPrintQueue(ctx context.Context, f Facts) Result {
	if f.Platform.OS == platform.Windows || !commandExists("lpstat") {
		return skipf("no CUPS on this machine")
	}
	// -W not-completed keeps this to jobs that are actually stuck.
	jobs := lines(capture(ctx, "lpstat", "-W", "not-completed", "-o"))
	disabled := parseDisabledPrinters(capture(ctx, "lpstat", "-p"))

	switch {
	case len(disabled) > 0:
		return warnf("Re-enable it with 'cupsenable <name>'. A queue disables itself after a failed job and stays that way.",
			"printer %s is disabled with %d job(s) waiting", strings.Join(disabled, ", "), len(jobs))
	case len(jobs) >= 5:
		return warnf("Clear the queue with 'cancel -a' if those jobs are stale.",
			"%d print jobs queued and not printing", len(jobs))
	case len(jobs) > 0:
		return infof("%d print job(s) queued", len(jobs))
	default:
		return okf("no stuck print jobs")
	}
}

// parseDisabledPrinters reads `lpstat -p`, whose lines read either
// "printer X is idle." or "printer X disabled since ...".
func parseDisabledPrinters(out string) []string {
	var names []string
	for _, line := range lines(out) {
		if !strings.HasPrefix(line, "printer ") || !strings.Contains(line, "disabled") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			names = append(names, fields[1])
		}
	}
	return names
}

// checkRAID reports a degraded array. A mirror running on one disk works
// perfectly and silently until the second disk goes, which is why it has to be
// surfaced rather than waited for.
func checkRAID(ctx context.Context, f Facts) Result {
	if f.Platform.OS != platform.Linux {
		return skipf("array status is only read on Linux so far")
	}

	if commandExists("zpool") {
		if res, found := zpoolVerdict(capture(ctx, "zpool", "status", "-x")); found {
			return res
		}
	}
	if !pathExists("/proc/mdstat") {
		return skipf("no software RAID on this machine")
	}
	return mdstatVerdict(readFileString("/proc/mdstat"))
}

// zpoolVerdict reads `zpool status -x`, which is designed to answer exactly
// this question: it prints "all pools are healthy" or describes the problem.
func zpoolVerdict(out string) (Result, bool) {
	out = strings.TrimSpace(out)
	switch {
	case out == "":
		return Result{}, false
	case strings.Contains(out, "no pools available"):
		return Result{}, false
	case strings.Contains(out, "all pools are healthy"):
		return okf("all ZFS pools healthy"), true
	default:
		return failf("Replace the failed disk and resilver. The pool is running without its redundancy.",
			"a ZFS pool is not healthy — run 'zpool status' for the detail"), true
	}
}

// mdstatVerdict reads /proc/mdstat. The status line carries a bracket map like
// [UU] or [U_], where an underscore is a missing member.
func mdstatVerdict(content string) Result {
	arrays, degraded := parseMdstat(content)
	switch {
	case arrays == 0:
		return skipf("no software RAID arrays configured")
	case len(degraded) > 0:
		return failf("Replace the failed disk and let the array rebuild. It is running without redundancy right now.",
			"%s degraded — a member disk is missing", strings.Join(degraded, ", "))
	default:
		return okf("%d array(s) healthy", arrays)
	}
}

// parseMdstat counts arrays and names the degraded ones.
func parseMdstat(content string) (arrays int, degraded []string) {
	var current string
	for _, line := range lines(content) {
		if name, _, found := strings.Cut(line, " : "); found && strings.HasPrefix(name, "md") {
			current = name
			arrays++
			continue
		}
		// "      1953514432 blocks super 1.2 [2/1] [U_]"
		open := strings.LastIndex(line, "[")
		if current == "" || open < 0 || !strings.HasSuffix(strings.TrimSpace(line), "]") {
			continue
		}
		if strings.Contains(line[open:], "_") {
			degraded = append(degraded, current)
		}
	}
	return arrays, degraded
}

// checkOSSupport flags an operating system past its support date. It stops
// receiving security fixes, and it's the reason a machine "suddenly" can't
// reach sites any more when a root certificate rolls over.
func checkOSSupport(_ context.Context, f Facts) Result {
	p := f.Platform
	if p.OS != platform.Linux || p.Distro == "" || p.DistroVersion == "" {
		return skipf("support dates are only tracked for Linux so far")
	}
	if eolDistros[p.Distro+" "+p.DistroVersion] {
		return failf("Upgrade to a supported release. This one no longer receives security updates.",
			"%s %s is past end of life", p.Distro, p.DistroVersion)
	}
	return okf("%s %s", p.Distro, p.DistroVersion)
}

// eolDistros lists releases known to be out of support. It is deliberately a
// static list rather than a network lookup: diagnose has to work on a machine
// with no working internet, which is the whole point.
var eolDistros = map[string]bool{
	"ubuntu 14.04": true, "ubuntu 16.04": true, "ubuntu 18.04": true,
	"ubuntu 20.04": true, "ubuntu 21.10": true, "ubuntu 22.10": true,
	"ubuntu 23.04": true, "ubuntu 23.10": true,
	"debian 8": true, "debian 9": true, "debian 10": true, "debian 11": true,
	"linuxmint 19": true, "linuxmint 20": true, "linuxmint 21": true,
	"raspbian 9": true, "raspbian 10": true,
}

// checkPendingUpdates reports outstanding security updates without touching
// the package index — refreshing it on someone else's machine would be a
// change, and this command doesn't make changes.
func checkPendingUpdates(ctx context.Context, f Facts) Result {
	if f.Platform.PackageManager != "apt" || !commandExists("apt-get") {
		return skipf("update counts are only read on apt so far")
	}
	out := capture(ctx, "apt-get", "--simulate", "--quiet", "dist-upgrade")
	total, security := parseAptSimulate(out)
	switch {
	case total == 0:
		return okf("up to date")
	case security > 0:
		return warnf("Run 'ctdev update' — these include security fixes.",
			"%d update(s) available, %d of them security", total, security)
	default:
		return infof("%d update(s) available", total)
	}
}

// parseAptSimulate counts upgrades in `apt-get --simulate dist-upgrade`, whose
// action lines start with "Inst".
func parseAptSimulate(out string) (total, security int) {
	for _, line := range lines(out) {
		if !strings.HasPrefix(line, "Inst ") {
			continue
		}
		total++
		if strings.Contains(strings.ToLower(line), "security") {
			security++
		}
	}
	return total, security
}

// checkUptime is context rather than a fault: a machine up for a year explains
// leaked memory and stale mounts, and one up for four minutes explains why the
// thing they were about to show you isn't happening right now.
func checkUptime(ctx context.Context, f Facts) Result {
	d := uptime(ctx, f.Platform)
	if d <= 0 {
		return skipf("could not read uptime")
	}
	if d.Hours() >= 180*24 {
		return infof("up %s — long enough that a restart is worth trying before anything else", humanDuration(d))
	}
	return infof("up %s", humanDuration(d))
}

func uptime(ctx context.Context, info platform.Info) time.Duration {
	switch info.OS {
	case platform.Linux:
		// /proc/uptime leads with seconds since boot as a float.
		return time.Duration(parseLeadingFloat(readFileString("/proc/uptime")) * float64(time.Second))
	case platform.MacOS:
		// "{ sec = 1755100000, usec = 123456 } Wed Aug 13 09:00:00 2026"
		out := capture(ctx, "sysctl", "-n", "kern.boottime")
		_, rest, found := strings.Cut(out, "sec = ")
		if !found {
			return 0
		}
		secs, _, _ := strings.Cut(rest, ",")
		boot, err := strconv.ParseInt(strings.TrimSpace(secs), 10, 64)
		if err != nil {
			return 0
		}
		return time.Since(time.Unix(boot, 0))
	case platform.Windows:
		out := powershell(ctx,
			`[int]((Get-Date) - (Get-CimInstance Win32_OperatingSystem).LastBootUpTime).TotalSeconds`)
		secs, err := strconv.Atoi(firstLine(out))
		if err != nil {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	return 0
}

func humanDuration(d time.Duration) string {
	switch days := int(d.Hours() / 24); {
	case days >= 2:
		return strconv.Itoa(days) + " days"
	case d.Hours() >= 2:
		return strconv.Itoa(int(d.Hours())) + " hours"
	default:
		return strconv.Itoa(int(d.Minutes())) + " minutes"
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
