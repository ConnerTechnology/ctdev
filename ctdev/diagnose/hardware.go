package diagnose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// Thresholds. These are the points at which a person notices, not the points
// at which a vendor datasheet complains.
const (
	// A filesystem over 95% breaks package updates, logins that write a
	// profile, and anything that needs a temp file.
	diskFullPct  = 95
	diskWarnPct  = 85
	inodeWarnPct = 90

	// Below 10% available memory the machine is swapping to stay alive.
	memCriticalPct = 10
	memWarnPct     = 20
	swapWarnPct    = 50

	// Load is per-core: 2x cores is busy, 4x is a machine you can't type on.
	loadWarnRatio = 2.0
	loadFailRatio = 4.0

	// Silicon throttles itself around 90-100°C. 80 is where fans should
	// already be loud, and where a clogged heatsink shows up.
	tempWarnC = 80
	tempFailC = 90

	// Battery health as a percentage of design capacity.
	batteryWarnPct = 60
	batteryFailPct = 40
)

// Filesystem is one mounted filesystem's usage.
type Filesystem struct {
	Device   string
	Mount    string
	TotalKB  int64
	UsedKB   int64
	AvailKB  int64
	UsedPct  int
	InodePct int
}

// checkDiskSpace reports the fullest real filesystem. A full disk presents as
// almost anything — failed updates, apps that won't start, a login that hangs
// — so it's worth catching before chasing any of those.
func checkDiskSpace(ctx context.Context, f Facts) Result {
	fsList := filesystems(ctx, f.Platform)
	if len(fsList) == 0 {
		return skipf("could not read filesystem usage")
	}

	worst := fsList[0]
	for _, fs := range fsList {
		if fs.UsedPct > worst.UsedPct {
			worst = fs
		}
	}

	detail := fmt.Sprintf("%s %d%% full (%s free of %s)",
		worst.Mount, worst.UsedPct, sysutil.HumanKB(worst.AvailKB), sysutil.HumanKB(worst.TotalKB))

	switch {
	case worst.UsedPct >= diskFullPct:
		return failf("Free space now — empty the trash, clear Downloads, and run 'ctdev cleanup'. Updates and logins fail at this level.",
			"%s", detail)
	case worst.UsedPct >= diskWarnPct:
		return warnf("Worth clearing before it becomes a problem — 'ctdev cleanup' finds the usual suspects.",
			"%s", detail)
	default:
		return okf("%s", detail)
	}
}

// checkInodes catches the failure that looks exactly like a full disk while df
// insists there's room: millions of tiny files (node_modules, mail spools)
// exhausting the inode table.
func checkInodes(ctx context.Context, f Facts) Result {
	if f.Platform.OS == platform.Windows {
		return skipf("NTFS has no fixed inode table")
	}
	out := capture(ctx, "df", "-P", "-i")
	fsList := parseDfInodes(out)
	if len(fsList) == 0 {
		return skipf("could not read inode usage")
	}

	worst := fsList[0]
	for _, fs := range fsList {
		if fs.InodePct > worst.InodePct {
			worst = fs
		}
	}
	if worst.InodePct >= inodeWarnPct {
		return warnf("The disk has space but no free inodes — find the directory with millions of small files and clear it.",
			"%s has used %d%% of its inodes", worst.Mount, worst.InodePct)
	}
	return okf("%s at %d%%", worst.Mount, worst.InodePct)
}

func filesystems(ctx context.Context, info platform.Info) []Filesystem {
	if info.OS == platform.Windows {
		return parseWindowsVolumes(powershell(ctx,
			`Get-Volume | Where-Object {$_.DriveLetter -and $_.Size -gt 0} |`+
				` ForEach-Object { "$($_.DriveLetter)|$($_.Size)|$($_.SizeRemaining)" }`))
	}
	return parseDf(capture(ctx, "df", "-P", "-k"))
}

