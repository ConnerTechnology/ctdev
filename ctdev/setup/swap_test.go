package setup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

const swapsHeader = "Filename\t\t\t\tType\t\tSize\t\tUsed\t\tPriority\n"

// fakeSwap points the swap settings at files in a temp dir and records the
// commands and writes they make instead of running them.
type fakeSwap struct {
	t       *testing.T
	dir     string
	ran     []string
	written map[string]string
	fail    map[string]bool // command names that fail
	freeKiB int64
	fsType  string
}

// newFakeSwap fakes /proc/swaps and /etc/fstab. The fixtures say /swapfile;
// the code under test sees a path in the temp dir, so the host's own
// /swapfile never counts, and what it runs and writes is reported back with
// /swapfile again to keep the tests readable.
func newFakeSwap(t *testing.T, swaps, fstab string) *fakeSwap {
	t.Helper()
	f := &fakeSwap{t: t, dir: t.TempDir(), written: map[string]string{}, fail: map[string]bool{},
		freeKiB: 100 << 20, fsType: "ext2/ext3"}
	oldPaths := []string{swapsPath, meminfoPath, fstabPath, swapfilePath, swappinessPath, swappinessConfPath}
	oldRun, oldWrite, oldFree, oldFS := swapRun, swapWrite, swapFreeKiB, swapFSType
	t.Cleanup(func() {
		swapsPath, meminfoPath, fstabPath, swapfilePath, swappinessPath, swappinessConfPath =
			oldPaths[0], oldPaths[1], oldPaths[2], oldPaths[3], oldPaths[4], oldPaths[5]
		swapRun, swapWrite, swapFreeKiB, swapFSType = oldRun, oldWrite, oldFree, oldFS
	})
	swapfilePath = filepath.Join(f.dir, "swapfile")
	swapsPath = f.write("swaps", swapsHeader+swaps)
	fstabPath = f.write("fstab", fstab)
	meminfoPath = f.write("meminfo", "MemTotal:        1000000 kB\nMemAvailable:     600000 kB\n")
	swappinessPath = f.write("swappiness", "60\n")
	swappinessConfPath = "/etc/sysctl.d/zz-ctdev-swappiness.conf"
	swapRun = func(_ context.Context, _ sysutil.Opts, name string, args ...string) error {
		f.ran = append(f.ran, f.readable(strings.Join(append([]string{name}, args...), " ")))
		if f.fail[name] {
			return errors.New(name + " failed")
		}
		return nil
	}
	swapWrite = func(_ context.Context, _ sysutil.Opts, content, path, _ string) error {
		f.written[f.readable(path)] = f.readable(content)
		if path == fstabPath {
			// Later reads see it, as they would the real fstab.
			return os.WriteFile(path, []byte(content), 0o644)
		}
		return nil
	}
	swapFreeKiB = func(context.Context, string) (int64, error) { return f.freeKiB, nil }
	swapFSType = func(context.Context, string) (string, error) { return f.fsType, nil }
	return f
}

// write puts a fixture in the temp dir, with /swapfile pointed at the fake.
func (f *fakeSwap) write(name, content string) string {
	f.t.Helper()
	p := filepath.Join(f.dir, name)
	if err := os.WriteFile(p, []byte(strings.ReplaceAll(content, "/swapfile", swapfilePath)), 0o644); err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *fakeSwap) readable(s string) string {
	s = strings.ReplaceAll(s, swapfilePath, "/swapfile")
	return strings.ReplaceAll(s, filepath.Join(f.dir, "fstab"), "/etc/fstab")
}

// makeSwapfile puts a file at the fake /swapfile, as a hand-made one would be.
func (f *fakeSwap) makeSwapfile() { f.write("swapfile", "not really swap") }

func (f *fakeSwap) commands() string { return strings.Join(f.ran, "\n") }

const (
	baseFstab     = "UUID=example-root / ext4 defaults 0 1\n"
	handFstab     = baseFstab + "/swapfile none swap sw 0 0\n"
	ctdevFstab    = baseFstab + swapFstabMarker + "\n/swapfile none swap sw 0 0\n"
	partFstab     = baseFstab + "UUID=example-swap none swap sw 0 0\n"
	handSwapfile8 = "/swapfile                               file\t\t8388604\t\t0\t\t-2\n"
	ctdevSwap1    = "/swapfile                               file\t\t1048572\t\t0\t\t-2\n"
	partitionSwap = "/dev/nvme0n1p3                          partition\t4194300\t\t0\t\t-2\n"
)

