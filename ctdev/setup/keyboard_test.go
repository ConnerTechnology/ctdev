package setup

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

func TestKeyRepeatUnitsToCps(t *testing.T) {
	tests := []struct {
		units int
		want  string
	}{
		{1, "67"}, // 15 ms
		{2, "33"}, // 30 ms
		{3, "22"},
		{4, "17"},
		{5, "13"},
		{6, "11"},
		{7, "10"},
		{8, "8"}, // 120 ms
	}
	for _, tt := range tests {
		got, ok := keyRepeatUnitsToCps(tt.units)
		if !ok || got != tt.want {
			t.Errorf("keyRepeatUnitsToCps(%d) = %q, %v; want %q", tt.units, got, ok, tt.want)
		}
	}
	if _, ok := keyRepeatUnitsToCps(0); ok {
		t.Error("KeyRepeat 0 should not convert")
	}
}

func TestKeyRepeatCpsToUnits(t *testing.T) {
	tests := []struct {
		cps  string
		want int
	}{
		{"67", 1},
		{"50", 1}, // 20 ms: nearer 15 ms than 30 ms
		{"40", 2}, // 25 ms
		{"33", 2},
		{"22", 3},
		{"8", 8},
		{"1000", 1}, // floor at 1
	}
	for _, tt := range tests {
		got, err := keyRepeatCpsToUnits(tt.cps)
		if err != nil || got != tt.want {
			t.Errorf("keyRepeatCpsToUnits(%q) = %d, %v; want %d", tt.cps, got, err, tt.want)
		}
	}
	for _, bad := range []string{"fast", "0", "-5"} {
		if _, err := keyRepeatCpsToUnits(bad); err == nil {
			t.Errorf("keyRepeatCpsToUnits(%q): expected an error", bad)
		}
	}
}

// Every offered rate writes the KeyRepeat it came from, and reads back as
// itself, so the wizard never shows a change that didn't happen.
func TestMacKeyRepeatRateRoundTrip(t *testing.T) {
	ctx := context.Background()
	choices := macKeyRepeatRateChoices()
	if len(choices) != macKeyRepeatMaxUnits {
		t.Fatalf("%d choices, want %d", len(choices), macKeyRepeatMaxUnits)
	}
	for i, c := range choices {
		f := &fakeDefaults{}
		useFake(t, f)
		if err := applyMacKeyRepeatRate(ctx, sysutil.Opts{Stdout: &bytes.Buffer{}}, c.Value); err != nil {
			t.Fatalf("apply %s: %v", c.Value, err)
		}
		want := [][]string{{"write", "NSGlobalDomain", "KeyRepeat", "-int", strconv.Itoa(i + 1)}}
		if got := f.writes(); !reflect.DeepEqual(got, want) {
			t.Errorf("apply %s: writes %v, want %v", c.Value, got, want)
		}
		if got := detectMacKeyRepeatRate(ctx); got != c.Value {
			t.Errorf("detect after apply %s = %q", c.Value, got)
		}
		if !strings.Contains(c.Description, "characters per second") {
			t.Errorf("choice %s description %q doesn't say cps", c.Value, c.Description)
		}
	}
}

func TestDetectMacKeyRepeatRate(t *testing.T) {
	ctx := context.Background()
	for read, want := range map[string]string{
		"2":   "33",
		"12":  "6",   // slower than ctdev's choices: shown, matches none
		"120": "0.6", // under 1 cps: one decimal, never "0"
		"134": "0.5",
		"0":   "",
		"x":   "",
	} {
		useFake(t, &fakeDefaults{values: map[string]string{"NSGlobalDomain KeyRepeat": read}})
		if got := detectMacKeyRepeatRate(ctx); got != want {
			t.Errorf("KeyRepeat %s: detect = %q, want %q", read, got, want)
		}
	}
	useFake(t, &fakeDefaults{})
	if got := detectMacKeyRepeatRate(ctx); got != "" {
		t.Errorf("missing key: detect = %q, want empty", got)
	}
}

