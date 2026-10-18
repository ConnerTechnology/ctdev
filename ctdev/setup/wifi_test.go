package setup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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

func TestKeepWiFiConnectedHiddenOnWiredOnlyMachine(t *testing.T) {
	orig := sysClassNet
	t.Cleanup(func() { sysClassNet = orig })
	sysClassNet = fakeSysClassNet(t, []string{"lo", "enp5s0"})

	visible := FilterByHardware(FilterBySlug(Registry, "network"))
	if s := findSetting(t, visible, keepWiFiConnectedName); s != nil {
		t.Errorf("%q shown on a wired-only machine", keepWiFiConnectedName)
	}
	if findSetting(t, Registry, "WiFi power save off") != nil {
		t.Error(`old name "WiFi power save off" still in the registry`)
	}
}

func TestKeepWiFiConnectedHasOffSwitch(t *testing.T) {
	s := findSetting(t, Registry, keepWiFiConnectedName)
	if s == nil {
		t.Fatalf("%q not in the registry", keepWiFiConnectedName)
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
	orig := sysClassNet
	t.Cleanup(func() { sysClassNet = orig })
	sysClassNet = fakeSysClassNet(t, []string{"lo", "enp5s0"}, "wlp4s0")

	visible := FilterByHardware(FilterBySlug(Registry, "network"))
	shown := findSetting(t, visible, keepWiFiConnectedName) != nil
	// NetworkManager is still required; only assert where the host has it.
	if shown != gateNetworkManager() {
		t.Errorf("shown = %v with a WiFi adapter, want %v (NetworkManager present)", shown, gateNetworkManager())
	}
}
