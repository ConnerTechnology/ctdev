package setup

import "testing"

func TestNextValue(t *testing.T) {
	picker := &Setting{Control: ControlPicker, Choices: []PickerChoice{{Value: "a"}, {Value: "b"}, {Value: "c"}}}
	if got := NextValue(picker, "a"); got != "b" {
		t.Errorf("picker a → %q, want b", got)
	}
	if got := NextValue(picker, "c"); got != "a" {
		t.Errorf("picker wraps c → %q, want a", got)
	}
	if got := NextValue(picker, "unknown"); got != "a" {
		t.Errorf("picker unknown → %q, want first choice", got)
	}

	toggle := &Setting{Control: ControlToggle, Default: "enabled"}
	if got := NextValue(toggle, "enabled"); got != "disabled" {
		t.Errorf("toggle enabled → %q, want disabled", got)
	}
	if got := NextValue(toggle, "disabled"); got != "enabled" {
		t.Errorf("toggle disabled → %q, want enabled", got)
	}
}

func TestAdjustSlider(t *testing.T) {
	s := &Setting{Control: ControlSlider, Default: "20", Slider: &SliderRange{Min: 0, Max: 30, Step: 10}}
	if got := AdjustSlider(s, "10", 1); got != "20" {
		t.Errorf("10+step = %q, want 20", got)
	}
	if got := AdjustSlider(s, "30", 1); got != "30" {
		t.Errorf("clamp at max: got %q, want 30", got)
	}
	if got := AdjustSlider(s, "0", -1); got != "0" {
		t.Errorf("clamp at min: got %q, want 0", got)
	}
	if got := AdjustSlider(s, "garbage", 1); got != "30" {
		t.Errorf("unparseable starts from default+step: got %q, want 30", got)
	}

	frac := &Setting{Control: ControlSlider, Default: "0.65", Slider: &SliderRange{Min: 0, Max: 1, Step: 0.05}}
	if got := AdjustSlider(frac, "0.65", 1); got != "0.7" {
		t.Errorf("0.65+0.05 = %q, want 0.7", got)
	}
}
