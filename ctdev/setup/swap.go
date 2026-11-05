package setup

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// Swap (`storage`): a swapfile at swapfilePath with one /etc/fstab line, and
// vm.swappiness in a sysctl.d drop-in. ctdev owns the swapfile only when its
// fstab line follows swapFstabMarker. Swap that a person or the installer set
// up, switched on or not, is reported and left alone by --batch.

// Paths the swap settings read and write. Vars so tests use a temp dir.
var (
	swapsPath      = "/proc/swaps"
	meminfoPath    = "/proc/meminfo"
	fstabPath      = "/etc/fstab"
	swapfilePath   = "/swapfile"
	swappinessPath = "/proc/sys/vm/swappiness"
	// The zz- prefix sorts it after every numbered sysctl.d file and the
	// 99-sysctl.conf link to /etc/sysctl.conf, so ctdev's value wins at boot.
	swappinessConfPath = "/etc/sysctl.d/zz-ctdev-swappiness.conf"
)

// swapRun and swapWrite change the system as root; tests substitute fakes.
var (
	swapRun   = sysutil.SudoRun
	swapWrite = sysutil.SudoWriteFileMode
)

// swapFreeKiB reports the space available on the filesystem holding dir, in
// KiB. Read-only; tests substitute a fake.
var swapFreeKiB = func(ctx context.Context, dir string) (int64, error) {
	out, err := runOutput(ctx, "df", "-Pk", dir)
	if err != nil {
		return 0, err
	}
	return parseDfAvailKiB(out)
}

// swapFSType reports the type of the filesystem holding dir, as `stat -f`
// names it (ext4 shows as "ext2/ext3"). Read-only; tests substitute a fake.
var swapFSType = func(ctx context.Context, dir string) (string, error) {
	return runOutput(ctx, "stat", "-f", "-c", "%T", dir)
}

// parseDfAvailKiB reads the Available column of `df -Pk` output.
func parseDfAvailKiB(out string) (int64, error) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("unexpected df output %q", out)
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 4 {
		return 0, fmt.Errorf("unexpected df output %q", out)
	}
	return strconv.ParseInt(fields[3], 10, 64)
}

// swapFstabMarker is the comment line ctdev writes above its fstab entry.
const swapFstabMarker = "# Swapfile managed by `ctdev configure storage`. Change it there, not here."

// swapoffHeadroomKiB is the memory left over after swapoff moves swapped-out
// pages back in. Without it swapoff can push the machine into the OOM killer.
const swapoffHeadroomKiB = 256 << 10

// swapEntry is one active swap area from /proc/swaps.
type swapEntry struct {
	name    string
	kib     int64
	usedKiB int64
}

// readSwaps lists the active swap areas: files, partitions and zram alike.
func readSwaps() ([]swapEntry, error) {
	data, err := os.ReadFile(swapsPath)
	if err != nil {
		return nil, err
	}
	var entries []swapEntry
	for i, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if i == 0 || len(f) < 4 {
			continue // header, or the trailing blank line
		}
		kib, err1 := strconv.ParseInt(f[2], 10, 64)
		used, err2 := strconv.ParseInt(f[3], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		entries = append(entries, swapEntry{name: f[0], kib: kib, usedKiB: used})
	}
	return entries, nil
}

// activeSwapfile returns ctdev's path's entry in /proc/swaps, or nil.
func activeSwapfile(entries []swapEntry) *swapEntry {
	for i := range entries {
		if entries[i].name == swapfilePath {
			return &entries[i]
		}
	}
	return nil
}

// memAvailableKiB reads MemAvailable from /proc/meminfo.
func memAvailableKiB() (int64, error) {
	data, err := os.ReadFile(meminfoPath)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) >= 2 && f[0] == "MemAvailable:" {
			return strconv.ParseInt(f[1], 10, 64)
		}
	}
	return 0, fmt.Errorf("no MemAvailable in %s", meminfoPath)
}

// formatGiB renders a swap size in GiB to one decimal place, so an 8 GiB
// file reads "8" even though /proc/swaps leaves out its one-page header.
func formatGiB(kib int64) string {
	gib := math.Round(float64(kib)/(1<<20)*10) / 10
	return strconv.FormatFloat(gib, 'f', -1, 64)
}