func TestMacBoolKeysOnOff(t *testing.T) {
	ctx := context.Background()
	keys := macBoolKeys{"A", "B"}
	f := &fakeDefaults{values: map[string]string{"NSGlobalDomain Other": "1"}}
	useFake(t, f)
	var buf bytes.Buffer

	if got := keys.detect(ctx); got != "disabled" {
		t.Errorf("unset: detect = %q, want disabled", got)
	}
	if err := keys.apply(ctx, sysutil.Opts{Stdout: &buf}, "enabled"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	want := [][]string{
		{"write", "NSGlobalDomain", "A", "-bool", "false"},
		{"write", "NSGlobalDomain", "B", "-bool", "false"},
	}
	if got := f.writes(); !reflect.DeepEqual(got, want) {
		t.Errorf("enable writes %v, want %v", got, want)
	}
	if got := keys.detect(ctx); got != "enabled" {
		t.Errorf("after enable: detect = %q, want enabled", got)
	}

	// Off removes exactly what on added, and nothing else.
	delete(f.values, "NSGlobalDomain B") // one key already gone: no failing delete
	if err := keys.apply(ctx, sysutil.Opts{Stdout: &buf}, "disabled"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !reflect.DeepEqual(f.values, map[string]string{"NSGlobalDomain Other": "1"}) {
		t.Errorf("after disable values = %v", f.values)
	}
	if got := keys.detect(ctx); got != "disabled" {
		t.Errorf("after disable: detect = %q, want disabled", got)
	}

	// A key set to true by hand isn't "enabled".
	f.values["NSGlobalDomain A"] = "0"
	f.values["NSGlobalDomain B"] = "1"
	if got := keys.detect(ctx); got != "disabled" {
		t.Errorf("one key true: detect = %q, want disabled", got)
	}

	if err := keys.apply(ctx, sysutil.Opts{Stdout: &buf}, "maybe"); err == nil {
		t.Error("unknown value: expected an error")
	}

	f.failWrite = map[string]bool{"NSGlobalDomain A": true}
	if err := keys.apply(ctx, sysutil.Opts{Stdout: &buf}, "enabled"); err == nil {
		t.Error("failed write: expected an error")
	}
	if f.values["NSGlobalDomain B"] != "0" {
		t.Error("a failed write should not stop the next key")
	}
}

func TestApplyNumlockxOff(t *testing.T) {
	var got []string
	orig := removePackages
	removePackages = func(_ context.Context, _ sysutil.Opts, names ...string) error {
		got = names
		return errors.New("boom")
	}
	t.Cleanup(func() { removePackages = orig })

	err := applyNumlockx(context.Background(), sysutil.Opts{Stdout: &bytes.Buffer{}}, "not installed")
	if err == nil || !reflect.DeepEqual(got, []string{"numlockx"}) {
		t.Errorf("off: removed %v, err %v; want numlockx removed and the error returned", got, err)
	}
}

func TestApplyNumlockxOnDryRun(t *testing.T) {
	var buf bytes.Buffer
	if err := applyNumlockx(context.Background(), sysutil.Opts{Stdout: &buf, DryRun: true}, "installed"); err != nil {
		t.Fatalf("on: %v", err)
	}
	if !strings.Contains(buf.String(), "apt-get install -y -qq numlockx") {
		t.Errorf("dry-run output = %q", buf.String())
	}
}

// Every keyboard setting has a "why", and every on/off one can be turned off.
func TestKeyboardSettingsHaveWhyAndOffSwitch(t *testing.T) {
	names := map[string]bool{}
	for _, s := range FilterBySlug(Registry, "keyboard") {
		names[s.Name] = true
		if strings.TrimSpace(s.Description) == "" {
			t.Errorf("%s: no description", s.Name)
		}
		if s.OneWay {
			t.Errorf("%s: OneWay, want an off switch", s.Name)
		}
		if s.HardwareFn == nil {
			t.Errorf("%s: no platform gate", s.Name)
		}
		if s.Control == ControlToggle {
			on, off := TogglePair(&s, "")
			if s.Default != on || off == "other" {
				t.Errorf("%s: toggle pair (%q, %q) for default %q", s.Name, on, off, s.Default)
			}
		}
	}
	for _, n := range []string{"Key repeat delay", "Key repeat rate", "Type exactly what I type", "Holding a key repeats it", "NumLock on boot"} {
		if !names[n] {
			t.Errorf("no keyboard setting %q", n)
		}
	}
}