func swapSettings(t *testing.T) []Setting {
	t.Helper()
	var out []Setting
	for _, s := range FilterBySlug(Registry, "storage") {
		if s.Name == "Swapfile size" || s.Name == "Swappiness" {
			out = append(out, s)
		}
	}
	if len(out) != 2 {
		t.Fatalf("found %d swap settings in storage, want 2", len(out))
	}
	return out
}

// batchApplies reports which swap settings --batch (with or without --force)
// would apply.
func batchApplies(t *testing.T) []string {
	t.Helper()
	var names []string
	for _, st := range InitStates(context.Background(), swapSettings(t)) {
		if st.NeedsApply(false) || st.NeedsApply(true) {
			names = append(names, st.Setting.Name)
		}
	}
	return names
}

// The owner's desktop: an 8 GiB /swapfile and fstab line made by hand. ctdev
// reports it and --batch leaves it, and swappiness, alone.
func TestSwapHandMadeSwapfileIsLeftAlone(t *testing.T) {
	f := newFakeSwap(t, handSwapfile8, handFstab)
	f.makeSwapfile()
	if got := detectSwapfileSize(context.Background()); got != "8" {
		t.Errorf("detectSwapfileSize = %q, want 8", got)
	}
	if why := swapfileNotNeeded(context.Background()); !strings.Contains(f.readable(why), "/swapfile, 8 GiB") {
		t.Errorf("swapfileNotNeeded = %q, want it to name /swapfile, 8 GiB", why)
	}
	if got := batchApplies(t); len(got) != 0 {
		t.Errorf("--batch would apply %v on a machine with its own swap", got)
	}
	// Changing it anyway starts from the 8 GiB it has, not the 1 GiB default.
	for _, st := range InitStates(context.Background(), swapSettings(t)) {
		if st.DesiredValue != st.CurrentValue {
			t.Errorf("%s: desired %q, want the current %q", st.Setting.Name, st.DesiredValue, st.CurrentValue)
		}
	}
}

// M1: a hand-made swapfile that isn't switched on, or any other file at
// /swapfile, is still the person's.
func TestSwapInactiveHandMadeSwapfileIsLeftAlone(t *testing.T) {
	for name, fstab := range map[string]string{"with its fstab line": handFstab, "no fstab line": baseFstab} {
		t.Run(name, func(t *testing.T) {
			f := newFakeSwap(t, "", fstab)
			f.makeSwapfile()
			if why := f.readable(swapfileNotNeeded(context.Background())); !strings.Contains(why, "/swapfile, not switched on") {
				t.Errorf("swapfileNotNeeded = %q, want it to report the file", why)
			}
			if got := batchApplies(t); len(got) != 0 {
				t.Errorf("--batch would apply %v", got)
			}
			if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "1"); err == nil {
				t.Error("applySwapfileSize replaced a file ctdev didn't create; want an error")
			}
			if len(f.ran) != 0 || len(f.written) != 0 {
				t.Errorf("ran %v, wrote %v; want nothing", f.ran, f.written)
			}
		})
	}
}

// M1: a swap entry in fstab that isn't switched on (a partition not yet
// active) counts as swap too.
func TestSwapFstabEntryIsLeftAlone(t *testing.T) {
	newFakeSwap(t, "", partFstab)
	if why := swapfileNotNeeded(context.Background()); !strings.Contains(why, "UUID=example-swap, in /etc/fstab") {
		t.Errorf("swapfileNotNeeded = %q, want it to name the fstab entry", why)
	}
	if got := batchApplies(t); len(got) != 0 {
		t.Errorf("--batch would apply %v", got)
	}
}

func TestSwapFstabCommentDoesNotCount(t *testing.T) {
	newFakeSwap(t, "", baseFstab+"#/dev/sda2 none swap sw 0 0\n# /swapfile was here\n")
	if why := swapfileNotNeeded(context.Background()); why != "" {
		t.Errorf("swapfileNotNeeded = %q from commented lines, want \"\"", why)
	}
}

