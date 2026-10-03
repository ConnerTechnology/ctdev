package setup

import (
	"bytes"
	"context"
	"errors"
	"reflect"
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

// allApplied returns a fake whose every table entry reads back at its value.
func allApplied() *fakeDefaults {
	f := &fakeDefaults{values: map[string]string{}}
	for _, d := range macOSDefaults {
		f.values[d.Domain+" "+d.Key] = readBack(d.Type.flag(), d.Value)
	}
	return f
}

func TestApplyMacOSDefaultsDryRun(t *testing.T) {
	f := &fakeDefaults{}
	useFake(t, f)
	var buf bytes.Buffer
	err := ApplyMacOSDefaults(context.Background(), sysutil.Opts{Stdout: &buf, DryRun: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if len(f.calls) != 0 {
		t.Errorf("dry-run ran commands: %v", f.calls)
	}

	// One line per section, in table order, then the restart line.
	var sections []string
	for _, d := range macOSDefaults {
		if len(sections) == 0 || sections[len(sections)-1] != d.Section {
			sections = append(sections, d.Section)
		}
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != len(sections)+1 {
		t.Fatalf("expected %d dry-run lines, got %d:\n%s", len(sections)+1, len(lines), output)
	}
	for i, sec := range sections {
		if want := "[dry-run] Would configure " + sec + ":"; !strings.HasPrefix(lines[i], want) {
			t.Errorf("line %d = %q, want prefix %q", i, lines[i], want)
		}
	}
	if lines[len(lines)-1] != "[dry-run] Would restart Dock and Finder" {
		t.Errorf("last line = %q", lines[len(lines)-1])
	}
	// Every entry is named with its value.
	for _, d := range macOSDefaults {
		if !strings.Contains(output, " "+d.Key+"="+d.Value) {
			t.Errorf("dry-run output missing %s=%s", d.Key, d.Value)
		}
	}
	if !strings.Contains(lines[0], "com.apple.dock autohide=true, launchanim=false") {
		t.Errorf("Dock line = %q", lines[0])
	}
}

func TestMacOSDefaultsTable_RequiredEntries(t *testing.T) {
	want := map[string]macDefault{
		"NSGlobalDomain ApplePressAndHoldEnabled":           {Type: defBool, Value: "false"},
		"NSGlobalDomain NSAutomaticWindowAnimationsEnabled": {Type: defBool, Value: "false"},
		"com.apple.dock autohide-delay":                     {Type: defFloat, Value: "0"},
		"com.apple.dock autohide-time-modifier":             {Type: defFloat, Value: "0.15"},
		"com.apple.dock mineffect":                          {Type: defString, Value: "scale"},
		"com.apple.dock expose-animation-duration":          {Type: defFloat, Value: "0.1"},
		"com.apple.dock autohide":                           {Type: defBool, Value: "true"},
		"com.apple.screensaver askForPasswordDelay":         {Type: defInt, Value: "0"},
	}
	got := map[string]macDefault{}
	for _, d := range macOSDefaults {
		id := d.Domain + " " + d.Key
		if _, dup := got[id]; dup {
			t.Errorf("duplicate entry %s", id)
		}
		got[id] = d
	}
	for id, w := range want {
		g, ok := got[id]
		if !ok {
			t.Errorf("missing entry %s", id)
			continue
		}
		if g.Type != w.Type || g.Value != w.Value {
			t.Errorf("%s = (%v, %q), want (%v, %q)", id, g.Type, g.Value, w.Type, w.Value)
		}
	}
	// Key repeat moved to the keyboard category; reduceMotion is left alone.
	for _, id := range []string{"NSGlobalDomain KeyRepeat", "NSGlobalDomain InitialKeyRepeat", "com.apple.universalaccess reduceMotion"} {
		if _, ok := got[id]; ok {
			t.Errorf("%s should not be in the macOS defaults table", id)
		}
	}
}

func TestApplyMacOSDefaults_WritesEveryEntryWithTypedArgs(t *testing.T) {
	f := &fakeDefaults{}
	useFake(t, f)
	var buf bytes.Buffer
	if err := ApplyMacOSDefaults(context.Background(), sysutil.Opts{Stdout: &buf}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var want [][]string
	for _, d := range macOSDefaults {
		want = append(want, []string{"write", d.Domain, d.Key, d.Type.flag(), d.Value})
	}
	if got := f.writes(); !reflect.DeepEqual(got, want) {
		t.Errorf("writes mismatch:\ngot  %v\nwant %v", got, want)
	}
	// Spot-check one of each type flag.
	flags := map[string]bool{}
	for _, w := range f.writes() {
		flags[w[3]] = true
	}
	for _, fl := range []string{"-bool", "-int", "-float", "-string"} {
		if !flags[fl] {
			t.Errorf("no write used %s", fl)
		}
	}
	last := f.calls[len(f.calls)-2:]
	if !reflect.DeepEqual(last, [][]string{{"killall", "Dock"}, {"killall", "Finder"}}) {
		t.Errorf("expected Dock and Finder restart last; got %v", last)
	}
	// After a successful apply, detect agrees.
	if got := detectMacOSDefaults(context.Background()); got != "applied" {
		t.Errorf("detect after apply = %q, want applied", got)
	}
}

func TestApplyMacOSDefaults_AccumulatesErrorsAndContinues(t *testing.T) {
	f := &fakeDefaults{failWrite: map[string]bool{
		"com.apple.dock autohide":      true,
		"com.apple.finder ShowPathbar": true,
	}}
	useFake(t, f)
	var buf bytes.Buffer
	err := ApplyMacOSDefaults(context.Background(), sysutil.Opts{Stdout: &buf})
	if err == nil {
		t.Fatal("expected error when writes fail")
	}
	if !strings.Contains(err.Error(), "defaults write com.apple.dock autohide") ||
		!strings.Contains(err.Error(), "defaults write com.apple.finder ShowPathbar") {
		t.Errorf("expected both failures in error chain; got %v", err)
	}
	if got := len(f.writes()); got != len(macOSDefaults) {
		t.Errorf("expected every entry attempted (%d), got %d", len(macOSDefaults), got)
	}
	out := buf.String()
	if !strings.Contains(out, "warning: defaults write com.apple.dock autohide") {
		t.Errorf("expected warning for autohide; got:\n%s", out)
	}
	if !strings.Contains(out, "applied with 2 warning(s)") {
		t.Errorf("expected summary count of warnings; got:\n%s", out)
	}
}

func TestDetectMacOSDefaults(t *testing.T) {
	ctx := context.Background()

	t.Run("all entries match", func(t *testing.T) {
		useFake(t, allApplied())
		if got := detectMacOSDefaults(ctx); got != "applied" {
			t.Errorf("got %q, want applied", got)
		}
	})

	t.Run("autohide on and everything else missing (original bug)", func(t *testing.T) {
		useFake(t, &fakeDefaults{values: map[string]string{"com.apple.dock autohide": "1"}})
		if got := detectMacOSDefaults(ctx); got != "not applied" {
			t.Errorf("got %q, want not applied", got)
		}
		st := SettingState{CurrentValue: detectMacOSDefaults(ctx), DesiredValue: "applied", Enabled: true}
		if !st.NeedsApply(false) {
			t.Error("batch apply should run when only autohide is set")
		}
	})

	t.Run("one entry differs", func(t *testing.T) {
		f := allApplied()
		f.values["com.apple.finder FXPreferredViewStyle"] = "icnv"
		useFake(t, f)
		if got := detectMacOSDefaults(ctx); got != "not applied" {
			t.Errorf("got %q, want not applied", got)
		}
	})

	t.Run("one key missing", func(t *testing.T) {
		f := allApplied()
		delete(f.values, "com.apple.dock mineffect")
		useFake(t, f)
		if got := detectMacOSDefaults(ctx); got != "not applied" {
			t.Errorf("got %q, want not applied", got)
		}
	})

	t.Run("bool reads back wrong", func(t *testing.T) {
		f := allApplied()
		f.values["NSGlobalDomain ApplePressAndHoldEnabled"] = "1"
		useFake(t, f)
		if got := detectMacOSDefaults(ctx); got != "not applied" {
			t.Errorf("got %q, want not applied", got)
		}
	})

	t.Run("floats compare numerically", func(t *testing.T) {
		f := allApplied()
		f.values["com.apple.dock autohide-time-modifier"] = "0.1500"
		f.values["com.apple.dock autohide-delay"] = "0.0"
		useFake(t, f)
		if got := detectMacOSDefaults(ctx); got != "applied" {
			t.Errorf("got %q, want applied", got)
		}
	})
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

	delay, ok := found[key{"Key repeat delay", "macos"}]
	if !ok {
		t.Fatal("no macOS-gated Key repeat delay")
	}
	interval, ok := found[key{"Key repeat interval", "macos"}]
	if !ok {
		t.Fatal("no macOS-gated Key repeat interval")
	}

	checks := []struct {
		s       Setting
		def     string
		min     float64
		max     float64
		storage string
	}{
		{delay, macKeyRepeatDelayDefaultMs, 120, 1800, "InitialKeyRepeat"},
		{interval, macKeyRepeatIntervalDefaultMs, 15, 120, "KeyRepeat"},
	}
	for _, c := range checks {
		s := c.s
		if s.Category != "Keyboard" || s.Control != ControlSlider {
			t.Errorf("%s: category %q control %v", s.Name, s.Category, s.Control)
		}
		if s.Default != c.def {
			t.Errorf("%s: default %q, want %q", s.Name, s.Default, c.def)
		}
		if s.Slider == nil || s.Slider.Min != c.min || s.Slider.Max != c.max || s.Slider.Step != 15 || s.Slider.Unit != "ms" {
			t.Errorf("%s: slider %+v", s.Name, s.Slider)
		}
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
