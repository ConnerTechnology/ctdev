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

// fakeAudioHost makes this a Linux Cinnamon desktop with a sound card and,
// when bluetooth is true, a Bluetooth adapter. No package is installed and
// bluetooth.service is disabled and stopped. It returns the directory where
// ctdev records what it added.
func fakeAudioHost(t *testing.T, bluetooth bool) string {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the audio and bluetooth gates are Linux-only")
	}
	t.Setenv("XDG_CURRENT_DESKTOP", "X-Cinnamon")

	sound := t.TempDir()
	if err := os.Mkdir(filepath.Join(sound, "card0"), 0o755); err != nil {
		t.Fatal(err)
	}
	bt := t.TempDir()
	if bluetooth {
		if err := os.Mkdir(filepath.Join(bt, "hci0"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	added := t.TempDir()

	origSound, origBT, origPkg, origCtl, origAdded := sysClassSound, sysClassBluetooth, packageInstalled, systemctlQuery, addedDir
	t.Cleanup(func() {
		sysClassSound, sysClassBluetooth, packageInstalled, systemctlQuery, addedDir = origSound, origBT, origPkg, origCtl, origAdded
	})
	sysClassSound, sysClassBluetooth = sound, bt
	packageInstalled = func(context.Context, string) bool { return false }
	systemctlQuery = func(_ context.Context, verb, _ string) string {
		if verb == "is-enabled" {
			return "disabled"
		}
		return "inactive"
	}
	addedDir = func() string { return added }
	return added
}

// fakeRoot puts logging stand-ins for sudo and the commands ctdev runs as root
// first on PATH, so apply paths run for real without touching the machine,
// root or not. It returns a function reading the logged command lines.
func fakeRoot(t *testing.T) func() string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "log")
	script := "#!/bin/sh\necho \"$(basename \"$0\") $*\" >> '" + log + "'\n"
	for _, name := range []string{"sudo", "apt-get", "systemctl", "rm", "mkdir", "cp"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() string {
		data, _ := os.ReadFile(log)
		return string(data)
	}
}

// applyCategory runs a category the way `configure <slug> --batch` does:
// visible settings, detected, applied in registry order where they differ
// from the recommended value.
func applyCategory(t *testing.T, slug string, o sysutil.Opts) {
	t.Helper()
	states := InitStates(context.Background(), FilterByHardware(FilterBySlug(Registry, slug)))
	for i := range states {
		if !states[i].NeedsApply(false) {
			continue
		}
		if err := states[i].Setting.ApplyFunc(context.Background(), o, states[i].DesiredValue); err != nil {
			t.Fatalf("%s: %v", states[i].Setting.Name, err)
		}
	}
}

func settingByName(t *testing.T, name string) *Setting {
	t.Helper()
	for i := range Registry {
		if Registry[i].Name == name {
			return &Registry[i]
		}
	}
	t.Fatalf("%q not in the registry", name)
	return nil
}

func TestConfigureBluetoothAloneInstallsBluezThenEnablesService(t *testing.T) {
	fakeAudioHost(t, true)
	logged := fakeRoot(t)

	applyCategory(t, "bluetooth", sysutil.Opts{Stdout: &bytes.Buffer{}})

	out := logged()
	install := strings.Index(out, "apt-get install -y -qq --no-install-recommends bluez blueman")
	enable := strings.Index(out, "systemctl enable bluetooth.service")
	start := strings.Index(out, "systemctl start bluetooth.service")
	if install < 0 || enable < 0 || start < 0 {
		t.Fatalf("want bluez and blueman installed, then the service enabled and started; ran:\n%s", out)
	}
	if install > enable || enable > start {
		t.Errorf("want install, then enable, then start; ran:\n%s", out)
	}
	for _, p := range append(append([]string{}, soundSystemPackages...), bluetoothAudioPackages...) {
		if strings.Contains(out, p) {
			t.Errorf("configure bluetooth installed audio package %s:\n%s", p, out)
		}
	}
	if got := readAdded("bluetooth"); strings.Join(got, " ") != "bluez blueman" {
		t.Errorf("recorded packages = %v, want [bluez blueman]", got)
	}
	if got := readAdded("bluetooth-service"); strings.Join(got, " ") != "enable start" {
		t.Errorf("recorded service changes = %v, want [enable start]", got)
	}
}

func TestConfigureAudioInstallsNoBluetoothPackages(t *testing.T) {
	fakeAudioHost(t, true)
	logged := fakeRoot(t)

	applyCategory(t, "audio", sysutil.Opts{Stdout: &bytes.Buffer{}})

	out := logged()
	for _, p := range soundSystemPackages {
		if !strings.Contains(out, p) {
			t.Errorf("configure audio didn't install %s:\n%s", p, out)
		}
	}
	if !strings.Contains(out, "libspa-0.2-bluetooth") {
		t.Errorf("configure audio didn't install the Bluetooth codecs:\n%s", out)
	}
	for _, p := range bluetoothPackages {
		if strings.Contains(out, " "+p) {
			t.Errorf("configure audio installed Bluetooth package %s:\n%s", p, out)
		}
	}
	if strings.Contains(out, "bluetooth.service") {
		t.Errorf("configure audio touched bluetooth.service:\n%s", out)
	}
	if strings.Contains(out, "pipewire-audio") {
		t.Errorf("configure audio installed the pipewire-audio metapackage, which depends on the Bluetooth codecs:\n%s", out)
	}
}

func TestAudioAndBluetoothPackageSetsAreDisjoint(t *testing.T) {
	audio := map[string]bool{}
	for _, p := range append(append([]string{}, soundSystemPackages...), bluetoothAudioPackages...) {
		audio[p] = true
	}
	for _, p := range bluetoothPackages {
		if audio[p] {
			t.Errorf("%s is in both categories", p)
		}
	}
}

func TestBluetoothSettingsHiddenWithoutAdapter(t *testing.T) {
	fakeAudioHost(t, false)

	if got := FilterByHardware(FilterBySlug(Registry, "bluetooth")); len(got) != 0 {
		t.Errorf("bluetooth settings shown on a machine with no adapter: %d", len(got))
	}
	for _, s := range FilterByHardware(FilterBySlug(Registry, "audio")) {
		if strings.Contains(s.Name, "Bluetooth") {
			t.Errorf("%q shown on a machine with no Bluetooth adapter", s.Name)
		}
	}
}

func TestSoundSystemHiddenWithoutSoundCard(t *testing.T) {
	fakeAudioHost(t, true)
	sysClassSound = t.TempDir()

	for _, s := range FilterByHardware(FilterBySlug(Registry, "audio")) {
		if s.Name == "Sound system (PipeWire)" {
			t.Error("sound system shown on a machine with no sound card")
		}
	}
}

func TestAudioAndBluetoothSettingsHaveOffSwitchAndOneLineWhy(t *testing.T) {
	for _, slug := range []string{"audio", "bluetooth"} {
		for _, s := range FilterBySlug(Registry, slug) {
			if s.OneWay {
				t.Errorf("%q has no off switch", s.Name)
			}
			if s.Description == "" || strings.Contains(s.Description, "\n") {
				t.Errorf("%q: want a one-line why, got %q", s.Name, s.Description)
			}
		}
	}
}

func TestPackageSettingsOffRemovesOnlyWhatCtdevAdded(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		recorded []string
	}{
		{"Bluetooth support", "bluetooth", []string{"blueman"}},
		{"Sound system (PipeWire)", "sound-system", []string{"pipewire-alsa", "pulseaudio-utils"}},
		{"Bluetooth headphones", "bluetooth-audio", []string{"libspa-0.2-bluetooth"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeAudioHost(t, true)
			logged := fakeRoot(t)
			if err := recordAdded(sysutil.Opts{}, tt.key, tt.recorded...); err != nil {
				t.Fatal(err)
			}

			s := settingByName(t, tt.name)
			if err := s.ApplyFunc(context.Background(), sysutil.Opts{Stdout: &bytes.Buffer{}}, "not installed"); err != nil {
				t.Fatal(err)
			}

			want := "apt-get remove -y -qq " + strings.Join(tt.recorded, " ") + "\n"
			if out := logged(); !strings.HasSuffix(out, want) || strings.Count(out, "apt-get") != 1 {
				t.Errorf("want only %q removed, ran:\n%s", strings.TrimSpace(want), out)
			}
			if got := readAdded(tt.key); len(got) != 0 {
				t.Errorf("record not cleared after removal: %v", got)
			}
		})
	}
}

