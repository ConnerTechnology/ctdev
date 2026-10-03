package cmd

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

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

func TestFormatSliderVal(t *testing.T) {
	tests := []struct {
		val  float64
		step float64
		want string
	}{
		{3.0, 1.0, "3"},
		{10.0, 5.0, "10"},
		{0.0, 1.0, "0"},
		{1.5, 0.5, "1.5"},
		{0.65, 0.05, "0.65"},
		{0.0, 0.1, "0"},
		{100.0, 25.0, "100"},
		{0.999, 0.001, "0.999"},
	}
	for _, tt := range tests {
		got := formatSliderVal(tt.val, tt.step)
		if got != tt.want {
			t.Errorf("formatSliderVal(%v, %v) = %q, want %q", tt.val, tt.step, got, tt.want)
		}
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