// skipFilesystems are pseudo, virtual, or read-only-by-design mounts. Every
// squashfs snap is 100% full by construction, so including them would make the
// report cry wolf on every Ubuntu machine.
var skipFilesystems = []string{
	"tmpfs", "devtmpfs", "squashfs", "overlay", "none", "udev",
	"devfs", "map", "autofs", "efivarfs", "ramfs",
}

var skipMountPrefixes = []string{
	"/snap", "/sys", "/proc", "/dev", "/run", "/System/Volumes",
	"/var/lib/docker", "/var/snap",
}

// parseDf reads POSIX `df -P -k` output, which is stable across Linux and
// macOS precisely because -P forces the portable one-line-per-filesystem form.
func parseDf(out string) []Filesystem {
	var list []Filesystem
	seen := make(map[string]bool)

	for i, line := range lines(out) {
		if i == 0 {
			continue // header
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		device, mount := fields[0], strings.Join(fields[5:], " ")
		if skipFS(device, mount) || seen[device] {
			continue
		}
		seen[device] = true

		total, err1 := strconv.ParseInt(fields[1], 10, 64)
		used, err2 := strconv.ParseInt(fields[2], 10, 64)
		avail, err3 := strconv.ParseInt(fields[3], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil || total <= 0 {
			continue
		}
		list = append(list, Filesystem{
			Device:  device,
			Mount:   mount,
			TotalKB: total,
			UsedKB:  used,
			AvailKB: avail,
			UsedPct: pctOf(used, used+avail),
		})
	}
	return list
}

// parseDfInodes reads `df -P -i`, whose columns are counts rather than blocks.
func parseDfInodes(out string) []Filesystem {
	var list []Filesystem
	seen := make(map[string]bool)

	for i, line := range lines(out) {
		if i == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		device, mount := fields[0], strings.Join(fields[5:], " ")
		if skipFS(device, mount) || seen[device] {
			continue
		}
		seen[device] = true

		used, err1 := strconv.ParseInt(fields[2], 10, 64)
		free, err2 := strconv.ParseInt(fields[3], 10, 64)
		if err1 != nil || err2 != nil || used+free <= 0 {
			continue
		}
		list = append(list, Filesystem{
			Device:   device,
			Mount:    mount,
			InodePct: pctOf(used, used+free),
		})
	}
	return list
}

// parseWindowsVolumes reads the "letter|size|remaining" lines the PowerShell
// probe emits.
func parseWindowsVolumes(out string) []Filesystem {
	var list []Filesystem
	for _, line := range lines(out) {
		parts := strings.Split(line, "|")
		if len(parts) != 3 {
			continue
		}
		size, err1 := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		free, err2 := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
		if err1 != nil || err2 != nil || size <= 0 {
			continue
		}
		totalKB, availKB := size/1024, free/1024
		list = append(list, Filesystem{
			Device:  strings.TrimSpace(parts[0]) + ":",
			Mount:   strings.TrimSpace(parts[0]) + ":",
			TotalKB: totalKB,
			UsedKB:  totalKB - availKB,
			AvailKB: availKB,
			UsedPct: pctOf(totalKB-availKB, totalKB),
		})
	}
	return list
}

func skipFS(device, mount string) bool {
	for _, s := range skipFilesystems {
		if device == s || strings.HasPrefix(device, s) {
			return true
		}
	}
	for _, p := range skipMountPrefixes {
		if mount == p || strings.HasPrefix(mount, p+"/") {
			return true
		}
	}
	return false
}

func pctOf(part, whole int64) int {
	if whole <= 0 {
		return 0
	}
	return int(part * 100 / whole)
}

// checkMemory reports pressure rather than raw usage: Linux caches aggressively
// on purpose, so "used" is meaningless and "available" is the number that says
// whether the machine is about to start swapping.
func checkMemory(ctx context.Context, f Facts) Result {
	m := memoryUsage(ctx, f.Platform)
	if m.TotalKB <= 0 {
		return skipf("could not read memory usage")
	}

	availPct := pctOf(m.AvailableKB, m.TotalKB)
	detail := fmt.Sprintf("%s of %s available", sysutil.HumanKB(m.AvailableKB), sysutil.HumanKB(m.TotalKB))

	swapUsedPct := 0
	if m.SwapTotalKB > 0 {
		swapUsedPct = pctOf(m.SwapTotalKB-m.SwapFreeKB, m.SwapTotalKB)
	}

	switch {
	case availPct <= memCriticalPct:
		return failf("Close the largest applications. The machine is spending its time swapping instead of working.",
			"%s — only %d%% free", detail, availPct)
	case availPct <= memWarnPct:
		return warnf("Getting tight. Expect slowdowns once anything else opens.",
			"%s — %d%% free", detail, availPct)
	case swapUsedPct >= swapWarnPct:
		// Plenty of RAM free but heavy swap in use means something already
		// forced pages out, and touching them again will stutter.
		return warnf("Something has already forced memory to disk. A reboot clears it if performance is poor.",
			"%s, but swap is %d%% used", detail, swapUsedPct)
	default:
		return okf("%s", detail)
	}
}

// MemoryUsage is the memory picture in kilobytes.
type MemoryUsage struct {
	TotalKB     int64
	AvailableKB int64
	SwapTotalKB int64
	SwapFreeKB  int64
}

func memoryUsage(ctx context.Context, info platform.Info) MemoryUsage {
	switch info.OS {
	case platform.Linux:
		data, err := os.ReadFile("/proc/meminfo")
		if err != nil {
			return MemoryUsage{}
		}
		return parseMeminfo(string(data))
	case platform.MacOS:
		return macMemory(ctx)
	case platform.Windows:
		return parseWindowsMemory(powershell(ctx,
			`$os = Get-CimInstance Win32_OperatingSystem;`+
				` "$($os.TotalVisibleMemorySize)|$($os.FreePhysicalMemory)|`+
				`$($os.TotalVirtualMemorySize)|$($os.FreeVirtualMemory)"`))
	}
	return MemoryUsage{}
}

// parseMeminfo reads /proc/meminfo. MemAvailable is the kernel's own estimate
// of what a new allocation could get, which is why it's preferred over
// arithmetic on MemFree.
func parseMeminfo(content string) MemoryUsage {
	var m MemoryUsage
	for _, line := range lines(content) {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		kb := parseLeadingKB(value)
		switch key {
		case "MemTotal":
			m.TotalKB = kb
		case "MemAvailable":
			m.AvailableKB = kb
		case "SwapTotal":
			m.SwapTotalKB = kb
		case "SwapFree":
			m.SwapFreeKB = kb
		}
	}
	return m
}

func parseLeadingKB(s string) int64 {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0
	}
	n, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// parseWindowsMemory reads the pipe-joined CIM values, which are already in KB.
func parseWindowsMemory(out string) MemoryUsage {
	parts := strings.Split(strings.TrimSpace(firstLine(out)), "|")
	if len(parts) != 4 {
		return MemoryUsage{}
	}
	num := func(s string) int64 {
		n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		return n
	}
	// Windows reports virtual (commit) totals rather than a swap file size;
	// the difference between virtual and physical is close enough to page
	// file usage for a warning threshold.
	m := MemoryUsage{TotalKB: num(parts[0]), AvailableKB: num(parts[1])}
	if v, fv := num(parts[2]), num(parts[3]); v > m.TotalKB {
		m.SwapTotalKB = v - m.TotalKB
		m.SwapFreeKB = fv - m.AvailableKB
	}
	return m
}

// checkLoad compares the run queue to the number of cores. The raw load figure
// means nothing without that ratio — 8 is idle on a 32-core box and fatal on a
// Raspberry Pi.
func checkLoad(ctx context.Context, f Facts) Result {
	load, cores, ok0 := loadAverage(ctx, f.Platform)
	if !ok0 || cores <= 0 {
		return skipf("could not read load average")
	}
	ratio := load / float64(cores)
	detail := fmt.Sprintf("%.2f across %d cores", load, cores)

	switch {
	case ratio >= loadFailRatio:
		return failf("Something is consuming the machine. Check which process before assuming it's the network or the disk.",
			"%s — %.1fx oversubscribed", detail, ratio)
	case ratio >= loadWarnRatio:
		return warnf("Busy enough to feel sluggish. Worth seeing what's running.",
			"%s — %.1fx oversubscribed", detail, ratio)
	default:
		return okf("%s", detail)
	}
}

func loadAverage(ctx context.Context, info platform.Info) (load float64, cores int, ok bool) {
	switch info.OS {
	case platform.Linux:
		data, err := os.ReadFile("/proc/loadavg")
		if err != nil {
			return 0, 0, false
		}
		load, ok = parseLoadavg(string(data))
	case platform.MacOS:
		// "{ 1.83 2.01 2.10 }"
		load, ok = parseLoadavg(strings.Trim(capture(ctx, "sysctl", "-n", "vm.loadavg"), "{} "))
	default:
		// Windows has no load average; the equivalent question is answered by
		// the memory and thermal checks.
		return 0, 0, false
	}
	return load, numCPU(), ok
}

// parseLoadavg reads the 1-minute figure, which leads both /proc/loadavg and
// the macOS sysctl.
func parseLoadavg(s string) (float64, bool) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0, false
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// checkTemperature catches thermal throttling, which is the real answer behind
// a large share of "my computer got slow" calls: a clogged heatsink or a dead
// fan, and the silicon quietly halving its own clock to survive.
func checkTemperature(ctx context.Context, f Facts) Result {
	if f.Platform.OS != platform.Linux {
		// macOS needs root for powermetrics, and Windows exposes nothing
		// consistent without vendor drivers.
		return skipf("CPU temperature needs root on this platform")
	}
	name, milliC, found := linuxCPUTemp()
	if !found {
		return skipf("no CPU temperature sensor found")
	}
	c := milliC / 1000

	// A Raspberry Pi records *why* it throttled, which is far more useful
	// than the instantaneous temperature.
	throttle := piThrottleReason(ctx)

	switch {
	case c >= tempFailC:
		return failf("Clean the fans and vents. At this temperature the CPU is slowing itself down to survive.",
			"CPU at %d°C (%s)%s", c, name, throttle)
	case c >= tempWarnC:
		return warnf("Warm. Worth cleaning the vents before it starts throttling.",
			"CPU at %d°C (%s)%s", c, name, throttle)
	default:
		if throttle != "" {
			return warnf("The CPU has throttled recently even though it's cool now — on a Pi this is usually an underpowered supply.",
				"CPU at %d°C (%s)%s", c, name, throttle)
		}
		return okf("CPU at %d°C (%s)", c, name)
	}
}

// cpuSensorNames are the hwmon drivers that expose a CPU package temperature,
// in preference order: AMD, Intel, Raspberry Pi, generic ACPI.
var cpuSensorNames = []string{"k10temp", "coretemp", "cpu_thermal", "zenpower", "acpitz"}

// linuxCPUTemp finds a CPU temperature by hwmon driver name. Reading
// thermal_zone0 directly — as older code does — is unreliable: on AMD desktops
// there are no thermal zones at all, and where there are, zone 0 is as likely
// to be a chipset or wireless sensor as the CPU.
func linuxCPUTemp() (name string, milliC int, found bool) {
	hwmons, _ := filepath.Glob("/sys/class/hwmon/hwmon*")

	for _, want := range cpuSensorNames {
		for _, dir := range hwmons {
			driver := strings.TrimSpace(readFileString(filepath.Join(dir, "name")))
			if driver != want {
				continue
			}
			if v, label, ok := hottestInput(dir); ok {
				return strings.TrimSpace(driver + " " + label), v, true
			}
		}
	}

	// Fall back to the thermal zones, preferring one that names itself.
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*")
	for _, z := range zones {
		t := strings.TrimSpace(readFileString(filepath.Join(z, "type")))
		if !strings.Contains(t, "cpu") && !strings.Contains(t, "x86_pkg") {
			continue
		}
		if v, err := strconv.Atoi(strings.TrimSpace(readFileString(filepath.Join(z, "temp")))); err == nil {
			return t, v, true
		}
	}
	return "", 0, false
}

// hottestInput returns the highest tempN_input in a hwmon directory, with its
// label. On a multi-die CPU the package sensor is the one that matters, and it
// reads highest.
func hottestInput(dir string) (milliC int, label string, found bool) {
	inputs, _ := filepath.Glob(filepath.Join(dir, "temp*_input"))
	for _, in := range inputs {
		v, err := strconv.Atoi(strings.TrimSpace(readFileString(in)))
		if err != nil || v <= 0 {
			continue
		}
		if v > milliC {
			milliC = v
			label = strings.TrimSpace(readFileString(strings.TrimSuffix(in, "_input") + "_label"))
			found = true
		}
	}
	return milliC, label, found
}

// piThrottleReason reads the Raspberry Pi throttle bitmask, which records both
// current and historical undervoltage and thermal events.
func piThrottleReason(ctx context.Context) string {
	if !commandExists("vcgencmd") {
		return ""
	}
	return parseThrottled(capture(ctx, "vcgencmd", "get_throttled"))
}

// parseThrottled decodes `vcgencmd get_throttled` output ("throttled=0x50005").
// The low bits are live conditions; bits 16+ are "has happened since boot",
// which is what catches an intermittent power supply.
func parseThrottled(out string) string {
	_, hex, found := strings.Cut(strings.TrimSpace(out), "=")
	if !found {
		return ""
	}
	v, err := strconv.ParseUint(strings.TrimPrefix(strings.TrimSpace(hex), "0x"), 16, 64)
	if err != nil || v == 0 {
		return ""
	}

	var now, past []string
	for _, b := range []struct {
		bit  uint
		text string
	}{
		{0, "under-voltage"},
		{1, "frequency capped"},
		{2, "throttled"},
		{3, "soft temperature limit"},
	} {
		if v&(1<<b.bit) != 0 {
			now = append(now, b.text)
		}
		if v&(1<<(b.bit+16)) != 0 {
			past = append(past, b.text)
		}
	}

	var parts []string
	if len(now) > 0 {
		parts = append(parts, "now: "+strings.Join(now, ", "))
	}
	if len(past) > 0 {
		parts = append(parts, "since boot: "+strings.Join(past, ", "))
	}
	if len(parts) == 0 {
		return ""
	}
	return " — " + strings.Join(parts, "; ")
}

// numCPU is the core count the load ratio is measured against. runtime honors
// cgroup limits, which matters inside a container.
func numCPU() int { return runtime.NumCPU() }

func readFileString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// checkSMART reads the drive's own health self-assessment. A drive reporting
// reallocated or pending sectors is failing whether or not anyone has noticed
// yet, and this is the one check where being early actually saves the data.
func checkSMART(ctx context.Context, f Facts) Result {
	if !commandExists("smartctl") {
		return skipf("smartctl is not installed")
	}
	if !f.Root {
		// Degrade rather than prompt: on a machine we're visiting, a password
		// prompt inside a report is the wrong thing to do.
		return skipf("needs root — re-run with sudo for disk health")
	}

	devices := smartDevices(ctx)
	if len(devices) == 0 {
		return skipf("no SMART-capable drives found")
	}

	var failing, aging []string
	for _, dev := range devices {
		out, gotRoot := sudoCapture(ctx, "smartctl", "-H", "-A", dev)
		if !gotRoot {
			return skipf("needs root — re-run with sudo for disk health")
		}
		switch smartVerdict(out) {
		case smartFailing:
			failing = append(failing, dev)
		case smartAging:
			aging = append(aging, dev)
		}
	}

	switch {
	case len(failing) > 0:
		return failf("Back up now, then replace the drive. A drive that reports this is failing, not 'might fail'.",
			"%s reports failing health", strings.Join(failing, ", "))
	case len(aging) > 0:
		return warnf("Reallocated or pending sectors mean the drive is quietly working around damage. Make sure backups are current.",
			"%s has reallocated or pending sectors", strings.Join(aging, ", "))
	default:
		return okf("%d drive(s) report healthy", len(devices))
	}
}

type smartHealth int

const (
	smartHealthy smartHealth = iota
	smartAging
	smartFailing
)

// smartVerdict reads `smartctl -H -A` output. The overall self-assessment is
// the headline, but a PASSED drive with growing reallocated or pending sectors
// is already on its way out — that's the early warning worth having.
func smartVerdict(out string) smartHealth {
	lower := strings.ToLower(out)
	if strings.Contains(lower, "self-assessment test result: failed") ||
		strings.Contains(lower, "smart health status: failed") {
		return smartFailing
	}

	for _, line := range lines(out) {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		switch fields[1] {
		case "Reallocated_Sector_Ct", "Current_Pending_Sector", "Offline_Uncorrectable":
			// The raw value is the last column.
			if n, err := strconv.ParseInt(fields[len(fields)-1], 10, 64); err == nil && n > 0 {
				return smartAging
			}
		}
	}
	return smartHealthy
}

// smartDevices lists block devices worth asking about, skipping partitions,
// loop devices, and anything virtual.
func smartDevices(ctx context.Context) []string {
	if !commandExists("lsblk") {
		return nil
	}
	return parseLsblkDisks(capture(ctx, "lsblk", "-dno", "NAME,TYPE"))
}

func parseLsblkDisks(out string) []string {
	var devices []string
	for _, line := range lines(out) {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[1] != "disk" {
			continue
		}
		if strings.HasPrefix(fields[0], "loop") || strings.HasPrefix(fields[0], "zram") {
			continue
		}
		devices = append(devices, "/dev/"+fields[0])
	}
	return devices
}

// checkBattery compares present capacity to the design capacity. A battery at
// half its original capacity is the whole explanation for "it doesn't hold a
// charge any more", and nothing in the OS surfaces it plainly.
func checkBattery(ctx context.Context, f Facts) Result {
	health, found := batteryHealth(ctx, f.Platform)
	if !found {
		return skipf("no battery, or capacity is not reported")
	}
	switch {
	case health <= batteryFailPct:
		return failf("The battery is worn out. Replace it, or expect the machine to run only on mains power.",
			"battery holds %d%% of its original capacity", health)
	case health <= batteryWarnPct:
		return warnf("Noticeably worn. Runtime will be roughly this fraction of what it was new.",
			"battery holds %d%% of its original capacity", health)
	default:
		return okf("battery holds %d%% of its original capacity", health)
	}
}

func batteryHealth(ctx context.Context, info platform.Info) (pct int, found bool) {
	switch info.OS {
	case platform.Linux:
		for _, dir := range globPaths("/sys/class/power_supply/BAT*") {
			// Laptops report either energy (µWh) or charge (µAh); the ratio
			// is what matters, so either pair works.
			for _, pair := range [][2]string{
				{"energy_full", "energy_full_design"},
				{"charge_full", "charge_full_design"},
			} {
				now := parseLeadingFloat(readFileString(filepath.Join(dir, pair[0])))
				design := parseLeadingFloat(readFileString(filepath.Join(dir, pair[1])))
				if now > 0 && design > 0 {
					return int(now / design * 100), true
				}
			}
		}
	case platform.MacOS:
		return parseMacBatteryHealth(capture(ctx, "system_profiler", "SPPowerDataType"))
	}
	return 0, false
}

// parseMacBatteryHealth reads the charge figures out of SPPowerDataType.
func parseMacBatteryHealth(out string) (pct int, found bool) {
	var now, design float64
	for _, line := range lines(out) {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "Full Charge Capacity (mAh)":
			now = parseLeadingFloat(strings.TrimSpace(value))
		case "Design Capacity (mAh)":
			design = parseLeadingFloat(strings.TrimSpace(value))
		}
	}
	if now > 0 && design > 0 {
		return int(now / design * 100), true
	}
	return 0, false
}
