package diagnose

import (
	"strings"
	"testing"
	"time"
)

func TestParseDf(t *testing.T) {
	// Real `df -P -k` output, including the pseudo-filesystems and squashfs
	// snap mounts that would otherwise dominate the report — every snap is
	// 100% full by construction.
	const out = `Filesystem              1024-blocks      Used  Available Capacity Mounted on
udev                       32685020         0   32685020       0% /dev
tmpfs                       6537204      2820    6534384       1% /run
/dev/mapper/vgmint-root  1917499848 276238444 1543783980      16% /
tmpfs                      32686020    150000   32536020       1% /dev/shm
/dev/nvme0n1p2               1687168    377856    1191424      25% /boot
/dev/sda1                   960302582 912287453   46015129      96% /mnt/data
/dev/loop12                     58880     58880          0     100% /snap/core22/1908`

	list := parseDf(out)

	mounts := make(map[string]Filesystem)
	for _, fs := range list {
		mounts[fs.Mount] = fs
	}
	if _, present := mounts["/dev"]; present {
		t.Error("udev should be excluded")
	}
	if _, present := mounts["/run"]; present {
		t.Error("tmpfs should be excluded")
	}
	if _, present := mounts["/snap/core22/1908"]; present {
		t.Error("squashfs snap mounts should be excluded — they are always 100% full")
	}
	if got := mounts["/"].UsedPct; got != 15 && got != 16 {
		t.Errorf("root usage = %d%%, want ~16%%", got)
	}
	if got := mounts["/mnt/data"].UsedPct; got < 90 {
		t.Errorf("/mnt/data usage = %d%%, want ~95%%", got)
	}
}

func TestParseDfInodes(t *testing.T) {
	const out = `Filesystem                Inodes     IUsed     IFree IUse% Mounted on
/dev/mapper/vgmint-root 119537664   1854321 117683343    2% /
/dev/sda1                61054976  60000000   1054976   99% /mnt/mail`

	list := parseDfInodes(out)
	byMount := make(map[string]int)
	for _, fs := range list {
		byMount[fs.Mount] = fs.InodePct
	}
	if byMount["/mnt/mail"] < 95 {
		t.Errorf("/mnt/mail inode use = %d%%, want ~99%%", byMount["/mnt/mail"])
	}
	if byMount["/"] > 5 {
		t.Errorf("root inode use = %d%%, want ~2%%", byMount["/"])
	}
}

func TestParseWindowsVolumes(t *testing.T) {
	const out = `C|511103041536|102220608307
D|2000398934016|1900398934016`

	list := parseWindowsVolumes(out)
	if len(list) != 2 {
		t.Fatalf("got %d volumes, want 2", len(list))
	}
	if list[0].Mount != "C:" {
		t.Errorf("mount = %q, want C:", list[0].Mount)
	}
	if list[0].UsedPct != 80 {
		t.Errorf("C: usage = %d%%, want 80%%", list[0].UsedPct)
	}
}

func TestParseMeminfo(t *testing.T) {
	const out = `MemTotal:       65372040 kB
MemFree:         1204188 kB
MemAvailable:   51226452 kB
Buffers:         2049204 kB
SwapTotal:       8388604 kB
SwapFree:        8388604 kB`

	m := parseMeminfo(out)
	if m.TotalKB != 65372040 {
		t.Errorf("total = %d", m.TotalKB)
	}
	// MemAvailable, not MemFree: Linux spends idle memory on cache by design,
	// so MemFree reads alarmingly low on a perfectly healthy machine.
	if m.AvailableKB != 51226452 {
		t.Errorf("available = %d, want MemAvailable not MemFree", m.AvailableKB)
	}
	if m.SwapTotalKB != 8388604 || m.SwapFreeKB != 8388604 {
		t.Errorf("swap = %d/%d", m.SwapFreeKB, m.SwapTotalKB)
	}
}

func TestParseLoadavg(t *testing.T) {
	if v, ok := parseLoadavg("0.83 0.46 0.39 1/3307 236031"); !ok || v != 0.83 {
		t.Errorf("linux loadavg = %v/%v, want 0.83", v, ok)
	}
	// macOS sysctl prints "{ 1.83 2.01 2.10 }"; the braces are trimmed first.
	if v, ok := parseLoadavg("1.83 2.01 2.10"); !ok || v != 1.83 {
		t.Errorf("macos loadavg = %v/%v, want 1.83", v, ok)
	}
	if _, ok := parseLoadavg(""); ok {
		t.Error("empty loadavg should not report ok")
	}
}

