package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// rgb returns the 8-bit channels of the current Bright color (RGBA reports
// 16-bit values, so shift down a byte).
func rgb(t *testing.T) (r, g, b uint32) {
	t.Helper()
	rr, gg, bb, _ := Bright.RGBA()
	return rr >> 8, gg >> 8, bb >> 8
}

func TestSetDarkBackgroundFlipsPrimaryText(t *testing.T) {
	t.Cleanup(func() { SetDarkBackground(true) }) // restore default for other tests

	// Dark terminal: primary text is near-white.
	SetDarkBackground(true)
	if r, _, _ := rgb(t); r < 0x80 {
		t.Errorf("dark background: Bright should be light, got R=%#x", r)
	}

	// Light terminal: primary text flips to near-black.
	SetDarkBackground(false)
	if r, _, _ := rgb(t); r > 0x80 {
		t.Errorf("light background: Bright should be dark, got R=%#x", r)
	}
}

func TestBrandAccentsStayFixed(t *testing.T) {
	t.Cleanup(func() { SetDarkBackground(true) })

	SetDarkBackground(true)
	dark := []uint32{}
	for _, c := range []interface{ RGBA() (uint32, uint32, uint32, uint32) }{Green, Yellow, Red, Blue, Orange} {
		r, g, b, _ := c.RGBA()
		dark = append(dark, r, g, b)
	}
	SetDarkBackground(false)
	light := []uint32{}
	for _, c := range []interface{ RGBA() (uint32, uint32, uint32, uint32) }{Green, Yellow, Red, Blue, Orange} {
		r, g, b, _ := c.RGBA()
		light = append(light, r, g, b)
	}
	for i := range dark {
		if dark[i] != light[i] {
			t.Errorf("brand accent channel %d changed with background: %d vs %d", i, dark[i], light[i])
		}
	}
}

func TestDisableRemovesSGRColorCodes(t *testing.T) {
	Disable()
	t.Cleanup(Enable)

	for name, rendered := range map[string]string{
		"Error":   Error.Render("boom"),
		"Warning": Warning.Render("careful"),
		"Success": Success.Render("ok"),
		"Dimmed":  Dimmed.Render("note"),
	} {
		if strings.Contains(rendered, "\x1b[38;") || strings.Contains(rendered, "\x1b[48;") {
			t.Errorf("styles.%s still emits color SGR after Disable(): %q", name, rendered)
		}
		if ansi.Strip(rendered) == "" {
			t.Errorf("styles.%s stripped to empty", name)
		}
	}
}