// formatSize renders an amount of memory or disk for a message: MiB under a
// GiB, where formatGiB would round a few hundred MiB to "0".
func formatSize(kib int64) string {
	if kib < 1<<20 {
		return strconv.FormatInt(kib>>10, 10) + " MiB"
	}
	return formatGiB(kib) + " GiB"
}

// detectSwapfileSize reports the active swapfile's size in GiB, or "0" when
// swapfilePath isn't swapping. Swap elsewhere (a partition, zram) isn't a
// swapfile, so it doesn't count here; the NotNeededFns report it.
func detectSwapfileSize(_ context.Context) string {
	entries, err := readSwaps()
	if err != nil {
		return ValueUnknown
	}
	if e := activeSwapfile(entries); e != nil {
		return formatGiB(e.kib)
	}
	return "0"
}

// fstabLines splits fstab content into lines without their line endings.
func fstabLines(content string) []string {
	content = strings.TrimRight(content, "\r\n")
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	return lines
}

// readFstabLines reads /etc/fstab's lines; a missing fstab has none.
func readFstabLines() []string {
	data, err := os.ReadFile(fstabPath)
	if err != nil {
		return nil
	}
	return fstabLines(string(data))
}

// ownsSwapfile reports whether /etc/fstab has ctdev's swapfile line.
func ownsSwapfile() bool {
	lines := readFstabLines()
	for i := 0; i+1 < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == swapFstabMarker && isSwapfileLine(lines[i+1]) {
			return true
		}
	}
	return false
}

// isSwapfileLine reports whether an fstab line mounts swapfilePath.
func isSwapfileLine(line string) bool {
	f := strings.Fields(line)
	return len(f) > 0 && f[0] == swapfilePath
}

// foreignSwap describes the swap on this machine ctdev didn't set up, or ""
// when there is none: what /proc/swaps lists, a file at swapfilePath that
// isn't switched on, and swap entries in /etc/fstab. Only called when ctdev
// doesn't own the swapfile.
func foreignSwap() string {
	var have []string
	seen := map[string]bool{}
	entries, _ := readSwaps()
	for _, e := range entries {
		have = append(have, fmt.Sprintf("%s, %s GiB", e.name, formatGiB(e.kib)))
		seen[e.name] = true
	}
	if _, err := os.Lstat(swapfilePath); err == nil && !seen[swapfilePath] {
		have = append(have, swapfilePath+", not switched on")
		seen[swapfilePath] = true
	}
	for _, line := range readFstabLines() {
		f := strings.Fields(line)
		if len(f) < 3 || strings.HasPrefix(f[0], "#") || f[2] != "swap" || seen[f[0]] {
			continue
		}
		have = append(have, f[0]+", in /etc/fstab")
		seen[f[0]] = true
	}
	return strings.Join(have, "; ")
}

// swapFSSupported reports whether the kernel can swap to a file on a
// filesystem of this type, as swapFSType names it.
func swapFSSupported(fs string) bool {
	switch fs {
	case "ext2/ext3", "ext4", "xfs", "btrfs":
		return true
	}
	return false
}

// swapfileNotNeeded is the swapfile's NotNeededFn. --batch only creates a
// swapfile where there is no swap at all: it never resizes one, ctdev's
// included, and never touches swap someone else set up. The wizard can still
// do either when asked.
func swapfileNotNeeded(ctx context.Context) string {
	if ownsSwapfile() {
		if size := detectSwapfileSize(ctx); size != "0" && size != ValueUnknown {
			return fmt.Sprintf("ctdev's %s GiB swapfile is already set up, and --batch doesn't resize it", size)
		}
		return ""
	}
	if have := foreignSwap(); have != "" {
		return "this machine already has swap ctdev didn't set up: " + have
	}
	dir := filepath.Dir(swapfilePath)
	if fs, err := swapFSType(ctx, dir); err == nil && !swapFSSupported(fs) {
		return fmt.Sprintf("%s is on %s, which can't hold a swapfile", dir, fs)
	}
	return ""
}

// swappinessNotNeeded is swappiness's NotNeededFn: on swap someone else set
// up, their tuning stays theirs.
func swappinessNotNeeded(_ context.Context) string {
	if ownsSwapfile() {
		return ""
	}
	if have := foreignSwap(); have != "" {
		return "this machine already has swap ctdev didn't set up: " + have
	}
	return ""
}