func TestSwapPartitionIsLeftAlone(t *testing.T) {
	newFakeSwap(t, partitionSwap, baseFstab)
	if got := detectSwapfileSize(context.Background()); got != "0" {
		t.Errorf("detectSwapfileSize = %q, want 0 (a partition isn't a swapfile)", got)
	}
	if why := swapfileNotNeeded(context.Background()); !strings.Contains(why, "/dev/nvme0n1p3, 4 GiB") {
		t.Errorf("swapfileNotNeeded = %q, want it to name the partition", why)
	}
}

// M2: --batch never resizes ctdev's own swapfile, but still sets swappiness.
func TestSwapBatchDoesNotResizeCtdevsSwapfile(t *testing.T) {
	newFakeSwap(t, ctdevSwap1+partitionSwap, ctdevFstab)
	if why := swapfileNotNeeded(context.Background()); !strings.Contains(why, "doesn't resize") {
		t.Errorf("swapfileNotNeeded = %q on ctdev's own swapfile, want the no-resize reason", why)
	}
	if why := swappinessNotNeeded(context.Background()); why != "" {
		t.Errorf("swappinessNotNeeded = %q on ctdev's own swapfile, want \"\"", why)
	}
	if got := batchApplies(t); len(got) != 1 || got[0] != "Swappiness" {
		t.Errorf("--batch would apply %v, want just Swappiness", got)
	}
}

// requestedSwapfile runs InitStates and RequestSwapfileSize as `configure
// storage --batch --swapfile-size gib` does, and returns the swapfile's state.
func requestedSwapfile(t *testing.T, gib int) SettingState {
	t.Helper()
	states := InitStates(context.Background(), swapSettings(t))
	if err := RequestSwapfileSize(states, gib); err != nil {
		t.Fatalf("RequestSwapfileSize(%d) = %v", gib, err)
	}
	for _, st := range states {
		if st.Setting.Name == SwapfileSizeSetting {
			return st
		}
	}
	t.Fatal("no swapfile setting")
	return SettingState{}
}

// A server's boot script asks for 2 GiB where ctdev made 1: --swapfile-size
// resizes ctdev's own swapfile, which the default never does.
func TestRequestSwapfileSizeResizesCtdevsSwapfile(t *testing.T) {
	newFakeSwap(t, ctdevSwap1, ctdevFstab)
	st := requestedSwapfile(t, 2)
	if !st.NeedsApply(false) || st.DesiredValue != "2" {
		t.Errorf("state %+v, want 1 -> 2 applied", st)
	}
}

func TestRequestSwapfileSizeSameSizeChangesNothing(t *testing.T) {
	newFakeSwap(t, ctdevSwap1, ctdevFstab)
	if st := requestedSwapfile(t, 1); st.NeedsApply(false) {
		t.Errorf("state %+v, want nothing to apply at the size it already has", st)
	}
}

func TestRequestSwapfileSizeCreates(t *testing.T) {
	newFakeSwap(t, "", baseFstab)
	st := requestedSwapfile(t, 2)
	if !st.NeedsApply(false) || st.DesiredValue != "2" {
		t.Errorf("state %+v, want a 2 GiB swapfile created", st)
	}
}

// Swap ctdev didn't set up stays the person's, whatever size is asked for.
func TestRequestSwapfileSizeLeavesOtherSwapAlone(t *testing.T) {
	f := newFakeSwap(t, handSwapfile8, handFstab)
	f.makeSwapfile()
	st := requestedSwapfile(t, 2)
	if st.NeedsApply(false) || st.NeedsApply(true) || st.DesiredValue != "8" {
		t.Errorf("state %+v, want the hand-made 8 GiB swapfile left alone", st)
	}
}

func TestRequestSwapfileSizeRejectsOutOfRange(t *testing.T) {
	newFakeSwap(t, "", baseFstab)
	for _, gib := range []int{-1, 33} {
		states := InitStates(context.Background(), swapSettings(t))
		if err := RequestSwapfileSize(states, gib); err == nil {
			t.Errorf("RequestSwapfileSize(%d) = nil, want an out-of-range error", gib)
		}
	}
}

