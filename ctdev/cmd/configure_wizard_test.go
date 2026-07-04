package cmd

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/ConnerTechnology/dotfiles/ctdev/setup"
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
			DetectFunc:  func() string { return "enabled" },
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