// renderSwapFstab returns fstab content with any swapfilePath line and
// ctdev's marker dropped, plus ctdev's entry at the end when keep is true.
// It keeps the file's own line endings.
func renderSwapFstab(content string, keep bool) string {
	nl := "\n"
	if strings.Contains(content, "\r\n") {
		nl = "\r\n"
	}
	var out []string
	for _, line := range fstabLines(content) {
		if strings.TrimSpace(line) == swapFstabMarker || isSwapfileLine(line) {
			continue
		}
		out = append(out, line)
	}
	if keep {
		out = append(out, swapFstabMarker, swapfilePath+" none swap sw 0 0")
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, nl) + nl
}

// writeSwapFstab rewrites /etc/fstab when its swapfile entry needs to change,
// keeping a copy of the original the first time, as applyGrubVar does.
func writeSwapFstab(ctx context.Context, o sysutil.Opts, keep bool) error {
	data, err := os.ReadFile(fstabPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", fstabPath, err)
	}
	want := renderSwapFstab(string(data), keep)
	if want == string(data) {
		return nil
	}
	if len(data) > 0 {
		if err := swapRun(ctx, o, "cp", "-n", fstabPath, fstabPath+".ctdev-backup"); err != nil {
			return fmt.Errorf("back up %s: %w", fstabPath, err)
		}
	}
	return swapWrite(ctx, o, want, fstabPath, "0644")
}

// swapoffSwapfile switches the swapfile off, unless what it holds wouldn't
// fit back in memory: swapoff then gets OOM-killed, or takes something with it.
func swapoffSwapfile(ctx context.Context, o sysutil.Opts, e *swapEntry) error {
	avail, err := memAvailableKiB()
	if err != nil {
		return fmt.Errorf("check free memory: %w", err)
	}
	if e.usedKiB > avail-swapoffHeadroomKiB {
		return fmt.Errorf("swap in use (%s) exceeds free memory (%s); free memory first",
			formatSize(e.usedKiB), formatSize(avail))
	}
	if err := swapRun(ctx, o, "swapoff", swapfilePath); err != nil {
		return fmt.Errorf("swapoff %s: %w", swapfilePath, err)
	}
	return nil
}

// applySwapfileSize makes swapfilePath a swapfile of value GiB, or removes it
// for "0". A swapfile already that size is left as it is; any other size is
// switched off and recreated, because a swap area can't be resized in place.
//
// It replaces only a swapfile ctdev owns or one that is switched on, which it
// reaches only when the person chose to take it over (--batch skips swap
// ctdev didn't set up). Any other file at swapfilePath is someone's, so it
// stops rather than delete it.
func applySwapfileSize(ctx context.Context, o sysutil.Opts, value string) error {
	gib, err := strconv.Atoi(value)
	if err != nil || gib < 0 {
		return fmt.Errorf("%q is not a whole number of GiB", value)
	}
	entries, err := readSwaps()
	if err != nil {
		return fmt.Errorf("read %s: %w", swapsPath, err)
	}
	active := activeSwapfile(entries)
	st, statErr := os.Lstat(swapfilePath)
	exists := statErr == nil
	if exists && active == nil && !ownsSwapfile() {
		return fmt.Errorf("%s exists, isn't switched on and ctdev didn't create it; move it away first", swapfilePath)
	}

	if gib == 0 {
		if active != nil {
			if err := swapoffSwapfile(ctx, o, active); err != nil {
				return err
			}
		}
		if exists {
			if err := swapRun(ctx, o, "rm", "-f", swapfilePath); err != nil {
				return fmt.Errorf("remove %s: %w", swapfilePath, err)
			}
		}
		return writeSwapFstab(ctx, o, false)
	}
	if active != nil && formatGiB(active.kib) == value {
		return writeSwapFstab(ctx, o, true)
	}

	dir := filepath.Dir(swapfilePath)
	fs, err := swapFSType(ctx, dir)
	if err != nil {
		return fmt.Errorf("check the filesystem at %s: %w", dir, err)
	}
	if !swapFSSupported(fs) {
		return fmt.Errorf("%s is on %s, which can't hold a swapfile", dir, fs)
	}
	// Check the space before switching anything off, so a full disk leaves
	// the old swapfile working, with headroom so the swapfile doesn't leave
	// the disk full. dd, the fallback, would otherwise fill it before failing.
	free, err := swapFreeKiB(ctx, dir)
	if err != nil {
		return fmt.Errorf("check free space: %w", err)
	}
	if exists {
		free += st.Size() / 1024
	}
	need := int64(gib) << 20
	if want := need + max(need/10, 512<<10); free < want {
		return fmt.Errorf("%d GiB needs %s free, and only %s is", gib, formatSize(want), formatSize(free))
	}

	if active != nil {
		if err := swapoffSwapfile(ctx, o, active); err != nil {
			return err
		}
	}
	if err := swapRun(ctx, o, "rm", "-f", swapfilePath); err != nil {
		return fmt.Errorf("remove %s: %w", swapfilePath, err)
	}
	// From here a failure leaves no swapfile, so it also leaves no fstab line
	// pointing at one. The cleanup runs even after Ctrl-C.
	rollback := func(err error) error {
		c := context.WithoutCancel(ctx)
		_ = swapRun(c, o, "rm", "-f", swapfilePath)
		_ = writeSwapFstab(c, o, false)
		return err
	}
	if err := createSwapfile(ctx, o, gib, fs); err != nil {
		return rollback(err)
	}
	if err := writeSwapFstab(ctx, o, true); err != nil {
		return rollback(err)
	}
	if err := swapRun(ctx, o, "swapon", swapfilePath); err != nil {
		return rollback(fmt.Errorf("swapon %s: %w", swapfilePath, err))
	}
	return nil
}

