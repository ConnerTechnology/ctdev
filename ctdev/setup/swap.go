package setup

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// Swap (`storage`): a swapfile at swapfilePath with one /etc/fstab line, and
// vm.swappiness in a sysctl.d drop-in. ctdev owns the swapfile only when its
// fstab line follows swapFstabMarker; swap that a person or the installer set
// up is reported and left alone by --batch.

// Paths the swap settings read and write. Vars so tests use a temp dir.
var (
	swapsPath          = "/proc/swaps"
	fstabPath          = "/etc/fstab"
	swapfilePath       = "/swapfile"
	swappinessPath     = "/proc/sys/vm/swappiness"
	swappinessConfPath = "/etc/sysctl.d/99-ctdev-swappiness.conf"
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
	lines := strings.Split(out, "\n")
	fields := strings.Fields(lines[len(lines)-1])
	if len(lines) < 2 || len(fields) < 4 {
		return 0, fmt.Errorf("unexpected df output %q", out)
	}
	return strconv.ParseInt(fields[3], 10, 64)
}

// swapFstabMarker is the comment line ctdev writes above its fstab entry.
const swapFstabMarker = "# Swapfile managed by `ctdev configure storage`. Change it there, not here."

// swapEntry is one active swap area from /proc/swaps.
type swapEntry struct {
	name string
	kib  int64
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
		if i == 0 || len(f) < 3 {
			continue // header, or the trailing blank line
		}
		kib, err := strconv.ParseInt(f[2], 10, 64)
		if err != nil {
			continue
		}
		entries = append(entries, swapEntry{name: f[0], kib: kib})
	}
	return entries, nil
}

// formatGiB renders a swap size in GiB to one decimal place, so an 8 GiB
// file reads "8" even though /proc/swaps leaves out its one-page header.
func formatGiB(kib int64) string {
	gib := math.Round(float64(kib)/(1<<20)*10) / 10
	return strconv.FormatFloat(gib, 'f', -1, 64)
}

// detectSwapfileSize reports the active swapfile's size in GiB, or "0" when
// swapfilePath isn't swapping. Swap elsewhere (a partition, zram) isn't a
// swapfile, so it doesn't count here; swapNotNeeded reports it.
func detectSwapfileSize(_ context.Context) string {
	entries, err := readSwaps()
	if err != nil {
		return ValueUnknown
	}
	for _, e := range entries {
		if e.name == swapfilePath {
			return formatGiB(e.kib)
		}
	}
	return "0"
}

// ownsSwapfile reports whether /etc/fstab has ctdev's swapfile line.
func ownsSwapfile() bool {
	data, err := os.ReadFile(fstabPath)
	if err != nil {
		return false
	}
	lines := strings.Split(string(data), "\n")
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

// swapNotNeeded is the NotNeededFn of both swap settings: a machine that
// already swaps somewhere ctdev didn't set up keeps that swap and its tuning,
// so --batch doesn't add a second swapfile or retune someone's choice.
func swapNotNeeded(_ context.Context) string {
	if ownsSwapfile() {
		return ""
	}
	entries, err := readSwaps()
	if err != nil || len(entries) == 0 {
		return ""
	}
	var have []string
	for _, e := range entries {
		have = append(have, fmt.Sprintf("%s, %s GiB", e.name, formatGiB(e.kib)))
	}
	return "this machine already has swap ctdev didn't set up: " + strings.Join(have, "; ")
}

// renderSwapFstab returns fstab content with any swapfilePath line and
// ctdev's marker dropped, plus ctdev's entry at the end when keep is true.
func renderSwapFstab(content string, keep bool) string {
	var out []string
	for _, line := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
		if strings.TrimSpace(line) == swapFstabMarker || isSwapfileLine(line) {
			continue
		}
		out = append(out, line)
	}
	if keep {
		out = append(out, swapFstabMarker, swapfilePath+" none swap sw 0 0")
	}
	return strings.Join(out, "\n") + "\n"
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

// applySwapfileSize makes swapfilePath a swapfile of value GiB, or removes it
// for "0". A swapfile already that size is left as it is; any other size is
// switched off and recreated, because a swap area can't be resized in place.
func applySwapfileSize(ctx context.Context, o sysutil.Opts, value string) error {
	gib, err := strconv.Atoi(value)
	if err != nil || gib < 0 {
		return fmt.Errorf("swapfile size: %q is not a whole number of GiB", value)
	}
	current := detectSwapfileSize(ctx)
	if current == ValueUnknown {
		return fmt.Errorf("swapfile size: can't read %s", swapsPath)
	}
	if current == value {
		return writeSwapFstab(ctx, o, gib > 0)
	}

	if gib > 0 {
		// Check the space before switching anything off, so a full disk
		// leaves the old swapfile working. dd, the fallback, would otherwise
		// fill the disk before failing.
		free, err := swapFreeKiB(ctx, "/")
		if err != nil {
			return fmt.Errorf("swapfile size: check free space: %w", err)
		}
		if st, err := os.Stat(swapfilePath); err == nil {
			free += st.Size() / 1024
		}
		if need := int64(gib) << 20; free < need {
			return fmt.Errorf("swapfile size: %d GiB needs %s GiB free, and only %s GiB is", gib, formatGiB(need), formatGiB(free))
		}
	}

	if current != "0" {
		if err := swapRun(ctx, o, "swapoff", swapfilePath); err != nil {
			return fmt.Errorf("swapoff %s: %w", swapfilePath, err)
		}
	}
	if err := swapRun(ctx, o, "rm", "-f", swapfilePath); err != nil {
		return fmt.Errorf("remove %s: %w", swapfilePath, err)
	}
	if gib == 0 {
		return writeSwapFstab(ctx, o, false)
	}

	if err := createSwapfile(ctx, o, gib); err != nil {
		// Don't leave a half-written file taking up the disk.
		_ = swapRun(ctx, o, "rm", "-f", swapfilePath)
		return err
	}
	if err := swapRun(ctx, o, "swapon", swapfilePath); err != nil {
		return fmt.Errorf("swapon %s: %w", swapfilePath, err)
	}
	return writeSwapFstab(ctx, o, true)
}

// createSwapfile allocates swapfilePath and formats it as swap. fallocate is
// instant, but some filesystems refuse it for swap, so dd writes the file out
// when it fails.
func createSwapfile(ctx context.Context, o sysutil.Opts, gib int) error {
	if err := swapRun(ctx, o, "fallocate", "-l", strconv.Itoa(gib)+"G", swapfilePath); err != nil {
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
		return fmt.Errorf("swappiness: %q is not a number from 0 to 100", value)
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
