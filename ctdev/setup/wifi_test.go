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

// fakeSysClassNet builds a fake /sys/class/net with one directory per
// interface. Interfaces listed in wireless get the "wireless" subdirectory the
// kernel adds to WiFi adapters.
func fakeSysClassNet(t *testing.T, ifaces []string, wireless ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range ifaces {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range wireless {
		if err := os.MkdirAll(filepath.Join(root, name, "wireless"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestHasWiFiAdapter(t *testing.T) {
	tests := []struct {
		name     string
		ifaces   []string
		wireless []string
		want     bool
	}{
		{"wired only", []string{"lo", "eth0", "enp5s0"}, nil, false},
		{"no interfaces", nil, nil, false},
		{"wifi adapter", []string{"lo", "enp5s0"}, []string{"wlp4s0"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := fakeSysClassNet(t, tt.ifaces, tt.wireless...)
			if got := hasWiFiAdapter(root); got != tt.want {
				t.Errorf("hasWiFiAdapter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasWiFiAdapterMissingDir(t *testing.T) {
	if hasWiFiAdapter(filepath.Join(t.TempDir(), "absent")) {
		t.Error("expected false when the net class directory doesn't exist")
	}
}

func findSetting(t *testing.T, settings []Setting, name string) *Setting {
	t.Helper()
	for i := range settings {
		if settings[i].Name == name {
			return &settings[i]
		}
	}
	return nil
}

// fakeNetworkManagerHost points the NetworkManager and network interface
// gates at fake trees: NetworkManager present, with the given interfaces.
// The gates are Linux-only, so the test is skipped elsewhere.
func fakeNetworkManagerHost(t *testing.T, ifaces []string, wireless ...string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the NetworkManager gate is Linux-only")
	}
	origNM, origNet := etcNetworkManager, sysClassNet
	t.Cleanup(func() { etcNetworkManager, sysClassNet = origNM, origNet })
	etcNetworkManager = t.TempDir()
	sysClassNet = fakeSysClassNet(t, ifaces, wireless...)
}

func TestKeepWiFiConnectedHiddenOnWiredOnlyMachine(t *testing.T) {
	fakeNetworkManagerHost(t, []string{"lo", "enp5s0"})

	visible := FilterByHardware(FilterBySlug(Registry, "network"))
	if s := findSetting(t, visible, "Keep WiFi connected when idle"); s != nil {
		t.Error(`"Keep WiFi connected when idle" shown on a wired-only machine`)
	}
	if findSetting(t, Registry, "WiFi power save off") != nil {
		t.Error(`old name "WiFi power save off" still in the registry`)
	}
}

func TestKeepWiFiConnectedHasOffSwitch(t *testing.T) {
	s := findSetting(t, Registry, "Keep WiFi connected when idle")
	if s == nil {
		t.Fatal(`"Keep WiFi connected when idle" not in the registry`)
	}
	if s.OneWay {
		t.Error("expected an off switch, but the setting is marked OneWay")
	}
	if strings.Contains(s.Description, "\n") || s.Description == "" {
		t.Errorf("want a one-line why, got %q", s.Description)
	}

	var out bytes.Buffer
	o := sysutil.Opts{Stdout: &out, DryRun: true}
	if err := s.ApplyFunc(context.Background(), o, "not installed"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "rm -f "+wifiPowersaveConfPath) {
		t.Errorf("turning it off should remove %s, got %q", wifiPowersaveConfPath, out.String())
	}

	out.Reset()
	if err := s.ApplyFunc(context.Background(), o, "installed"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "write "+wifiPowersaveConfPath) {
		t.Errorf("turning it on should write %s, got %q", wifiPowersaveConfPath, out.String())
	}
}

func TestKeepWiFiConnectedShownWithWiFiAdapter(t *testing.T) {
	fakeNetworkManagerHost(t, []string{"lo", "enp5s0"}, "wlp4s0")

	visible := FilterByHardware(FilterBySlug(Registry, "network"))
	if findSetting(t, visible, "Keep WiFi connected when idle") == nil {
		t.Error(`"Keep WiFi connected when idle" hidden on a machine with a WiFi adapter`)
	}
}