// createSwapfile allocates swapfilePath and formats it as swap. fallocate is
// instant, but some filesystems refuse it for swap, so dd writes the file out
// when it fails. btrfs only swaps to a file that is never copy-on-write.
func createSwapfile(ctx context.Context, o sysutil.Opts, gib int, fs string) error {
	size := strconv.Itoa(gib) + "G"
	if fs == "btrfs" {
		// btrfs-progs 6.1+ does all of it: NOCOW, allocation, mode and mkswap.
		if swapRun(ctx, o, "btrfs", "filesystem", "mkswapfile", "--size", size, swapfilePath) == nil {
			return nil
		}
		// Older btrfs-progs: NOCOW only takes on an empty file.
		if err := swapRun(ctx, o, "truncate", "-s", "0", swapfilePath); err != nil {
			return fmt.Errorf("create %s: %w", swapfilePath, err)
		}
		if err := swapRun(ctx, o, "chattr", "+C", swapfilePath); err != nil {
			return fmt.Errorf("make %s NOCOW: %w", swapfilePath, err)
		}
	}
	if err := swapRun(ctx, o, "fallocate", "-l", size, swapfilePath); err != nil {
		if err := swapRun(ctx, o, "dd", "if=/dev/zero", "of="+swapfilePath, "bs=1M",
			"count="+strconv.Itoa(gib*1024), "status=none"); err != nil {
			return fmt.Errorf("create %s: %w", swapfilePath, err)
		}
	}
	// swapon refuses a swapfile other users can read.
	if err := swapRun(ctx, o, "chmod", "600", swapfilePath); err != nil {
		return fmt.Errorf("chmod %s: %w", swapfilePath, err)
	}
	if err := swapRun(ctx, o, "mkswap", swapfilePath); err != nil {
		return fmt.Errorf("mkswap %s: %w", swapfilePath, err)
	}
	return nil
}

// detectSwappiness reads the kernel's current vm.swappiness.
func detectSwappiness(_ context.Context) string {
	data, err := os.ReadFile(swappinessPath)
	if err != nil {
		return ValueUnknown
	}
	return strings.TrimSpace(string(data))
}

// applySwappiness sets vm.swappiness now with sysctl and for every boot with
// ctdev's sysctl.d drop-in.
func applySwappiness(ctx context.Context, o sysutil.Opts, value string) error {
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 || n > 100 {
		return fmt.Errorf("%q is not a number from 0 to 100", value)
	}
	conf := "# Written by `ctdev configure storage` (Swappiness). Change it there, not here.\n" +
		"vm.swappiness = " + strconv.Itoa(n) + "\n"
	if err := swapWrite(ctx, o, conf, swappinessConfPath, "0644"); err != nil {
		return fmt.Errorf("write %s: %w", swappinessConfPath, err)
	}
	if err := swapRun(ctx, o, "sysctl", "-q", "-w", "vm.swappiness="+strconv.Itoa(n)); err != nil {
		return fmt.Errorf("sysctl vm.swappiness: %w", err)
	}
	return nil
}
