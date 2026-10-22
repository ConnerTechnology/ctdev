package cmd

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/ConnerTechnology/ctdev/ctdev/setup"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// captureStdout runs fn with os.Stdout redirected to a buffer and returns
// whatever fn wrote. Used so we can assert on fmt.Println output from the
// configure wizard without adding a writer parameter to every helper.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	w.Close()
	os.Stdout = old
	return <-done
}

func TestSlugOrder_DerivedFromRegistry(t *testing.T) {
	want := setup.Slugs(setup.Registry)
	if len(slugOrder) != len(want) {
		t.Fatalf("slugOrder length = %d, want %d (derived from setup.Slugs)", len(slugOrder), len(want))
	}
	for i := range want {
		if slugOrder[i] != want[i] {
			t.Errorf("slugOrder[%d] = %q, want %q", i, slugOrder[i], want[i])
		}
	}
}

func TestSlugDescription_FallsBackToSlug(t *testing.T) {
	if got := slugDescription("gpu"); got != "GPU & NVIDIA" {
		t.Errorf("gpu description = %q, want %q", got, "GPU & NVIDIA")
	}
	if got := slugDescription("brand-new-slug"); got != "brand-new-slug" {
		t.Errorf("unknown slug should fall through to itself; got %q", got)
	}
}