// ctdev's fstab line with no swapfile switched on (deleted, say): --batch
// recreates it.
func TestSwapCtdevsMissingSwapfileIsRecreated(t *testing.T) {
	newFakeSwap(t, "", ctdevFstab)
	if why := swapfileNotNeeded(context.Background()); why != "" {
		t.Errorf("swapfileNotNeeded = %q, want \"\"", why)
	}
}

func TestSwapNoSwapIsNeeded(t *testing.T) {
	newFakeSwap(t, "", baseFstab)
	if why := swapfileNotNeeded(context.Background()); why != "" {
		t.Errorf("swapfileNotNeeded = %q with no swap, want \"\"", why)
	}
	if got := batchApplies(t); len(got) != 2 {
		t.Errorf("--batch would apply %v, want both", got)
	}
}

func TestSwapUnsupportedFilesystem(t *testing.T) {
	f := newFakeSwap(t, "", baseFstab)
	f.fsType = "zfs"
	if why := swapfileNotNeeded(context.Background()); !strings.Contains(why, "zfs, which can't hold a swapfile") {
		t.Errorf("swapfileNotNeeded = %q, want the filesystem reason", why)
	}
	if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "1"); err == nil || !strings.Contains(err.Error(), "zfs") {
		t.Errorf("apply on zfs: err = %v, want a filesystem error", err)
	}
	if len(f.ran) != 0 {
		t.Errorf("ran %v on zfs; want nothing", f.ran)
	}
}

func TestApplySwapfileCreates(t *testing.T) {
	f := newFakeSwap(t, "", baseFstab)
	if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "1"); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"rm -f /swapfile",
		"fallocate -l 1G /swapfile",
		"chmod 600 /swapfile",
		"mkswap /swapfile",
		"cp -n /etc/fstab /etc/fstab.ctdev-backup",
		"swapon /swapfile",
	}, "\n")
	if got := f.commands(); got != want {
		t.Errorf("ran:\n%s\nwant:\n%s", got, want)
	}
	if got := f.written["/etc/fstab"]; got != ctdevFstab {
		t.Errorf("fstab = %q, want %q", got, ctdevFstab)
	}
}

func TestApplySwapfileFallsBackToDD(t *testing.T) {
	f := newFakeSwap(t, "", baseFstab)
	f.fail["fallocate"] = true
	if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "2"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.commands(), "dd if=/dev/zero of=/swapfile bs=1M count=2048 status=none\nchmod 600") {
		t.Errorf("ran:\n%s\nwant dd after fallocate failed", f.commands())
	}
}

func TestApplySwapfileBtrfs(t *testing.T) {
	t.Run("mkswapfile", func(t *testing.T) {
		f := newFakeSwap(t, "", baseFstab)
		f.fsType = "btrfs"
		if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "1"); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(f.commands(), "rm -f /swapfile\nbtrfs filesystem mkswapfile --size 1G /swapfile\ncp -n") {
			t.Errorf("ran:\n%s\nwant btrfs mkswapfile, then fstab", f.commands())
		}
	})
	t.Run("old btrfs-progs", func(t *testing.T) {
		f := newFakeSwap(t, "", baseFstab)
		f.fsType = "btrfs"
		f.fail["btrfs"] = true
		if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "1"); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(f.commands(), "truncate -s 0 /swapfile\nchattr +C /swapfile\nfallocate -l 1G /swapfile") {
			t.Errorf("ran:\n%s\nwant NOCOW before allocating", f.commands())
		}
	})
}

func TestApplySwapfileRightSizeIsLeftAlone(t *testing.T) {
	f := newFakeSwap(t, ctdevSwap1, ctdevFstab)
	f.makeSwapfile()
	if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "1"); err != nil {
		t.Fatal(err)
	}
	if len(f.ran) != 0 || len(f.written) != 0 {
		t.Errorf("right-size swapfile: ran %v, wrote %v; want nothing", f.ran, f.written)
	}
}

