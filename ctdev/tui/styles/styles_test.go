package styles

import "testing"

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

	greenDark, _, _, _ := func() (uint32, uint32, uint32, uint32) { return Green.RGBA() }()
	SetDarkBackground(false)
	greenLight, _, _, _ := Green.RGBA()
	if greenDark != greenLight {
		t.Errorf("brand accent Green must not change with background: %d vs %d", greenDark, greenLight)
	}
}