func TestPackageSettingOffLeavesPreexistingPackages(t *testing.T) {
	fakeAudioHost(t, true)
	logged := fakeRoot(t)
	var out bytes.Buffer

	s := settingByName(t, "Bluetooth support")
	if err := s.ApplyFunc(context.Background(), sysutil.Opts{Stdout: &out}, "not installed"); err != nil {
		t.Fatal(err)
	}
	if ran := logged(); ran != "" {
		t.Errorf("ctdev added nothing, so nothing should run; ran:\n%s", ran)
	}
	if !strings.Contains(out.String(), "didn't install") {
		t.Errorf("want a note that ctdev didn't install them, got %q", out.String())
	}
}

func TestPackageSettingRecordsOnlyMissingPackages(t *testing.T) {
	fakeAudioHost(t, true)
	logged := fakeRoot(t)
	packageInstalled = func(_ context.Context, p string) bool { return p == "bluez" }

	s := settingByName(t, "Bluetooth support")
	if err := s.ApplyFunc(context.Background(), sysutil.Opts{Stdout: &bytes.Buffer{}}, "installed"); err != nil {
		t.Fatal(err)
	}
	if out := logged(); !strings.Contains(out, "--no-install-recommends blueman\n") {
		t.Errorf("want only blueman installed, ran:\n%s", out)
	}
	if got := readAdded("bluetooth"); strings.Join(got, " ") != "blueman" {
		t.Errorf("recorded = %v, want [blueman]", got)
	}
}