// Taking over a switched-on hand-made swapfile of the right size marks its
// fstab line as ctdev's without touching the file.
func TestApplySwapfileAdoptsRightSize(t *testing.T) {
	f := newFakeSwap(t, handSwapfile8, handFstab)
	f.makeSwapfile()
	if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "8"); err != nil {
		t.Fatal(err)
	}
	if got := f.commands(); got != "cp -n /etc/fstab /etc/fstab.ctdev-backup" {
		t.Errorf("adopting ran %q; want only the fstab backup", got)
	}
	if got := f.written["/etc/fstab"]; got != ctdevFstab {
		t.Errorf("fstab = %q, want %q", got, ctdevFstab)
	}
}

func TestApplySwapfileResizes(t *testing.T) {
	f := newFakeSwap(t, ctdevSwap1, ctdevFstab)
	f.makeSwapfile()
	if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "2"); err != nil {
		t.Fatal(err)
	}
	want := "swapoff /swapfile\nrm -f /swapfile\nfallocate -l 2G /swapfile\nchmod 600 /swapfile\nmkswap /swapfile\nswapon /swapfile"
	if got := f.commands(); got != want {
		t.Errorf("ran:\n%s\nwant:\n%s", got, want)
	}
	if len(f.written) != 0 {
		t.Errorf("fstab rewritten on resize though its line didn't change: %v", f.written)
	}
}

func TestApplySwapfileRemoves(t *testing.T) {
	f := newFakeSwap(t, ctdevSwap1, ctdevFstab)
	f.makeSwapfile()
	if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "0"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.ran[:2], "\n"); got != "swapoff /swapfile\nrm -f /swapfile" {
		t.Errorf("ran %v, want swapoff then rm", f.ran)
	}
	if got := f.written["/etc/fstab"]; got != baseFstab {
		t.Errorf("fstab = %q, want %q", got, baseFstab)
	}
}

func TestApplySwapfileRefusesSwapoffWithoutMemory(t *testing.T) {
	// 700 MB of swap in use with 600 MB available.
	f := newFakeSwap(t, "/swapfile file 1048572 700000 -2\n", ctdevFstab)
	f.makeSwapfile()
	err := applySwapfileSize(context.Background(), sysutil.Opts{}, "2")
	if err == nil || !strings.Contains(err.Error(), "exceeds free memory") {
		t.Fatalf("err = %v, want the swap-in-use error", err)
	}
	if len(f.ran) != 0 {
		t.Errorf("ran %v; want nothing switched off", f.ran)
	}
}

func TestApplySwapfileSwapoffFails(t *testing.T) {
	f := newFakeSwap(t, ctdevSwap1, ctdevFstab)
	f.makeSwapfile()
	f.fail["swapoff"] = true
	if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "2"); err == nil {
		t.Fatal("want an error when swapoff fails")
	}
	if got := f.commands(); got != "swapoff /swapfile" {
		t.Errorf("ran %q after swapoff failed; want nothing else", got)
	}
}

func TestApplySwapfileSwaponFailsRollsBack(t *testing.T) {
	f := newFakeSwap(t, "", baseFstab)
	f.fail["swapon"] = true
	if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "1"); err == nil {
		t.Fatal("want an error when swapon fails")
	}
	if !strings.Contains(f.commands(), "swapon /swapfile\nrm -f /swapfile") {
		t.Errorf("ran:\n%s\nwant the swapfile removed after swapon failed", f.commands())
	}
	if got := f.written["/etc/fstab"]; got != baseFstab {
		t.Errorf("fstab after the rollback = %q, want ctdev's line gone: %q", got, baseFstab)
	}
}

func TestApplySwapfileNotEnoughSpace(t *testing.T) {
	f := newFakeSwap(t, ctdevSwap1, ctdevFstab)
	f.makeSwapfile()
	// 4 GiB plus 512 MiB headroom, and only 4 GiB free.
	f.freeKiB = 4 << 20
	err := applySwapfileSize(context.Background(), sysutil.Opts{}, "4")
	if err == nil || !strings.Contains(err.Error(), "free") {
		t.Fatalf("err = %v, want a free-space error", err)
	}
	if len(f.ran) != 0 {
		t.Errorf("ran %v before failing the space check; want nothing switched off", f.ran)
	}
}