func TestParseThrottled(t *testing.T) {
	// A Pi records both live conditions (low bits) and "has happened since
	// boot" (bits 16+). The historical bits catch the intermittent power
	// supply that a spot temperature reading never would.
	got := parseThrottled("throttled=0x50005")
	if !strings.Contains(got, "now:") || !strings.Contains(got, "under-voltage") {
		t.Errorf("parseThrottled = %q, want a live under-voltage condition", got)
	}
	if !strings.Contains(got, "since boot:") {
		t.Errorf("parseThrottled = %q, want the historical bits reported", got)
	}

	if got := parseThrottled("throttled=0x0"); got != "" {
		t.Errorf("a healthy Pi should report nothing, got %q", got)
	}
	if got := parseThrottled("nonsense"); got != "" {
		t.Errorf("unparseable output should report nothing, got %q", got)
	}
}

func TestSmartVerdict(t *testing.T) {
	const healthy = `SMART overall-health self-assessment test result: PASSED

ID# ATTRIBUTE_NAME          FLAG     VALUE WORST THRESH TYPE      UPDATED  WHEN_FAILED RAW_VALUE
  5 Reallocated_Sector_Ct   0x0033   100   100   010    Pre-fail  Always       -       0
197 Current_Pending_Sector  0x0032   100   100   000    Old_age   Always       -       0`

	if got := smartVerdict(healthy); got != smartHealthy {
		t.Errorf("healthy drive = %v, want smartHealthy", got)
	}

	const failed = `SMART overall-health self-assessment test result: FAILED!`
	if got := smartVerdict(failed); got != smartFailing {
		t.Errorf("failed drive = %v, want smartFailing", got)
	}

	// A drive that still says PASSED while reallocating sectors is already on
	// its way out. That early warning is the whole reason to read attributes
	// rather than just the headline.
	const aging = `SMART overall-health self-assessment test result: PASSED

ID# ATTRIBUTE_NAME          FLAG     VALUE WORST THRESH TYPE      UPDATED  WHEN_FAILED RAW_VALUE
  5 Reallocated_Sector_Ct   0x0033   095   095   010    Pre-fail  Always       -       184
197 Current_Pending_Sector  0x0032   100   100   000    Old_age   Always       -       0`
	if got := smartVerdict(aging); got != smartAging {
		t.Errorf("drive with reallocated sectors = %v, want smartAging", got)
	}
}

