package setup

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// fakeDefaults stands in for the `defaults` and `killall` binaries. values
// holds "domain key" → the text `defaults read` prints; a missing entry exits
// non-zero the way macOS does for an unknown key.
type fakeDefaults struct {
	values    map[string]string
	failWrite map[string]bool // "domain key" whose write fails
	calls     [][]string
}

// exec stands in for macExec. Dry-run goes to the real sysutil.Run, which
// only prints, so tests see exactly the dry-run output users get.
func (f *fakeDefaults) exec(ctx context.Context, o sysutil.Opts, name string, args ...string) error {
	if o.DryRun {
		return sysutil.Run(ctx, o, name, args...)
	}
	_, err := f.run(ctx, name, args...)
	return err
}

func (f *fakeDefaults) run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	if name != "defaults" || len(args) < 3 {
		return nil, nil
	}
	id := args[1] + " " + args[2]
	switch args[0] {
	case "read":
		v, ok := f.values[id]
		if !ok {
			return nil, errors.New("exit status 1")
		}
		return []byte(v + "\n"), nil
	case "write":
		if f.failWrite[id] {
			return nil, errors.New("exit status 1")
		}
		if f.values == nil {
			f.values = map[string]string{}
		}
		if len(args) >= 5 {
			f.values[id] = readBack(args[3], args[4])
		}
		return nil, nil
	case "delete":
		if _, ok := f.values[id]; !ok {
			return nil, errors.New("exit status 1")
		}
		delete(f.values, id)
		return nil, nil
	}
	return nil, nil
}

// readBack mimics what `defaults read` prints after a typed write.
func readBack(flag, v string) string {
	if flag == "-bool" {
		if v == "true" {
			return "1"
		}
		return "0"
	}
	return v
}

// writes returns the recorded `defaults write` calls, minus the binary name.
func (f *fakeDefaults) writes() [][]string {
	var out [][]string
	for _, c := range f.calls {
		if c[0] == "defaults" && len(c) > 1 && c[1] == "write" {
			out = append(out, c[1:])
		}
	}
	return out
}

func useFake(t *testing.T, f *fakeDefaults) {
	t.Helper()
	origRead, origExec := macRead, macExec
	macRead, macExec = f.run, f.exec
	t.Cleanup(func() { macRead, macExec = origRead, origExec })
}

func TestMacDefaultMatches(t *testing.T) {
	tests := []struct {
		d    macDefault
		read string
		want bool
	}{
		{macDefault{Type: defBool, Value: "true"}, "1", true},
		{macDefault{Type: defBool, Value: "true"}, "0", false},
		{macDefault{Type: defBool, Value: "false"}, "0", true},
		{macDefault{Type: defBool, Value: "false"}, "1", false},
		{macDefault{Type: defBool, Value: "false"}, "", false},
		{macDefault{Type: defInt, Value: "1"}, "1", true},
		{macDefault{Type: defInt, Value: "1"}, "2", false},
		{macDefault{Type: defFloat, Value: "0.15"}, "0.15", true},
		{macDefault{Type: defFloat, Value: "0.15"}, "0.150", true},
		{macDefault{Type: defFloat, Value: "0"}, "0", true},
		{macDefault{Type: defFloat, Value: "0"}, "0.5", false},
		{macDefault{Type: defFloat, Value: "0.1"}, "abc", false},
		{macDefault{Type: defString, Value: "scale"}, "scale", true},
		{macDefault{Type: defString, Value: "scale"}, "genie", false},
	}
	for _, tt := range tests {
		if got := tt.d.matches(tt.read); got != tt.want {
			t.Errorf("%v %q matches(%q) = %v, want %v", tt.d.Type, tt.d.Value, tt.read, got, tt.want)
		}
	}
}