func TestRunCategoryWizardOn_EmptySettingsPrintsNotice(t *testing.T) {
	// Registry with a setting whose hardware gate always returns false.
	reg := []setup.Setting{
		{
			Name:       "hidden-only",
			Slug:       "phantom",
			HardwareFn: func() bool { return false },
		},
	}
	out := captureStdout(t, func() {
		if err := runCategoryWizardOn(context.Background(), reg, "phantom", true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "No applicable phantom settings") {
		t.Errorf("expected 'No applicable' notice; got:\n%s", out)
	}
}

func TestReadLineCtx_ReadsFromStdinScanner(t *testing.T) {
	orig := stdinScanner
	t.Cleanup(func() { stdinScanner = orig })

	stdinScanner = bufio.NewScanner(strings.NewReader("  hello world  \nnext line\n"))
	ctx := context.Background()

	if got, ok := readLineCtx(ctx); !ok || got != "hello world" {
		t.Errorf("first call = %q, %v; want %q, true (whitespace trimmed)", got, ok, "hello world")
	}
	if got, ok := readLineCtx(ctx); !ok || got != "next line" {
		t.Errorf("second call = %q, %v; want %q, true", got, ok, "next line")
	}
	if got, ok := readLineCtx(ctx); ok || got != "" {
		t.Errorf("eof call = %q, %v; want empty string, false", got, ok)
	}
}

func TestReadLineCtx_CancelledContextReturnsNotOK(t *testing.T) {
	orig := stdinScanner
	t.Cleanup(func() { stdinScanner = orig })

	// A reader that never delivers a line, so only cancellation can end the read.
	r, w := io.Pipe()
	t.Cleanup(func() { w.Close() })
	stdinScanner = bufio.NewScanner(r)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, ok := readLineCtx(ctx); ok || got != "" {
		t.Errorf("cancelled read = %q, %v; want empty string, false", got, ok)
	}
}

func TestRunCategoryWizardOn_ShowOnlyRendersSetting(t *testing.T) {
	reg := []setup.Setting{
		{
			Name:        "test-setting",
			Slug:        "testslug",
			Description: "a test",
			Control:     setup.ControlToggle,
			Default:     "enabled",
			DetectFunc:  func(context.Context) string { return "enabled" },
		},
	}
	out := captureStdout(t, func() {
		if err := runCategoryWizardOn(context.Background(), reg, "testslug", true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "test-setting") {
		t.Errorf("expected setting name in output; got:\n%s", out)
	}
	if strings.Contains(out, "No applicable") {
		t.Errorf("should not emit 'No applicable' when settings exist; got:\n%s", out)
	}
}

func TestPromptSlider_EnterDefaults(t *testing.T) {
	orig := stdinScanner
	t.Cleanup(func() { stdinScanner = orig })
	ctx := context.Background()
	slider := &setup.Setting{
		Name:    "Test slider",
		Control: setup.ControlSlider,
		Default: "195",
		Slider:  &setup.SliderRange{Min: 120, Max: 1800, Step: 15, Unit: "ms"},
	}

	tests := []struct {
		name    string
		current string
		desired string
		want    string
	}{
		{"unset takes the recommended value", "", "195", "195"},
		{"unset falls back to Default", "", "", "195"},
		{"set keeps the current value", "225", "195", "225"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdinScanner = bufio.NewScanner(strings.NewReader("\n"))
			st := &setup.SettingState{Setting: slider, CurrentValue: tt.current, DesiredValue: tt.desired}
			var got string
			out := captureStdout(t, func() {
				var err error
				got, err = promptSlider(ctx, st)
				if err != nil {
					t.Errorf("promptSlider: %v", err)
				}
			})
			if got != tt.want {
				t.Errorf("promptSlider = %q, want %q", got, tt.want)
			}
			if !strings.Contains(out, "["+tt.want+"]") {
				t.Errorf("prompt should show [%s]; got %q", tt.want, out)
			}
		})
	}

	// Through promptSetting, an unset slider plus Enter is queued for apply.
	stdinScanner = bufio.NewScanner(strings.NewReader("\n"))
	st := &setup.SettingState{Setting: slider, CurrentValue: "", DesiredValue: "195", Enabled: true}
	var changed bool
	captureStdout(t, func() {
		var err error
		changed, err = promptSetting(ctx, st)
		if err != nil {
			t.Errorf("promptSetting: %v", err)
		}
	})
	if !changed || !st.Enabled || st.DesiredValue != "195" {
		t.Errorf("promptSetting: changed=%v enabled=%v desired=%q, want true/true/195", changed, st.Enabled, st.DesiredValue)
	}
}

func TestApplySettings_MacKeyRepeatNotePrintsOnce(t *testing.T) {
	// The registry's macOS key repeat settings, with their real ApplyGroup and
	// post-apply hook. ApplyFunc is stubbed so nothing is written to this
	// machine's defaults. They are the only keyboard settings with a group.
	var settings []setup.Setting
	for _, s := range setup.FilterBySlug(setup.Registry, "keyboard") {
		if s.ApplyGroup == "" {
			continue
		}
		if setup.PostApplyHooks[s.ApplyGroup] == nil {
			t.Fatalf("%s: no post-apply hook for group %q", s.Name, s.ApplyGroup)
		}
		s.ApplyFunc = func(context.Context, sysutil.Opts, string) error { return nil }
		settings = append(settings, s)
	}
	if len(settings) != 2 {
		t.Fatalf("expected the 2 macOS key repeat settings, got %d", len(settings))
	}
	states := make([]setup.SettingState, len(settings))
	for i := range settings {
		states[i] = setup.SettingState{Setting: &settings[i], DesiredValue: settings[i].Default, Enabled: true}
	}

	out := captureStdout(t, func() {
		if err := applySettings(context.Background(), states, false, false, false); err != nil {
			t.Errorf("applySettings: %v", err)
		}
	})
	if n := strings.Count(out, "log out and back in"); n != 1 {
		t.Errorf("log-out note printed %d times, want 1; output:\n%s", n, out)
	}
	if !strings.Contains(out, "2 applied") {
		t.Errorf("expected 2 applied; output:\n%s", out)
	}
}

func TestPromptPicker_EnterDefault(t *testing.T) {
	orig := stdinScanner
	t.Cleanup(func() { stdinScanner = orig })
	picker := &setup.Setting{
		Name:    "Test picker",
		Control: setup.ControlPicker,
		Default: "67",
		Choices: []setup.PickerChoice{{Value: "67", Description: "fast"}, {Value: "33", Description: "slower"}},
	}
	tests := []struct {
		name    string
		current string
		label   string
	}{
		{"matching current shows its number", "33", "[2]"},
		{"unmatched current shows the value Enter keeps", "6", "[6]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdinScanner = bufio.NewScanner(strings.NewReader("\n"))
			st := &setup.SettingState{Setting: picker, CurrentValue: tt.current}
			var got string
			out := captureStdout(t, func() {
				var err error
				got, err = promptPicker(context.Background(), st)
				if err != nil {
					t.Errorf("promptPicker: %v", err)
				}
			})
			if got != tt.current {
				t.Errorf("Enter returned %q, want current %q", got, tt.current)
			}
			if !strings.Contains(out, "Select "+tt.label) {
				t.Errorf("prompt should show Select %s; got %q", tt.label, out)
			}
		})
	}
}

func TestPromptSlider_Never(t *testing.T) {
	orig := stdinScanner
	t.Cleanup(func() { stdinScanner = orig })
	ctx := context.Background()
	slider := &setup.Setting{
		Name:    "Test timer",
		Control: setup.ControlSlider,
		Default: setup.SliderNever,
		Slider:  &setup.SliderRange{Min: 5, Max: 120, Step: 5, Unit: "min", Never: true},
	}

	tests := []struct{ input, want string }{
		{"never", setup.SliderNever},
		{"Never", setup.SliderNever},
		{"0", setup.SliderNever},
		{"3", "5"},
		{"45", "45"},
		{"\n", "60"}, // Enter keeps the current value
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			stdinScanner = bufio.NewScanner(strings.NewReader(tt.input + "\n"))
			st := &setup.SettingState{Setting: slider, CurrentValue: "60", DesiredValue: setup.SliderNever}
			var got string
			out := captureStdout(t, func() {
				var err error
				got, err = promptSlider(ctx, st)
				if err != nil {
					t.Errorf("promptSlider: %v", err)
				}
			})
			if got != tt.want {
				t.Errorf("promptSlider(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if !strings.Contains(out, "never, 5–120min") {
				t.Errorf("range should offer never; got %q", out)
			}
		})
	}
}

func TestShowSetting_SliderUnit(t *testing.T) {
	s := &setup.Setting{
		Name:    "Test timer",
		Control: setup.ControlSlider,
		Default: setup.SliderNever,
		Slider:  &setup.SliderRange{Min: 5, Max: 120, Step: 5, Unit: "min", Never: true},
	}
	out := captureStdout(t, func() { showSetting(&setup.SettingState{Setting: s, CurrentValue: "60"}, wizardLabelWidth) })
	if !strings.Contains(out, "60 min") || !strings.Contains(out, "recommended: never") {
		t.Errorf("show = %q, want 60 min and recommended: never", out)
	}
	out = captureStdout(t, func() {
		showSetting(&setup.SettingState{Setting: s, CurrentValue: setup.SliderNever}, wizardLabelWidth)
	})
	if strings.Contains(out, "never min") || strings.Contains(out, "recommended") {
		t.Errorf("show = %q, want a bare never at the recommended value", out)
	}
}

func TestShowSetting_NotNeeded(t *testing.T) {
	s := &setup.Setting{Name: "Lock on suspend", Control: setup.ControlToggle, Default: "true"}
	st := &setup.SettingState{Setting: s, CurrentValue: "false", NotNeeded: "suspend is turned off, so this has no effect"}
	out := ansi.Strip(captureStdout(t, func() { showSetting(st, wizardLabelWidth) }))
	if !strings.Contains(out, "(not needed: suspend is turned off, so this has no effect)") || strings.Contains(out, "recommended") {
		t.Errorf("show = %q, want the not-needed reason instead of a recommendation", out)
	}
}

func TestPromptSetting_NotNeededDefaultsToSkip(t *testing.T) {
	orig := stdinScanner
	t.Cleanup(func() { stdinScanner = orig })
	ctx := context.Background()
	s := &setup.Setting{Name: "Suspend timer", Control: setup.ControlSlider, Default: setup.SliderNever,
		Slider: &setup.SliderRange{Min: 5, Max: 120, Step: 5, Unit: "min", Never: true}}
	reason := "suspend is turned off, so this has no effect"

	// Enter keeps it out of the apply set.
	stdinScanner = bufio.NewScanner(strings.NewReader("\n"))
	st := &setup.SettingState{Setting: s, CurrentValue: "30", DesiredValue: s.Default, NotNeeded: reason}
	var changed bool
	out := ansi.Strip(captureStdout(t, func() {
		var err error
		changed, err = promptSetting(ctx, st)
		if err != nil {
			t.Errorf("promptSetting: %v", err)
		}
	}))
	if changed || st.Enabled {
		t.Errorf("Enter on a not-needed setting: changed=%v enabled=%v, want false/false", changed, st.Enabled)
	}
	if !strings.Contains(out, "Not needed on this machine: "+reason) {
		t.Errorf("wizard should give the reason; got %q", out)
	}
	if strings.Contains(out, "recommended") {
		t.Errorf("wizard shouldn't recommend a setting this machine doesn't need; got %q", out)
	}

	// Yes, then a value, changes it anyway.
	stdinScanner = bufio.NewScanner(strings.NewReader("y\nnever\n"))
	st = &setup.SettingState{Setting: s, CurrentValue: "30", DesiredValue: s.Default, NotNeeded: reason}
	captureStdout(t, func() {
		var err error
		changed, err = promptSetting(ctx, st)
		if err != nil {
			t.Errorf("promptSetting: %v", err)
		}
	})
	if !changed || !st.Enabled || st.DesiredValue != setup.SliderNever {
		t.Errorf("yes on a not-needed setting: changed=%v enabled=%v desired=%q, want true/true/never", changed, st.Enabled, st.DesiredValue)
	}
}