func TestBluetoothServiceOffUndoesOnlyWhatCtdevDid(t *testing.T) {
	t.Run("enabled and started by ctdev", func(t *testing.T) {
		fakeAudioHost(t, true)
		logged := fakeRoot(t)
		if err := recordAdded(sysutil.Opts{}, "bluetooth-service", "enable", "start"); err != nil {
			t.Fatal(err)
		}
		if err := applyBluetoothService(context.Background(), sysutil.Opts{Stdout: &bytes.Buffer{}}, "inactive"); err != nil {
			t.Fatal(err)
		}
		out := logged()
		disable := strings.Index(out, "systemctl disable bluetooth.service")
		stop := strings.Index(out, "systemctl stop bluetooth.service")
		if disable < 0 || stop < disable {
			t.Errorf("want disable then stop, ran:\n%s", out)
		}
		if got := readAdded("bluetooth-service"); len(got) != 0 {
			t.Errorf("record not cleared: %v", got)
		}
	})
	t.Run("already on before ctdev", func(t *testing.T) {
		fakeAudioHost(t, true)
		logged := fakeRoot(t)
		systemctlQuery = func(_ context.Context, verb, _ string) string {
			if verb == "is-enabled" {
				return "enabled"
			}
			return "active"
		}
		var out bytes.Buffer
		if err := applyBluetoothService(context.Background(), sysutil.Opts{Stdout: &out}, "active"); err != nil {
			t.Fatal(err)
		}
		if err := applyBluetoothService(context.Background(), sysutil.Opts{Stdout: &out}, "inactive"); err != nil {
			t.Fatal(err)
		}
		if ran := logged(); ran != "" {
			t.Errorf("the service was already on, so nothing should run; ran:\n%s", ran)
		}
	})
}

func TestLDACOffRemovesConfig(t *testing.T) {
	var out bytes.Buffer
	o := sysutil.Opts{Stdout: &out, DryRun: true}
	s := settingByName(t, "Hi-fi Bluetooth audio")

	if err := s.ApplyFunc(context.Background(), o, "not installed"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "rm -f "+ldacConfPath) {
		t.Errorf("turning it off should remove %s, got %q", ldacConfPath, out.String())
	}

	out.Reset()
	if err := s.ApplyFunc(context.Background(), o, "installed"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "write "+ldacConfPath) {
		t.Errorf("turning it on should write %s, got %q", ldacConfPath, out.String())
	}
}

func TestAlertSoundsOffAndOn(t *testing.T) {
	var out bytes.Buffer
	o := sysutil.Opts{Stdout: &out, DryRun: true}
	s := settingByName(t, "Alert sounds")

	for _, v := range []string{"false", "true"} {
		out.Reset()
		if err := s.ApplyFunc(context.Background(), o, v); err != nil {
			t.Fatal(err)
		}
		if want := "dconf write /org/cinnamon/desktop/sound/event-sounds " + v; !strings.Contains(out.String(), want) {
			t.Errorf("want %q, got %q", want, out.String())
		}
	}
}

func TestAudioAndBluetoothDefaultsMatchThisComputer(t *testing.T) {
	// Thomas's Linux desktop, 2026-10-15: every package installed, the LDAC
	// config in place, the service active, event sounds off.
	want := map[string]string{
		"Sound system (PipeWire)": "installed",
		"Bluetooth headphones":    "installed",
		"Hi-fi Bluetooth audio":   "installed",
		"Alert sounds":            "false",
		"Bluetooth support":       "installed",
		"Bluetooth on at startup": "active",
	}
	for name, def := range want {
		if got := settingByName(t, name).Default; got != def {
			t.Errorf("%q default = %q, want %q", name, got, def)
		}
	}
}