func TestKeyRepeatMsToUnits(t *testing.T) {
	tests := []struct {
		ms   string
		want int
	}{
		{"15", 1},
		{"30", 2},
		{"195", 13},
		{"200", 13}, // 13.33 rounds down
		{"203", 14}, // 13.53 rounds up
		{"1800", 120},
		{"0", 1}, // floor at 1
		{"5", 1},
	}
	for _, tt := range tests {
		got, err := keyRepeatMsToUnits(tt.ms)
		if err != nil {
			t.Errorf("keyRepeatMsToUnits(%q) error: %v", tt.ms, err)
			continue
		}
		if got != tt.want {
			t.Errorf("keyRepeatMsToUnits(%q) = %d, want %d", tt.ms, got, tt.want)
		}
	}
	if _, err := keyRepeatMsToUnits("fast"); err == nil {
		t.Error("expected error for non-numeric ms")
	}
}

func TestDetectMacKeyRepeat(t *testing.T) {
	ctx := context.Background()
	useFake(t, &fakeDefaults{values: map[string]string{
		"NSGlobalDomain KeyRepeat":        "2",
		"NSGlobalDomain InitialKeyRepeat": "15",
	}})
	if got := detectMacKeyRepeat(ctx, "KeyRepeat"); got != "30" {
		t.Errorf("KeyRepeat = %q, want 30", got)
	}
	if got := detectMacKeyRepeat(ctx, "InitialKeyRepeat"); got != "225" {
		t.Errorf("InitialKeyRepeat = %q, want 225", got)
	}

	useFake(t, &fakeDefaults{})
	if got := detectMacKeyRepeat(ctx, "KeyRepeat"); got != "" {
		t.Errorf("missing key = %q, want empty", got)
	}
}

