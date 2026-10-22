package setup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// fakeBlockDev describes one entry under a fake /sys/block.
type fakeBlockDev struct {
	name       string
	rotational string // contents of queue/rotational
	removable  string // contents of removable
	discardMax string // contents of queue/discard_max_bytes
	hardware   bool   // has a device link, as real disks do (loop and dm don't)
}

// fakeSysBlock builds a /sys/block lookalike in a temp dir and points
// sysBlockDir at it for the rest of the test.
func fakeSysBlock(t *testing.T, devs ...fakeBlockDev) {
	t.Helper()
	root := t.TempDir()
	for _, d := range devs {
		dir := filepath.Join(root, d.name)
		if err := os.MkdirAll(filepath.Join(dir, "queue"), 0o755); err != nil {
			t.Fatal(err)
		}
		write := func(rel, val string) {
			if val == "" {
				return
			}
			if err := os.WriteFile(filepath.Join(dir, rel), []byte(val+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		write("queue/rotational", d.rotational)
		write("removable", d.removable)
		write("queue/discard_max_bytes", d.discardMax)
		if d.hardware {
			if err := os.Mkdir(filepath.Join(dir, "device"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	old := sysBlockDir
	sysBlockDir = root
	t.Cleanup(func() { sysBlockDir = old })
}

var (
	nvmeSSD   = fakeBlockDev{name: "nvme0n1", rotational: "0", removable: "0", discardMax: "2199023255040", hardware: true}
	sataHDD   = fakeBlockDev{name: "sda", rotational: "1", removable: "0", discardMax: "0", hardware: true}
	loopDev   = fakeBlockDev{name: "loop0", rotational: "0", removable: "0", discardMax: "4294966784"}
	dmDev     = fakeBlockDev{name: "dm-0", rotational: "0", removable: "0", discardMax: "2199023255040"}
	usbStick  = fakeBlockDev{name: "sdb", rotational: "0", removable: "1", discardMax: "4294966784", hardware: true}
	noDiscard = fakeBlockDev{name: "sdc", rotational: "0", removable: "0", discardMax: "0", hardware: true}
)

func TestHasSSD(t *testing.T) {
	tests := []struct {
		name string
		devs []fakeBlockDev
		want bool
	}{
		{"nvme SSD", []fakeBlockDev{nvmeSSD}, true},
		{"SSD next to an HDD", []fakeBlockDev{sataHDD, loopDev, nvmeSSD}, true},
		{"HDD only", []fakeBlockDev{sataHDD}, false},
		{"loop and device-mapper don't count", []fakeBlockDev{loopDev, dmDev}, false},
		{"removable flash doesn't count", []fakeBlockDev{usbStick}, false},
		{"no discard support doesn't count", []fakeBlockDev{noDiscard}, false},
		{"no block devices", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeSysBlock(t, tt.devs...)
			if got := hasSSD(); got != tt.want {
				t.Errorf("hasSSD() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasSSDMissingSysfs(t *testing.T) {
	old := sysBlockDir
	sysBlockDir = filepath.Join(t.TempDir(), "absent")
	t.Cleanup(func() { sysBlockDir = old })
	if hasSSD() {
		t.Error("hasSSD() = true with no /sys/block")
	}
}

func trimSetting(t *testing.T) *Setting {
	t.Helper()
	for i := range Registry {
		if Registry[i].Name == "Keep SSDs fast (weekly TRIM)" {
			return &Registry[i]
		}
	}
	t.Fatal("TRIM setting not found in Registry")
	return nil
}

func TestTrimSettingIsInStorage(t *testing.T) {
	s := trimSetting(t)
	if s.Slug != "storage" {
		t.Errorf("TRIM setting slug = %q, want storage", s.Slug)
	}
	if s.OneWay {
		t.Error("TRIM setting is OneWay; it must offer an off switch")
	}
	if s.Control != ControlToggle || s.Default != "active" {
		t.Errorf("TRIM setting = control %v default %q, want toggle defaulting to active", s.Control, s.Default)
	}
	if strings.Contains(s.Description, "\n") || s.Description == "" {
		t.Errorf("TRIM setting needs a one-line why, got %q", s.Description)
	}
	for _, other := range FilterBySlug(Registry, "system") {
		if strings.Contains(strings.ToLower(other.Name), "trim") {
			t.Errorf("TRIM is still in the system category: %q", other.Name)
		}
	}
}

func TestTrimSettingShowsOnlyWithAnSSD(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("fstrim.timer is Linux-only")
	}
	s := trimSetting(t)

	fakeSysBlock(t, sataHDD, loopDev)
	if got := FilterByHardware(FilterBySlug(Registry, "storage")); len(got) != 0 {
		t.Errorf("storage shows %d settings on an HDD-only machine, want 0", len(got))
	}

	fakeSysBlock(t, sataHDD, nvmeSSD)
	if !s.HardwareFn() {
		t.Error("TRIM setting hidden on a machine with an SSD")
	}
	got := FilterByHardware(FilterBySlug(Registry, "storage"))
	if len(got) != 1 || got[0].Name != s.Name {
		t.Errorf("storage on an SSD machine = %v, want just the TRIM setting", got)
	}
}

func TestApplySSDTrim(t *testing.T) {
	tests := []struct {
		value string
		want  []string
	}{
		{"active", []string{"systemctl enable fstrim.timer", "systemctl start fstrim.timer"}},
		{"inactive", []string{"systemctl stop fstrim.timer", "systemctl disable fstrim.timer"}},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			var buf bytes.Buffer
			o := sysutil.Opts{Stdout: &buf, DryRun: true}
			if err := trimSetting(t).ApplyFunc(context.Background(), o, tt.value); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("apply %q ran:\n%s\nwant it to include %q", tt.value, out, w)
				}
			}
			if lines := strings.Count(out, "\n"); lines != len(tt.want) {
				t.Errorf("apply %q ran %d commands, want %d:\n%s", tt.value, lines, len(tt.want), out)
			}
		})
	}
}
