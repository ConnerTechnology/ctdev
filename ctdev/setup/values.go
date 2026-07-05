package setup

import (
	"math"
	"strconv"
	"strings"
)

// Value-manipulation helpers shared by the settings UIs: cycling a picker or
// toggle to its next value, stepping a slider, and formatting slider values.
// Pure functions so every UI treats values identically.

// togglePairs maps a toggle's "on" rendering to its "off" counterpart. The
// wizard renders whichever pair matches the setting's Default/current value.
var togglePairs = [][2]string{
	{"true", "false"},
	{"enabled", "disabled"},
	{"installed", "not installed"},
	{"signed", "unsigned"},
	{"active", "inactive"},
}

// TogglePair returns the on/off value pair for a toggle setting, derived from
// its Default and current value. Falls back to (Default, "other").
func TogglePair(s *Setting, current string) (on, off string) {
	for _, pair := range togglePairs {
		for i := range pair {
			if s.Default == pair[i] || current == pair[i] {
				return pair[0], pair[1]
			}
		}
	}
	return s.Default, "other"
}

// NextValue cycles a picker or toggle setting to the value after cur.
// Sliders and unknown values return cur unchanged.
func NextValue(s *Setting, cur string) string {
	switch s.Control {
	case ControlPicker:
		if len(s.Choices) == 0 {
			return cur
		}
		for i, c := range s.Choices {
			if c.Value == cur {
				return s.Choices[(i+1)%len(s.Choices)].Value
			}
		}
		return s.Choices[0].Value
	case ControlToggle:
		on, off := TogglePair(s, cur)
		if cur == on {
			return off
		}
		return on
	}
	return cur
}

// AdjustSlider steps a slider setting by dir (±1) steps, clamped to its range
// and snapped to its step size. Non-sliders and unparseable values return cur.
func AdjustSlider(s *Setting, cur string, dir int) string {
	r := s.Slider
	if s.Control != ControlSlider || r == nil {
		return cur
	}
	val, err := strconv.ParseFloat(cur, 64)
	if err != nil {
		// An undetectable current value starts from the recommended one.
		val, err = strconv.ParseFloat(s.Default, 64)
		if err != nil {
			return cur
		}
	}
	val += float64(dir) * r.Step
	val = math.Round(val/r.Step) * r.Step
	if val < r.Min {
		val = r.Min
	}
	if val > r.Max {
		val = r.Max
	}
	return FormatSliderVal(val, r.Step)
}

// FormatSliderVal formats a slider value, using integer format when the step
// is >= 1 and the step's decimal precision otherwise — so float artifacts
// (0.65+0.05 = 0.7000000000000001) never leak into the rendered value.
func FormatSliderVal(val, step float64) string {
	if step >= 1 {
		return strconv.Itoa(int(math.Round(val)))
	}
	decimals := 0
	for s := step; s < 1 && decimals < 10; s *= 10 {
		decimals++
	}
	out := strconv.FormatFloat(val, 'f', decimals, 64)
	out = strings.TrimRight(out, "0")
	out = strings.TrimRight(out, ".")
	if out == "" {
		return "0"
	}
	return out
}