func TestApplyMacKeyRepeat(t *testing.T) {
	ctx := context.Background()
	f := &fakeDefaults{}
	useFake(t, f)
	var buf bytes.Buffer
	if err := applyMacKeyRepeat(ctx, sysutil.Opts{Stdout: &buf}, "InitialKeyRepeat", "195"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := [][]string{{"write", "NSGlobalDomain", "InitialKeyRepeat", "-int", "13"}}
	if got := f.writes(); !reflect.DeepEqual(got, want) {
		t.Errorf("writes = %v, want %v", got, want)
	}
	if got := detectMacKeyRepeat(ctx, "InitialKeyRepeat"); got != "195" {
		t.Errorf("detect after apply = %q, want 195", got)
	}

	if err := applyMacKeyRepeat(ctx, sysutil.Opts{Stdout: &buf}, "KeyRepeat", "nope"); err == nil {
		t.Error("expected error for non-numeric value")
	}

	f.failWrite = map[string]bool{"NSGlobalDomain KeyRepeat": true}
	if err := applyMacKeyRepeat(ctx, sysutil.Opts{Stdout: &buf}, "KeyRepeat", "30"); err == nil {
		t.Error("expected error when the write fails")
	}

	dry := &fakeDefaults{}
	useFake(t, dry)
	buf.Reset()
	if err := applyMacKeyRepeat(ctx, sysutil.Opts{Stdout: &buf, DryRun: true}, "KeyRepeat", "30"); err != nil {
		t.Fatalf("dry-run error: %v", err)
	}
	if len(dry.calls) != 0 {
		t.Errorf("dry-run ran commands: %v", dry.calls)
	}
	if !strings.Contains(buf.String(), "KeyRepeat -int 2") {
		t.Errorf("dry-run output = %q", buf.String())
	}
}

func TestMacKeyRepeatPostApplyNote(t *testing.T) {
	hook, ok := PostApplyHooks[macKeyRepeatGroup]
	if !ok {
		t.Fatalf("no post-apply hook for %q", macKeyRepeatGroup)
	}
	var buf bytes.Buffer
	if err := hook(context.Background(), sysutil.Opts{Stdout: &buf}); err != nil {
		t.Fatalf("hook error: %v", err)
	}
	if !strings.Contains(buf.String(), "log out") {
		t.Errorf("expected a log-out note; got %q", buf.String())
	}
}

func funcPtr(fn func() bool) uintptr { return reflect.ValueOf(fn).Pointer() }

func TestRegistryKeyRepeatSettings(t *testing.T) {
	type key struct{ name, gate string }
	found := map[key]Setting{}
	for _, s := range FilterBySlug(Registry, "keyboard") {
		switch funcPtr(s.HardwareFn) {
		case funcPtr(gateMacOS):
			found[key{s.Name, "macos"}] = s
		case funcPtr(gateCinnamon):
			found[key{s.Name, "cinnamon"}] = s
		}
	}
	for _, name := range []string{"Key repeat delay", "Key repeat rate"} {
		if _, ok := found[key{name, "cinnamon"}]; !ok {
			t.Errorf("Cinnamon %q should keep gateCinnamon", name)
		}
	}

	s, ok := found[key{"Key repeat delay", "macos"}]
	if !ok {
		t.Fatal("no macOS-gated Key repeat delay")
	}
	if s.Category != "Keyboard" || s.Control != ControlSlider {
		t.Errorf("%s: category %q control %v", s.Name, s.Category, s.Control)
	}
	if s.Default != macKeyRepeatDelayDefaultMs {
		t.Errorf("%s: default %q, want %q", s.Name, s.Default, macKeyRepeatDelayDefaultMs)
	}
	if s.Slider == nil || s.Slider.Min != 120 || s.Slider.Max != 1800 || s.Slider.Step != 15 || s.Slider.Unit != "ms" {
		t.Errorf("%s: slider %+v", s.Name, s.Slider)
	}

	rate, ok := found[key{"Key repeat rate", "macos"}]
	if !ok {
		t.Fatal("no macOS-gated Key repeat rate")
	}
	if rate.Category != "Keyboard" || rate.Control != ControlPicker {
		t.Errorf("%s: category %q control %v", rate.Name, rate.Category, rate.Control)
	}
	if rate.Default != macKeyRepeatRateDefaultCps {
		t.Errorf("%s: default %q, want %q", rate.Name, rate.Default, macKeyRepeatRateDefaultCps)
	}
	inChoices := false
	for _, c := range rate.Choices {
		inChoices = inChoices || c.Value == rate.Default
	}
	if !inChoices {
		t.Errorf("%s: default %q is not one of its choices %v", rate.Name, rate.Default, rate.Choices)
	}

	for _, c := range []struct {
		s       Setting
		storage string
	}{{s, "InitialKeyRepeat"}, {rate, "KeyRepeat"}} {
		s := c.s
		if s.ApplyGroup != macKeyRepeatGroup {
			t.Errorf("%s: ApplyGroup %q, want %q", s.Name, s.ApplyGroup, macKeyRepeatGroup)
		}
		if s.DetectFunc == nil || s.ApplyFunc == nil {
			t.Fatalf("%s: missing detect/apply", s.Name)
		}

		f := &fakeDefaults{}
		useFake(t, f)
		if got := s.DetectFunc(context.Background()); got != "" {
			t.Errorf("%s: detect with key missing = %q, want empty", s.Name, got)
		}
		var buf bytes.Buffer
		if err := s.ApplyFunc(context.Background(), sysutil.Opts{Stdout: &buf}, s.Default); err != nil {
			t.Fatalf("%s: apply: %v", s.Name, err)
		}
		w := f.writes()
		if len(w) != 1 || w[0][2] != c.storage {
			t.Errorf("%s: writes %v, want one write of %s", s.Name, w, c.storage)
		}
		if got := s.DetectFunc(context.Background()); got != s.Default {
			t.Errorf("%s: detect after apply = %q, want %q", s.Name, got, s.Default)
		}
	}
}

func TestMacDesktopTables(t *testing.T) {
	want := map[string]macDefault{
		"com.apple.dock autohide":                            {Type: defBool, Value: "true"},
		"com.apple.dock autohide-delay":                      {Type: defFloat, Value: "0"},
		"com.apple.dock autohide-time-modifier":              {Type: defFloat, Value: "0.15"},
		"com.apple.dock show-recents":                        {Type: defBool, Value: "false"},
		"com.apple.dock launchanim":                          {Type: defBool, Value: "false"},
		"com.apple.dock mineffect":                           {Type: defString, Value: "scale"},
		"com.apple.dock expose-animation-duration":           {Type: defFloat, Value: "0.1"},
		"NSGlobalDomain NSAutomaticWindowAnimationsEnabled":  {Type: defBool, Value: "false"},
		"com.apple.finder ShowPathbar":                       {Type: defBool, Value: "true"},
		"com.apple.finder ShowStatusBar":                     {Type: defBool, Value: "true"},
		"com.apple.finder FXDefaultSearchScope":              {Type: defString, Value: "SCcf"},
		"com.apple.finder FXPreferredViewStyle":              {Type: defString, Value: "Nlsv"},
		"com.apple.finder QuitMenuItem":                      {Type: defBool, Value: "true"},
		"com.apple.desktopservices DSDontWriteNetworkStores": {Type: defBool, Value: "true"},
		"com.apple.desktopservices DSDontWriteUSBStores":     {Type: defBool, Value: "true"},
	}
	got := map[string]macDefault{}
	for _, table := range [][]macDefault{macDesktopDock, macDesktopAnimations, macDesktopFinder, macDesktopDSStore} {
		for _, d := range table {
			id := d.Domain + " " + d.Key
			if _, dup := got[id]; dup {
				t.Errorf("duplicate entry %s", id)
			}
			got[id] = d
		}
	}
	if !reflect.DeepEqual(keys(got), keys(want)) {
		t.Errorf("desktop keys = %v, want %v", keys(got), keys(want))
	}
	for id, w := range want {
		if g := got[id]; g.Type != w.Type || g.Value != w.Value {
			t.Errorf("%s = (%v, %q), want (%v, %q)", id, g.Type, g.Value, w.Type, w.Value)
		}
	}
}

func keys(m map[string]macDefault) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// TestRegistryMacDesktopSettings checks each macOS desktop setting: gated to
// macOS, on writes its keys and detects "enabled", off deletes exactly those
// keys and detects "disabled", and the setting is in the group whose hook
// restarts the right app.
func TestRegistryMacDesktopSettings(t *testing.T) {
	cases := map[string]struct {
		table []macDefault
		group string
	}{
		"Auto-hiding Dock":                             {macDesktopDock, macDockGroup},
		"Faster animations":                            {macDesktopAnimations, macDockGroup},
		"Finder list view with path and status bars":   {macDesktopFinder, macFinderGroup},
		"No .DS_Store files on network and USB drives": {macDesktopDSStore, macFinderGroup},
	}
	seen := map[string]bool{}
	for _, s := range FilterBySlug(Registry, "desktop") {
		c, ok := cases[s.Name]
		if !ok {
			continue
		}
		seen[s.Name] = true
		t.Run(s.Name, func(t *testing.T) {
			if funcPtr(s.HardwareFn) != funcPtr(gateMacOS) {
				t.Error("not gated to macOS")
			}
			if s.Control != ControlToggle || s.OneWay || s.Default != "enabled" || s.Description == "" {
				t.Errorf("control %v oneway %v default %q", s.Control, s.OneWay, s.Default)
			}
			ctx := context.Background()
			o := sysutil.Opts{Stdout: &bytes.Buffer{}}
			f := &fakeDefaults{}
			useFake(t, f)
			if got := s.DetectFunc(ctx); got != "disabled" {
				t.Errorf("detect on a fresh Mac = %q", got)
			}
			if err := s.ApplyFunc(ctx, o, "enabled"); err != nil {
				t.Fatalf("apply on: %v", err)
			}
			if len(f.writes()) != len(c.table) {
				t.Errorf("on wrote %v, want %d writes", f.writes(), len(c.table))
			}
			if got := s.DetectFunc(ctx); got != "enabled" {
				t.Errorf("detect after on = %q", got)
			}
			if s.ApplyGroup != c.group {
				t.Errorf("ApplyGroup = %q, want %q", s.ApplyGroup, c.group)
			}
			for _, call := range f.calls {
				if call[0] == "killall" {
					t.Errorf("apply restarted an app itself: %v", call)
				}
			}

			f.calls = nil
			if err := s.ApplyFunc(ctx, o, "disabled"); err != nil {
				t.Fatalf("apply off: %v", err)
			}
			var deletes [][]string
			for _, call := range f.calls {
				if call[0] == "defaults" && call[1] == "delete" {
					deletes = append(deletes, call[2:])
				}
			}
			var want [][]string
			for _, d := range c.table {
				want = append(want, []string{d.Domain, d.Key})
			}
			if !reflect.DeepEqual(deletes, want) {
				t.Errorf("off deleted %v, want %v", deletes, want)
			}
			if len(f.writes()) != 0 {
				t.Errorf("off wrote %v", f.writes())
			}
		})
	}
	for name := range cases {
		if !seen[name] {
			t.Errorf("%q not in the desktop category", name)
		}
	}
}

func TestMacRestartHooks(t *testing.T) {
	for group, app := range map[string]string{macDockGroup: "Dock", macFinderGroup: "Finder"} {
		hook, ok := PostApplyHooks[group]
		if !ok {
			t.Fatalf("no post-apply hook for %q", group)
		}
		f := &fakeDefaults{}
		useFake(t, f)
		if err := hook(context.Background(), sysutil.Opts{Stdout: &bytes.Buffer{}}); err != nil {
			t.Fatalf("%s hook: %v", group, err)
		}
		if !reflect.DeepEqual(f.calls, [][]string{{"killall", app}}) {
			t.Errorf("%s hook ran %v, want one killall %s", group, f.calls, app)
		}
	}
}

// TestMacOnOffTables pins the keys and values of the macOS on/off settings
// outside desktop's Dock/Finder/animations, and checks no key is in two tables.
func TestMacOnOffTables(t *testing.T) {
	want := map[string]macDefault{
		"NSGlobalDomain com.apple.sound.beep.feedback":        {Type: defBool, Value: "true"},
		"NSGlobalDomain NSNavPanelExpandedStateForSaveMode":   {Type: defBool, Value: "true"},
		"NSGlobalDomain NSNavPanelExpandedStateForSaveMode2":  {Type: defBool, Value: "true"},
		"NSGlobalDomain PMPrintingExpandedStateForPrint":      {Type: defBool, Value: "true"},
		"NSGlobalDomain PMPrintingExpandedStateForPrint2":     {Type: defBool, Value: "true"},
		"com.apple.screensaver askForPassword":                {Type: defInt, Value: "1"},
		"com.apple.screensaver askForPasswordDelay":           {Type: defInt, Value: "0"},
		"NSGlobalDomain NSAutomaticQuoteSubstitutionEnabled":  {Type: defBool, Value: "false"},
		"NSGlobalDomain NSAutomaticDashSubstitutionEnabled":   {Type: defBool, Value: "false"},
		"NSGlobalDomain NSAutomaticSpellingCorrectionEnabled": {Type: defBool, Value: "false"},
		"NSGlobalDomain NSAutomaticCapitalizationEnabled":     {Type: defBool, Value: "false"},
		"NSGlobalDomain NSAutomaticPeriodSubstitutionEnabled": {Type: defBool, Value: "false"},
		"NSGlobalDomain ApplePressAndHoldEnabled":             {Type: defBool, Value: "false"},
	}
	got := map[string]macDefault{}
	for _, table := range [][]macDefault{
		macAudioVolumeClick, macDesktopDialogs, macPowerScreensaverPassword,
		macKeyboardTypeAsTyped, macKeyboardHoldToRepeat,
	} {
		for _, d := range table {
			got[d.Domain+" "+d.Key] = d
		}
	}
	if !reflect.DeepEqual(keys(got), keys(want)) {
		t.Errorf("keys = %v, want %v", keys(got), keys(want))
	}
	for id, w := range want {
		if g := got[id]; g.Type != w.Type || g.Value != w.Value {
			t.Errorf("%s = (%v, %q), want (%v, %q)", id, g.Type, g.Value, w.Type, w.Value)
		}
	}

	// No key belongs to two settings, desktop's included.
	seen := map[string]bool{}
	for _, table := range [][]macDefault{
		macAudioVolumeClick, macDesktopDialogs, macPowerScreensaverPassword,
		macKeyboardTypeAsTyped, macKeyboardHoldToRepeat,
		macDesktopDock, macDesktopAnimations, macDesktopFinder, macDesktopDSStore,
	} {
		for _, d := range table {
			id := d.Domain + " " + d.Key
			if seen[id] {
				t.Errorf("%s is in two tables", id)
			}
			seen[id] = true
		}
	}
}

// TestRegistryMacOnOffSettings checks the macOS settings that moved out of
// the old `macos` category: each is in its topic's category, gated to
// macOS, on by default, on writes its keys and detects "enabled", and off
// deletes exactly those keys and detects "disabled". None restarts an app.
func TestRegistryMacOnOffSettings(t *testing.T) {
	cases := []struct {
		slug, name string
		table      []macDefault
	}{
		{"audio", "Click when changing volume", macAudioVolumeClick},
		{"desktop", "Expanded save and print dialogs", macDesktopDialogs},
		{"power", "Ask for the password right after the screensaver", macPowerScreensaverPassword},
		{"keyboard", "Type exactly what I type", macKeyboardTypeAsTyped},
		{"keyboard", "Holding a key repeats it", macKeyboardHoldToRepeat},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var s *Setting
			for _, r := range FilterBySlug(Registry, c.slug) {
				if r.Name == c.name {
					s = &r
				}
			}
			if s == nil {
				t.Fatalf("%q not in the %s category", c.name, c.slug)
			}
			if funcPtr(s.HardwareFn) != funcPtr(gateMacOS) {
				t.Error("not gated to macOS")
			}
			if s.Control != ControlToggle || s.OneWay || s.Default != "enabled" || s.Description == "" || s.ApplyGroup != "" {
				t.Errorf("control %v oneway %v default %q group %q", s.Control, s.OneWay, s.Default, s.ApplyGroup)
			}

			ctx := context.Background()
			o := sysutil.Opts{Stdout: &bytes.Buffer{}}
			f := &fakeDefaults{values: map[string]string{"NSGlobalDomain Unrelated": "1"}}
			useFake(t, f)
			if got := s.DetectFunc(ctx); got != "disabled" {
				t.Errorf("detect on a fresh Mac = %q", got)
			}
			if err := s.ApplyFunc(ctx, o, "enabled"); err != nil {
				t.Fatalf("apply on: %v", err)
			}
			var wantWrites [][]string
			for _, d := range c.table {
				wantWrites = append(wantWrites, []string{"write", d.Domain, d.Key, d.Type.flag(), d.Value})
			}
			if !reflect.DeepEqual(f.writes(), wantWrites) {
				t.Errorf("on wrote %v, want %v", f.writes(), wantWrites)
			}
			if got := s.DetectFunc(ctx); got != "enabled" {
				t.Errorf("detect after on = %q", got)
			}

			f.calls = nil
			if err := s.ApplyFunc(ctx, o, "disabled"); err != nil {
				t.Fatalf("apply off: %v", err)
			}
			if len(f.writes()) != 0 {
				t.Errorf("off wrote %v", f.writes())
			}
			if !reflect.DeepEqual(f.values, map[string]string{"NSGlobalDomain Unrelated": "1"}) {
				t.Errorf("after off, values = %v; want only the unrelated key", f.values)
			}
			if got := s.DetectFunc(ctx); got != "disabled" {
				t.Errorf("detect after off = %q", got)
			}
			for _, call := range f.calls {
				if call[0] == "killall" {
					t.Errorf("apply restarted an app: %v", call)
				}
			}
		})
	}
}
