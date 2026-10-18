package setup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// fakeHideDrives stands in for lsblk, the root file write and the root
// commands, and points the rule path at a temp dir.
type fakeHideDrives struct {
	lsblk  string
	path   string
	writes map[string]string // path → content
	runs   [][]string
}

func useFakeHideDrives(t *testing.T, lsblk string) *fakeHideDrives {
	t.Helper()
	f := &fakeHideDrives{
		lsblk:  lsblk,
		path:   filepath.Join(t.TempDir(), "99-hide-drives.rules"),
		writes: map[string]string{},
	}
	origRead, origWrite, origRun, origPath := lsblkRead, hideDrivesWrite, hideDrivesRun, hideDrivesRulePath
	lsblkRead = func(context.Context) ([]byte, error) {
		if f.lsblk == "" {
			return nil, errors.New("lsblk: not found")
		}
		return []byte(f.lsblk), nil
	}
	hideDrivesWrite = func(_ context.Context, _ sysutil.Opts, content, path string) error {
		f.writes[path] = content
		return os.WriteFile(path, []byte(content), 0o644)
	}
	hideDrivesRun = func(_ context.Context, _ sysutil.Opts, name string, args ...string) error {
		f.runs = append(f.runs, append([]string{name}, args...))
		if name == "rm" {
			return os.Remove(args[len(args)-1])
		}
		return nil
	}
	hideDrivesRulePath = f.path
	t.Cleanup(func() {
		lsblkRead, hideDrivesWrite, hideDrivesRun, hideDrivesRulePath = origRead, origWrite, origRun, origPath
	})
	return f
}

// fakeLsblk is `lsblk -J -l -o NAME,UUID,FSTYPE,LABEL,SIZE,MOUNTPOINT,TYPE`
// on a synthetic two-disk machine.
const fakeLsblk = `{
   "blockdevices": [
      {"name":"sda", "uuid":null, "fstype":null, "label":null, "size":"100G", "mountpoint":null, "type":"disk"},
      {"name":"sda1", "uuid":"AAAA-1111", "fstype":"vfat", "label":null, "size":"512M", "mountpoint":"/boot/efi", "type":"part"},
      {"name":"sda2", "uuid":"11111111-2222-3333-4444-555555555555", "fstype":"ext4", "label":null, "size":"99G", "mountpoint":"/", "type":"part"},
      {"name":"sdb", "uuid":null, "fstype":null, "label":null, "size":"200G", "mountpoint":null, "type":"disk"},
      {"name":"sdb1", "uuid":null, "fstype":null, "label":null, "size":"16M", "mountpoint":null, "type":"part"},
      {"name":"sdb2", "uuid":"0123456789ABCDEF", "fstype":"ntfs", "label":"Example Windows", "size":"199G", "mountpoint":null, "type":"part"},
      {"name":"sr0", "uuid":"2026-01-01-00-00-00-00", "fstype":"iso9660", "label":"EXAMPLE", "size":"1G", "mountpoint":null, "type":"rom"}
   ]
}`

func TestHideDrivesChoicesListsPartitionsWithUUID(t *testing.T) {
	useFakeHideDrives(t, fakeLsblk)
	got := hideDrivesChoices(context.Background())
	var uuids []string
	for _, c := range got {
		uuids = append(uuids, c.Value)
	}
	want := []string{"AAAA-1111", "11111111-2222-3333-4444-555555555555", "0123456789ABCDEF"}
	if !reflect.DeepEqual(uuids, want) {
		t.Fatalf("choices = %v, want %v (partitions with a filesystem UUID, in lsblk order)", uuids, want)
	}
	if d := got[2].Description; !strings.Contains(d, "/dev/sdb2") || !strings.Contains(d, "ntfs") ||
		!strings.Contains(d, "199G") || !strings.Contains(d, "Example Windows") {
		t.Errorf("sdb2 description = %q, want device, filesystem, size and label", d)
	}
	if d := got[1].Description; !strings.Contains(d, "mounted at /") {
		t.Errorf("sda2 description = %q, want its mount point", d)
	}
}

func TestHideDrivesChoicesNoLsblk(t *testing.T) {
	useFakeHideDrives(t, "")
	if got := hideDrivesChoices(context.Background()); len(got) != 0 {
		t.Errorf("choices without lsblk = %v, want none", got)
	}
}

func TestApplyHideDrivesWritesOnlyCheckedUUIDs(t *testing.T) {
	f := useFakeHideDrives(t, fakeLsblk)
	ctx := context.Background()
	var buf bytes.Buffer
	if err := applyHideDrives(ctx, sysutil.Opts{Stdout: &buf}, "0123456789ABCDEF,AAAA-1111"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	content, ok := f.writes[f.path]
	if !ok {
		t.Fatalf("no rule written to %s; writes %v", f.path, f.writes)
	}
	var rules []string
	for _, line := range strings.Split(content, "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			rules = append(rules, line)
		}
	}
	want := []string{
		`ENV{ID_FS_UUID}=="0123456789ABCDEF", ENV{UDISKS_IGNORE}="1"`,
		`ENV{ID_FS_UUID}=="AAAA-1111", ENV{UDISKS_IGNORE}="1"`,
	}
	if !reflect.DeepEqual(rules, want) {
		t.Errorf("rule lines = %q, want %q", rules, want)
	}
	if strings.Contains(content, "DEVNAME") {
		t.Errorf("rule keys on device names: %q", content)
	}
	if !hasRun(f.runs, "udevadm", "control", "--reload-rules") {
		t.Errorf("udev rules not reloaded; runs %v", f.runs)
	}
	if got := detectHideDrives(ctx); got != "0123456789ABCDEF,AAAA-1111" {
		t.Errorf("detect after apply = %q", got)
	}
}

