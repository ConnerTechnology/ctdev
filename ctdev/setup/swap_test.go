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
	dir     string
	ran     []string
	written map[string]string
	fail    map[string]bool // command names that fail
	freeKiB int64
}

func newFakeSwap(t *testing.T, swaps, fstab string) *fakeSwap {
	t.Helper()
	f := &fakeSwap{dir: t.TempDir(), written: map[string]string{}, fail: map[string]bool{}, freeKiB: 100 << 20}
	write := func(name, content string) string {
		p := filepath.Join(f.dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	oldPaths := []string{swapsPath, fstabPath, swapfilePath, swappinessPath, swappinessConfPath}
	oldRun, oldWrite, oldFree := swapRun, swapWrite, swapFreeKiB
	t.Cleanup(func() {
		swapsPath, fstabPath, swapfilePath, swappinessPath, swappinessConfPath =
			oldPaths[0], oldPaths[1], oldPaths[2], oldPaths[3], oldPaths[4]
		swapRun, swapWrite, swapFreeKiB = oldRun, oldWrite, oldFree
	})
	// A path in the temp dir, so the host's own /swapfile never counts. The
	// fixtures, recorded commands and writes say /swapfile to stay readable.
	swapfilePath = filepath.Join(f.dir, "swapfile")
	swapsPath = write("swaps", swapsHeader+strings.ReplaceAll(swaps, "/swapfile", swapfilePath))
	fstabPath = write("fstab", strings.ReplaceAll(fstab, "/swapfile", swapfilePath))
	swappinessPath = write("swappiness", "60\n")
	swappinessConfPath = "/etc/sysctl.d/99-ctdev-swappiness.conf"
	swapRun = func(_ context.Context, _ sysutil.Opts, name string, args ...string) error {
		f.ran = append(f.ran, strings.ReplaceAll(strings.Join(append([]string{name}, args...), " "), swapfilePath, "/swapfile"))
		if f.fail[name] {
			return errors.New(name + " failed")
		}
		return nil
	}
	swapWrite = func(_ context.Context, _ sysutil.Opts, content, path, _ string) error {
		f.written[path] = strings.ReplaceAll(content, swapfilePath, "/swapfile")
		return nil
	}
	swapFreeKiB = func(context.Context, string) (int64, error) { return f.freeKiB, nil }
	return f
}

const (
	baseFstab     = "UUID=example-root / ext4 defaults 0 1\n"
	handFstab     = baseFstab + "/swapfile none swap sw 0 0\n"
	ctdevFstab    = baseFstab + swapFstabMarker + "\n/swapfile none swap sw 0 0\n"
	handSwapfile8 = "/swapfile                               file\t\t8388604\t\t0\t\t-2\n"
	ctdevSwap1    = "/swapfile                               file\t\t1048572\t\t0\t\t-2\n"
	partitionSwap = "/dev/nvme0n1p3                          partition\t4194300\t\t0\t\t-2\n"
)

// The owner's desktop: an 8 GiB /swapfile and fstab line made by hand. ctdev
// reports it and --batch leaves it, and swappiness, alone.
func TestSwapHandMadeSwapfileIsLeftAlone(t *testing.T) {
	newFakeSwap(t, handSwapfile8, handFstab)
	ctx := context.Background()
	if got := detectSwapfileSize(ctx); got != "8" {
		t.Errorf("detectSwapfileSize = %q, want 8", got)
	}
	why := swapNotNeeded(ctx)
	if !strings.Contains(why, "/swapfile, 8 GiB") {
		t.Errorf("swapNotNeeded = %q, want it to name /swapfile, 8 GiB", why)
	}
	settings := FilterBySlug(Registry, "storage")
	var swapSettings []Setting
	for _, s := range settings {
		if s.ApplyFunc != nil && s.NotNeededFn != nil {
			swapSettings = append(swapSettings, s)
		}
	}
	if len(swapSettings) != 2 {
		t.Fatalf("found %d swap settings in storage, want 2", len(swapSettings))
	}
	for _, st := range InitStates(ctx, swapSettings) {
		if st.NeedsApply(false) || st.NeedsApply(true) {
			t.Errorf("%s would be applied by --batch on a machine with its own swap", st.Setting.Name)
		}
	}
}

func TestSwapPartitionIsLeftAlone(t *testing.T) {
	newFakeSwap(t, partitionSwap, baseFstab)
	if got := detectSwapfileSize(context.Background()); got != "0" {
		t.Errorf("detectSwapfileSize = %q, want 0 (a partition isn't a swapfile)", got)
	}
	if why := swapNotNeeded(context.Background()); !strings.Contains(why, "/dev/nvme0n1p3, 4 GiB") {
		t.Errorf("swapNotNeeded = %q, want it to name the partition", why)
	}
}

func TestSwapCtdevsSwapfileIsNeeded(t *testing.T) {
	newFakeSwap(t, ctdevSwap1+partitionSwap, ctdevFstab)
	if why := swapNotNeeded(context.Background()); why != "" {
		t.Errorf("swapNotNeeded = %q on ctdev's own swapfile, want \"\"", why)
	}
	if got := detectSwapfileSize(context.Background()); got != "1" {
		t.Errorf("detectSwapfileSize = %q, want 1", got)
	}
}

func TestSwapNoSwapIsNeeded(t *testing.T) {
	newFakeSwap(t, "", baseFstab)
	if why := swapNotNeeded(context.Background()); why != "" {
		t.Errorf("swapNotNeeded = %q with no swap, want \"\"", why)
	}
}

func TestApplySwapfileCreates(t *testing.T) {
	f := newFakeSwap(t, "", baseFstab)
	if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "1"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"rm -f /swapfile",
		"fallocate -l 1G /swapfile",
		"chmod 600 /swapfile",
		"mkswap /swapfile",
		"swapon /swapfile",
		"cp -n " + fstabPath + " " + fstabPath + ".ctdev-backup",
	}
	if strings.Join(f.ran, "\n") != strings.Join(want, "\n") {
		t.Errorf("ran:\n%s\nwant:\n%s", strings.Join(f.ran, "\n"), strings.Join(want, "\n"))
	}
	if got := f.written[fstabPath]; got != ctdevFstab {
		t.Errorf("fstab = %q, want %q", got, ctdevFstab)
	}
}

func TestApplySwapfileFallsBackToDD(t *testing.T) {
	f := newFakeSwap(t, "", baseFstab)
	f.fail["fallocate"] = true
	if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "2"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(f.ran, "\n"), "dd if=/dev/zero of=/swapfile bs=1M count=2048 status=none\nchmod 600") {
		t.Errorf("ran:\n%s\nwant dd after fallocate failed", strings.Join(f.ran, "\n"))
	}
}

