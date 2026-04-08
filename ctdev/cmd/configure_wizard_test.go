package cmd

import "testing"

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