func TestApplyHideDrivesNothingCheckedRemovesRule(t *testing.T) {
	f := useFakeHideDrives(t, fakeLsblk)
	ctx := context.Background()
	o := sysutil.Opts{Stdout: &bytes.Buffer{}}
	if err := applyHideDrives(ctx, o, "AAAA-1111"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	f.runs = nil
	if err := applyHideDrives(ctx, o, hideDrivesNone); err != nil {
		t.Fatalf("apply none: %v", err)
	}
	if !hasRun(f.runs, "rm", "-f", f.path) {
		t.Errorf("rule not removed; runs %v", f.runs)
	}
	if _, err := os.Stat(f.path); !os.IsNotExist(err) {
		t.Errorf("rule file still present: %v", err)
	}
	if !hasRun(f.runs, "udevadm", "control", "--reload-rules") {
		t.Errorf("udev rules not reloaded; runs %v", f.runs)
	}
	if got := detectHideDrives(ctx); got != hideDrivesNone {
		t.Errorf("detect after removal = %q, want %q", got, hideDrivesNone)
	}
}

func TestApplyHideDrivesNothingCheckedNoRuleIsNoOp(t *testing.T) {
	f := useFakeHideDrives(t, fakeLsblk)
	if err := applyHideDrives(context.Background(), sysutil.Opts{Stdout: &bytes.Buffer{}}, hideDrivesNone); err != nil {
		t.Fatalf("apply none: %v", err)
	}
	if len(f.runs) != 0 || len(f.writes) != 0 {
		t.Errorf("nothing to remove, but ran %v and wrote %v", f.runs, f.writes)
	}
}

func TestApplyHideDrivesRejectsBadUUID(t *testing.T) {
	f := useFakeHideDrives(t, fakeLsblk)
	err := applyHideDrives(context.Background(), sysutil.Opts{Stdout: &bytes.Buffer{}}, `AAAA-1111,x", RUN+="/bin/sh`)
	if err == nil {
		t.Fatal("apply accepted a value that is not a filesystem UUID")
	}
	if len(f.writes) != 0 {
		t.Errorf("wrote a rule despite a bad UUID: %v", f.writes)
	}
}

func TestDetectHideDrives(t *testing.T) {
	f := useFakeHideDrives(t, fakeLsblk)
	ctx := context.Background()
	if got := detectHideDrives(ctx); got != hideDrivesNone {
		t.Errorf("no rule: detect = %q, want %q", got, hideDrivesNone)
	}
	// The rule ctdev embedded before CTD-94: one UUID plus device names.
	legacy := "# Hide Windows/secondary partitions from the file manager\n" +
		`ENV{ID_FS_UUID}=="0123456789ABCDEF", ENV{UDISKS_IGNORE}="1"` + "\n" +
		`ENV{DEVNAME}=="/dev/nvme1n1p1", ENV{UDISKS_IGNORE}="1"` + "\n"
	if err := os.WriteFile(f.path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := detectHideDrives(ctx); got != hideDrivesLegacy {
		t.Errorf("legacy rule: detect = %q, want %q", got, hideDrivesLegacy)
	}
}

func TestPickValueRoundTrip(t *testing.T) {
	if got := JoinPicks(nil); got != hideDrivesNone {
		t.Errorf("JoinPicks(nil) = %q", got)
	}
	if got := JoinPicks([]string{"b", "a", "b"}); got != "a,b" {
		t.Errorf("JoinPicks sorts and dedupes: got %q", got)
	}
	if got := SplitPicks(hideDrivesNone); len(got) != 0 {
		t.Errorf("SplitPicks(none) = %v", got)
	}
	if got := SplitPicks("a,b"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("SplitPicks(a,b) = %v", got)
	}
}

func TestRegistryHideDrives(t *testing.T) {
	var found *Setting
	for i := range Registry {
		if Registry[i].Name == "Hide drives" {
			found = &Registry[i]
		}
	}
	if found == nil {
		t.Fatal("Hide drives not in Registry")
	}
	s := found
	if s.Slug != "desktop" || s.Control != ControlMultiPick || s.OneWay {
		t.Errorf("Hide drives: slug %q control %v oneway %v", s.Slug, s.Control, s.OneWay)
	}
	if s.Default != "" {
		t.Errorf("Hide drives default = %q; partitions are per machine, so there is no recommendation", s.Default)
	}
	if s.ChoicesFunc == nil || s.DetectFunc == nil || s.ApplyFunc == nil {
		t.Error("Hide drives needs ChoicesFunc, DetectFunc and ApplyFunc")
	}
	if funcPtr(s.HardwareFn) != funcPtr(gateCinnamon) {
		t.Error("Hide drives should be gated to Cinnamon")
	}
}

func TestInitStatesNoDefaultKeepsCurrent(t *testing.T) {
	settings := []Setting{{
		Name:       "Pick",
		Control:    ControlMultiPick,
		DetectFunc: func(context.Context) string { return "a,b" },
	}}
	states := InitStates(context.Background(), settings)
	if states[0].DesiredValue != "a,b" {
		t.Errorf("desired = %q; a setting with no recommendation should start at its current value", states[0].DesiredValue)
	}
	if states[0].NeedsApply(false) {
		t.Error("--batch would change a setting that has no recommendation")
	}
}

func hasRun(runs [][]string, want ...string) bool {
	for _, r := range runs {
		if reflect.DeepEqual(r, want) {
			return true
		}
	}
	return false
}
