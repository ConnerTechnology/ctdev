package setup

import (
	"math"
	"slices"
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

// SliderNever is the value a slider with SliderRange.Never shows and stores
// for "never", one step below its Min.
const SliderNever = "never"

// SliderRangeLabel renders a slider's valid range, e.g. "5–120min", or
// "never, 5–120min" when the slider offers never.
func SliderRangeLabel(r *SliderRange) string {
	label := FormatSliderVal(r.Min, r.Step) + "–" + FormatSliderVal(r.Max, r.Step) + r.Unit
	if r.Never {
		return SliderNever + ", " + label
	}
	return label
}

// DisplayValue renders a setting's value for people: a slider's number gets
// its unit ("60 min"); anything else, including SliderNever, is shown as is.
func DisplayValue(s *Setting, v string) string {
	if s.Control != ControlSlider || s.Slider == nil || s.Slider.Unit == "" {
		return v
	}
	if _, err := strconv.ParseFloat(v, 64); err != nil {
		return v
	}
	return v + " " + s.Slider.Unit
}

// AdjustSlider steps a slider setting by dir (±1) steps, clamped to its range
// and snapped to its step size. Non-sliders and unparseable values return cur.
func AdjustSlider(s *Setting, cur string, dir int) string {
	r := s.Slider
	if s.Control != ControlSlider || r == nil {
		return cur
	}
	start := cur
	if _, err := strconv.ParseFloat(start, 64); err != nil && !(r.Never && start == SliderNever) {
		// An undetectable current value starts from the recommended one.
		start = s.Default
	}
	if r.Never && start == SliderNever {
		if dir > 0 {
			return FormatSliderVal(r.Min, r.Step)
		}
		return SliderNever
	}
	val, err := strconv.ParseFloat(start, 64)
	if err != nil {
		return cur
	}
	val += float64(dir) * r.Step
	val = math.Round(val/r.Step) * r.Step
	if val < r.Min {
		if r.Never {
			return SliderNever
		}
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

// PicksNone is a ControlMultiPick value with nothing checked.
const PicksNone = "none"

// JoinPicks returns the ControlMultiPick value for picks: sorted, deduplicated
// and comma-joined, or PicksNone when empty.
func JoinPicks(picks []string) string {
	var out []string
	for _, p := range picks {
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return PicksNone
	}
	slices.Sort(out)
	return strings.Join(slices.Compact(out), ",")
}

// SplitPicks returns the checked values in a ControlMultiPick value.
func SplitPicks(value string) []string {
	if value == "" || value == PicksNone {
		return nil
	}
	return strings.Split(value, ",")
}