func TestApplySwapfileCleansUpFailedCreate(t *testing.T) {
	f := newFakeSwap(t, "", baseFstab)
	f.fail["mkswap"] = true
	if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "1"); err == nil {
		t.Fatal("want an error when mkswap fails")
	}
	if last := f.ran[len(f.ran)-1]; last != "rm -f /swapfile" {
		t.Errorf("last command = %q, want the half-made swapfile removed", last)
	}
	if len(f.written) != 0 {
		t.Errorf("fstab written after a failed create: %v", f.written)
	}
}

func TestApplySwapfileRejectsBadValue(t *testing.T) {
	newFakeSwap(t, "", baseFstab)
	for _, v := range []string{"-1", "0.5", "lots"} {
		if err := applySwapfileSize(context.Background(), sysutil.Opts{}, v); err == nil {
			t.Errorf("applySwapfileSize(%q) = nil, want an error", v)
		}
	}
}

func TestRenderSwapFstab(t *testing.T) {
	old := swapfilePath
	swapfilePath = "/swapfile"
	t.Cleanup(func() { swapfilePath = old })
	entry := swapFstabMarker + "\n/swapfile none swap sw 0 0\n"
	tests := []struct {
		name, in string
		keep     bool
		want     string
	}{
		{"adds the entry", baseFstab, true, baseFstab + entry},
		{"no trailing newline", strings.TrimSuffix(baseFstab, "\n"), true, baseFstab + entry},
		{"already there", baseFstab + entry, true, baseFstab + entry},
		{"replaces duplicates", baseFstab + "/swapfile none swap sw 0 0\n" + entry + "/swapfile none swap defaults 0 0\n", true, baseFstab + entry},
		{"keeps comments that mention it", baseFstab + "# /swapfile was 8G\n", true, baseFstab + "# /swapfile was 8G\n" + entry},
		{"CRLF stays CRLF", "UUID=x / ext4 defaults 0 1\r\n", true,
			"UUID=x / ext4 defaults 0 1\r\n" + strings.ReplaceAll(entry, "\n", "\r\n")},
		{"removes", baseFstab + entry, false, baseFstab},
		{"empty fstab", "", true, entry},
		{"empty stays empty", "", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderSwapFstab(tt.in, tt.keep); got != tt.want {
				t.Errorf("renderSwapFstab(%q, %v) =\n%q\nwant\n%q", tt.in, tt.keep, got, tt.want)
			}
		})
	}
}

func TestParseDfAvailKiB(t *testing.T) {
	good := "Filesystem     1024-blocks    Used Available Capacity Mounted on\n/dev/vda1         10218772 2311456   7366644      24% /\n"
	if got, err := parseDfAvailKiB(good); err != nil || got != 7366644 {
		t.Errorf("parseDfAvailKiB = %d, %v; want 7366644", got, err)
	}
	for _, bad := range []string{"", "Filesystem 1024-blocks Used Available Capacity Mounted on\n", "header\n/dev/vda1 1 2\n", "header\n/dev/vda1 1 2 lots 3% /\n"} {
		if _, err := parseDfAvailKiB(bad); err == nil {
			t.Errorf("parseDfAvailKiB(%q) = nil error, want one", bad)
		}
	}
}

func TestFormatGiB(t *testing.T) {
	for kib, want := range map[int64]string{8388604: "8", 1048572: "1", 1048560: "1", 524284: "0.5", 0: "0"} {
		if got := formatGiB(kib); got != want {
			t.Errorf("formatGiB(%d) = %q, want %q", kib, got, want)
		}
	}
}

func TestSwappiness(t *testing.T) {
	f := newFakeSwap(t, "", baseFstab)
	if got := detectSwappiness(context.Background()); got != "60" {
		t.Errorf("detectSwappiness = %q, want 60", got)
	}
	if err := applySwappiness(context.Background(), sysutil.Opts{}, "10"); err != nil {
		t.Fatal(err)
	}
	if conf := f.written[swappinessConfPath]; !strings.Contains(conf, "vm.swappiness = 10\n") {
		t.Errorf("drop-in = %q, want vm.swappiness = 10", conf)
	}
	if got := f.commands(); got != "sysctl -q -w vm.swappiness=10" {
		t.Errorf("ran %q, want sysctl -w", got)
	}
	if err := applySwappiness(context.Background(), sysutil.Opts{}, "101"); err == nil {
		t.Error("applySwappiness(101) = nil, want an error")
	}
}