func TestParseLsblkDisks(t *testing.T) {
	const out = `nvme0n1 disk
sda     disk
loop0   loop
zram0   disk
sr0     rom`

	got := parseLsblkDisks(out)
	want := []string{"/dev/nvme0n1", "/dev/sda"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

func TestParseMdstat(t *testing.T) {
	const healthy = `Personalities : [raid1]
md0 : active raid1 sdb1[1] sda1[0]
      1953514432 blocks super 1.2 [2/2] [UU]

unused devices: <none>`
	arrays, degraded := parseMdstat(healthy)
	if arrays != 1 || len(degraded) != 0 {
		t.Errorf("healthy array: arrays=%d degraded=%v", arrays, degraded)
	}

	// [U_] is a mirror running on one disk: it works perfectly and silently
	// until the survivor dies, which is exactly why it has to be surfaced.
	const broken = `Personalities : [raid1]
md0 : active raid1 sda1[0]
      1953514432 blocks super 1.2 [2/1] [U_]

unused devices: <none>`
	arrays, degraded = parseMdstat(broken)
	if arrays != 1 || len(degraded) != 1 || degraded[0] != "md0" {
		t.Errorf("degraded array: arrays=%d degraded=%v", arrays, degraded)
	}
	if got := mdstatVerdict(broken).Severity; got != Fail {
		t.Errorf("degraded array severity = %v, want Fail", got)
	}
}

func TestZpoolVerdict(t *testing.T) {
	if res, found := zpoolVerdict("all pools are healthy"); !found || res.Severity != OK {
		t.Errorf("healthy pool = %v/%v", res.Severity, found)
	}
	// No pools at all must not read as healthy — the caller falls through to
	// mdadm instead.
	if _, found := zpoolVerdict("no pools available"); found {
		t.Error("no pools should not produce a verdict")
	}
	if res, found := zpoolVerdict("  pool: tank\n state: DEGRADED"); !found || res.Severity != Fail {
		t.Errorf("degraded pool = %v/%v", res.Severity, found)
	}
}

func TestParseFailedUnits(t *testing.T) {
	const out = `● nginx.service loaded failed failed A high performance web server
  restic-backup.service loaded failed failed restic backup`

	got := parseFailedUnits(out)
	if len(got) != 2 || got[0] != "nginx.service" || got[1] != "restic-backup.service" {
		t.Errorf("parseFailedUnits = %v", got)
	}
	if got := parseFailedUnits(""); len(got) != 0 {
		t.Errorf("no failed units should give none, got %v", got)
	}
}

func TestParseOOMVictims(t *testing.T) {
	const out = `Out of memory: Killed process 1234 (chrome) total-vm:8000000kB
Out of memory: Killed process 5678 (node) total-vm:4000000kB
Out of memory: Killed process 9012 (chrome) total-vm:8000000kB`

	got := parseOOMVictims(out)
	// Repeated kills of the same program are one finding, not three.
	if len(got) != 2 {
		t.Fatalf("parseOOMVictims = %v, want chrome and node deduplicated", got)
	}
}

func TestParseDisabledPrinters(t *testing.T) {
	const out = `printer Brother_HL is disabled since Wed 13 Aug 2026 09:14:02 AM CDT -
	Unable to connect
printer Office_Laser is idle.  enabled since Tue 12 Aug 2026`

	got := parseDisabledPrinters(out)
	if len(got) != 1 || got[0] != "Brother_HL" {
		t.Errorf("parseDisabledPrinters = %v, want [Brother_HL]", got)
	}
}

func TestParseAptSimulate(t *testing.T) {
	const out = `Inst libssl3 [3.0.13-0ubuntu3.4] (3.0.13-0ubuntu3.5 Ubuntu:24.04/noble-security [amd64])
Inst vim [2:9.1.0016-1ubuntu7] (2:9.1.0016-1ubuntu7.1 Ubuntu:24.04/noble-updates [amd64])
Conf libssl3 (3.0.13-0ubuntu3.5 Ubuntu:24.04/noble-security [amd64])`

	total, security := parseAptSimulate(out)
	if total != 2 {
		t.Errorf("total = %d, want 2 (Conf lines are not upgrades)", total)
	}
	if security != 1 {
		t.Errorf("security = %d, want 1", security)
	}
}

// This is a regression test for a false positive found on a real machine: the
// drop-ins in sshd_config.d are commonly root-only, so an unreadable file that
// disables password auth was being reported as the opposite.
func TestParsePasswordAuth(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   authState
	}{
		{"commented out is not a setting", "#PasswordAuthentication yes", authUnset},
		{"explicitly disabled", "PasswordAuthentication no", authNo},
		{"explicitly enabled", "PasswordAuthentication yes", authYes},
		// sshd -T emits lowercase keywords.
		{"sshd -T output", "passwordauthentication no", authNo},
		{"nothing to go on", "Port 22\nPermitRootLogin no", authUnset},
		// sshd takes the first occurrence, and drop-ins are included at the
		// top of the shipped config — so the drop-in wins.
		{"first occurrence wins", "PasswordAuthentication no\nPasswordAuthentication yes", authNo},
	}
	for _, tt := range tests {
		if got := parsePasswordAuth(tt.config); got != tt.want {
			t.Errorf("%s: parsePasswordAuth = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestParseSwapusage(t *testing.T) {
	total, used := parseSwapusage("total = 2048.00M  used = 512.00M  free = 1536.00M  (encrypted)")
	if total != 2048*1024 {
		t.Errorf("total = %d KB, want %d", total, 2048*1024)
	}
	if used != 512*1024 {
		t.Errorf("used = %d KB, want %d", used, 512*1024)
	}
}

func TestParseVMStatAvailableKB(t *testing.T) {
	const out = `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                               65536.
Pages active:                            500000.
Pages inactive:                           32768.
Pages speculative:                        16384.
Pages wired down:                        200000.
Pages purgeable:                           8192.`

	// free + inactive + speculative + purgeable = 122880 pages of 16 KiB.
	want := int64(122880) * 16384 / 1024
	if got := parseVMStatAvailableKB(out); got != want {
		t.Errorf("parseVMStatAvailableKB = %d, want %d", got, want)
	}
}

func TestParseMacBatteryHealth(t *testing.T) {
	const out = `      Battery Information:
          Model Information:
              Design Capacity (mAh): 8790
          Charge Information:
              Full Charge Capacity (mAh): 7032
          Health Information:
              Cycle Count: 412
              Condition: Normal`

	pct, found := parseMacBatteryHealth(out)
	if !found || pct != 80 {
		t.Errorf("parseMacBatteryHealth = %d/%v, want 80/true", pct, found)
	}
}

func TestHumanDuration(t *testing.T) {
	tests := map[float64]string{
		0.5:    "30 minutes",
		5:      "5 hours",
		50:     "2 days",
		24 * 8: "8 days",
	}
	for hours, want := range tests {
		d := time.Duration(hours * float64(time.Hour))
		if got := humanDuration(d); got != want {
			t.Errorf("humanDuration(%vh) = %q, want %q", hours, got, want)
		}
	}
}
