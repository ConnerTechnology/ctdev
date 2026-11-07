package setup

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

func TestStripGVariantType(t *testing.T) {
	tests := []struct{ in, want string }{
		{"uint32 0", "0"},
		{"uint32 1800", "1800"},
		{"3600", "3600"},
		{"int64 5", "5"},
		{"", ""},
		{"true", "true"},
	}
	for _, tt := range tests {
		if got := stripGVariantType(tt.in); got != tt.want {
			t.Errorf("stripGVariantType(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDconfSecondsToMinutes(t *testing.T) {
	tests := []struct{ in, want string }{
		{"uint32 0", "never"}, // idle-delay on a never-screensaver machine
		{"0", "never"},
		{"3600", "60"},
		{"uint32 1800", "30"},
		{"90", "90 s"}, // hand-set, not whole minutes: exact seconds
		{"7", "7 s"},
		{"int32 45", "45 s"},
		{"", ""}, // unset key: nothing to report
		{"garbage", "garbage"},
	}
	for _, tt := range tests {
		if got := dconfSecondsToMinutes(tt.in); got != tt.want {
			t.Errorf("dconfSecondsToMinutes(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMinutesToDconf(t *testing.T) {
	tests := []struct {
		value, gvType, want string
		wantErr             bool
	}{
		{"never", "", "0", false},
		{"60", "", "3600", false},
		{"never", "uint32", "uint32 0", false},
		{"30", "uint32", "uint32 1800", false},
		{"-5", "", "", true},
		{"soon", "", "", true},
	}
	for _, tt := range tests {
		got, err := minutesToDconf(tt.value, tt.gvType)
		if (err != nil) != tt.wantErr {
			t.Errorf("minutesToDconf(%q, %q) err = %v, wantErr %v", tt.value, tt.gvType, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("minutesToDconf(%q, %q) = %q, want %q", tt.value, tt.gvType, got, tt.want)
		}
	}
}

func TestApplyDconfMinutesWritesSeconds(t *testing.T) {
	var buf bytes.Buffer
	o := sysutil.Opts{Stdout: &buf, DryRun: true}
	if err := applyDconfMinutes(context.Background(), o, "/org/cinnamon/desktop/session/idle-delay", "never", "uint32"); err != nil {
		t.Fatal(err)
	}
	if want := "dconf write /org/cinnamon/desktop/session/idle-delay uint32 0"; !strings.Contains(buf.String(), want) {
		t.Errorf("output %q missing %q", buf.String(), want)
	}
}

func fakeSystemctlIsEnabled(t *testing.T, states map[string]string) {
	t.Helper()
	orig := systemctlIsEnabled
	t.Cleanup(func() { systemctlIsEnabled = orig })
	systemctlIsEnabled = func(_ context.Context, unit string) string { return states[unit] }
}

func TestSuspendMasked(t *testing.T) {
	allMasked := map[string]string{}
	for _, u := range sleepTargets {
		allMasked[u] = "masked"
	}
	fakeSystemctlIsEnabled(t, allMasked)
	if !SuspendMasked(context.Background()) {
		t.Error("SuspendMasked = false with every sleep target masked")
	}
	if got := detectSuspendMasked(context.Background()); got != "enabled" {
		t.Errorf("detectSuspendMasked = %q, want enabled", got)
	}

	partly := map[string]string{}
	for k, v := range allMasked {
		partly[k] = v
	}
	partly["hibernate.target"] = "static"
	fakeSystemctlIsEnabled(t, partly)
	if SuspendMasked(context.Background()) {
		t.Error("SuspendMasked = true with hibernate.target unmasked")
	}
	if got := detectSuspendMasked(context.Background()); got != "disabled" {
		t.Errorf("detectSuspendMasked = %q, want disabled", got)
	}
}

func TestApplyNeverSuspend(t *testing.T) {
	tests := []struct {
		value, verb string
	}{
		{"enabled", "systemctl mask " + strings.Join(sleepTargets, " ")},
		{"disabled", "systemctl unmask " + strings.Join(sleepTargets, " ")},
	}
	for _, tt := range tests {
		var buf bytes.Buffer
		o := sysutil.Opts{Stdout: &buf, DryRun: true}
		if err := applyNeverSuspend(context.Background(), o, tt.value); err != nil {
			t.Fatalf("%s: %v", tt.value, err)
		}
		if !strings.Contains(buf.String(), tt.verb) {
			t.Errorf("%s: output %q missing %q", tt.value, buf.String(), tt.verb)
		}
	}
	if err := applyNeverSuspend(context.Background(), sysutil.Opts{Stdout: &bytes.Buffer{}, DryRun: true}, "maybe"); err == nil {
		t.Error("applyNeverSuspend accepted an unknown value")
	}
}

func powerSetting(t *testing.T, name string) *Setting {
	t.Helper()
	for i := range Registry {
		if Registry[i].Slug == "power" && Registry[i].Name == name {
			return &Registry[i]
		}
	}
	t.Fatalf("power setting %q not in Registry", name)
	return nil
}

func TestPowerCategoryAbsorbsSleep(t *testing.T) {
	for _, s := range Registry {
		if s.Slug == "sleep" {
			t.Errorf("setting %q still in the removed sleep category", s.Name)
		}
	}
	s := powerSetting(t, neverSuspendName)
	if s.OneWay {
		t.Error("never suspend must have an off switch (not OneWay)")
	}
	if s.Default != "enabled" {
		t.Errorf("never suspend default = %q, want enabled", s.Default)
	}
	// It sits right after "Suspend when idle after".
	var power []string
	for _, r := range Registry {
		if r.Slug == "power" {
			power = append(power, r.Name)
		}
	}
	for i, n := range power {
		if n == neverSuspendName {
			if i == 0 || power[i-1] != "Suspend when idle after" {
				t.Errorf("never suspend should follow \"Suspend when idle after\"; power order = %v", power)
			}
		}
	}
}

func TestPowerDefaultsFromReferenceDesktop(t *testing.T) {
	// CTD-90 "Recommended defaults": Thomas's Linux desktop on 2026-10-15.
	want := map[string]string{
		"Power profile":                        "balanced",
		"Turn off the display after":           "60",
		"Suspend when idle after":              "never",
		"Start the screensaver after":          "never",
		"Lock the screen on suspend":           "true",
		"Lock the screen with the screensaver": "false",
		neverSuspendName:                       "enabled",
	}
	for name, def := range want {
		if got := powerSetting(t, name).Default; got != def {
			t.Errorf("%s default = %q, want %q", name, got, def)
		}
	}
}

func TestPowerTimersOfferNeverInMinutes(t *testing.T) {
	for _, name := range []string{"Turn off the display after", "Suspend when idle after", "Start the screensaver after"} {
		s := powerSetting(t, name)
		if s.Slider == nil || !s.Slider.Never || s.Slider.Unit != "min" {
			t.Errorf("%s: slider %+v, want a minutes slider with never", name, s.Slider)
		}
	}
}

func TestEveryPowerSettingHasOneLineWhy(t *testing.T) {
	for _, s := range Registry {
		if s.Slug != "power" {
			continue
		}
		if s.Description == "" || strings.Contains(s.Description, "\n") {
			t.Errorf("%s: description must be one line, got %q", s.Name, s.Description)
		}
		if s.OneWay {
			t.Errorf("%s: every power setting needs an off switch", s.Name)
		}
	}
}

func TestSuspendOnlySettingsNotNeededWhenMasked(t *testing.T) {
	allMasked := map[string]string{}
	for _, u := range sleepTargets {
		allMasked[u] = "masked"
	}
	for _, name := range []string{"Suspend when idle after", "Lock the screen on suspend"} {
		s := powerSetting(t, name)
		if s.NotNeededFn == nil {
			t.Fatalf("%s: must check whether suspend can happen", name)
		}
		fakeSystemctlIsEnabled(t, allMasked)
		if got := s.NotNeededFn(context.Background()); !strings.Contains(got, "suspend is turned off") {
			t.Errorf("%s with suspend masked: NotNeeded = %q, want a 'suspend is turned off' reason", name, got)
		}
		states := InitStates(context.Background(), []Setting{{Name: s.Name, Default: s.Default, NotNeededFn: s.NotNeededFn}})
		if states[0].NeedsApply(true) {
			t.Errorf("%s with suspend masked: applied by default", name)
		}
		fakeSystemctlIsEnabled(t, map[string]string{})
		if got := s.NotNeededFn(context.Background()); got != "" {
			t.Errorf("%s with suspend allowed: NotNeeded = %q, want needed", name, got)
		}
	}
}