func TestApplySwapfileRightSizeIsLeftAlone(t *testing.T) {
	f := newFakeSwap(t, ctdevSwap1, ctdevFstab)
	if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "1"); err != nil {
		t.Fatal(err)
	}
	if len(f.ran) != 0 || len(f.written) != 0 {
		t.Errorf("right-size swapfile: ran %v, wrote %v; want nothing", f.ran, f.written)
	}
}

// Adopting a hand-made swapfile of the right size marks its fstab line as
// ctdev's without touching the file.
func TestApplySwapfileAdoptsRightSize(t *testing.T) {
	f := newFakeSwap(t, handSwapfile8, handFstab)
	if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "8"); err != nil {
		t.Fatal(err)
	}
	for _, r := range f.ran {
		if !strings.HasPrefix(r, "cp -n ") {
			t.Errorf("adopting ran %q; want only the fstab backup", r)
		}
	}
	if got := f.written[fstabPath]; got != ctdevFstab {
		t.Errorf("fstab = %q, want %q", got, ctdevFstab)
	}
}

func TestApplySwapfileResizes(t *testing.T) {
	f := newFakeSwap(t, ctdevSwap1, ctdevFstab)
	if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "2"); err != nil {
		t.Fatal(err)
	}
	want := "swapoff /swapfile\nrm -f /swapfile\nfallocate -l 2G /swapfile\nchmod 600 /swapfile\nmkswap /swapfile\nswapon /swapfile"
	if got := strings.Join(f.ran, "\n"); got != want {
		t.Errorf("ran:\n%s\nwant:\n%s", got, want)
	}
	if len(f.written) != 0 {
		t.Errorf("fstab rewritten on resize though its line didn't change: %v", f.written)
	}
}

func TestApplySwapfileRemoves(t *testing.T) {
	f := newFakeSwap(t, ctdevSwap1, ctdevFstab)
	if err := applySwapfileSize(context.Background(), sysutil.Opts{}, "0"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.ran[:2], "\n"); got != "swapoff /swapfile\nrm -f /swapfile" {
		t.Errorf("ran %v, want swapoff then rm", f.ran)
	}
	if got := f.written[fstabPath]; got != baseFstab {
		t.Errorf("fstab = %q, want %q", got, baseFstab)
	}
}

func TestApplySwapfileNotEnoughSpace(t *testing.T) {
	f := newFakeSwap(t, ctdevSwap1, ctdevFstab)
	f.freeKiB = 1 << 20
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
	if got := strings.Join(f.ran, "\n"); got != "sysctl -q -w vm.swappiness=10" {
		t.Errorf("ran %q, want sysctl -w", got)
	}
	if err := applySwappiness(context.Background(), sysutil.Opts{}, "101"); err == nil {
		t.Error("applySwappiness(101) = nil, want an error")
	}
}
