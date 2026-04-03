package gpu

import "testing"

func TestMibToGB(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1024", "1.0"},
		{"0", "0.0"},
		{"512", "0.5"},
		{"bad", "?"},
		{"", "?"},
	}
	for _, tt := range tests {
		if got := mibToGB(tt.input); got != tt.want {
			t.Errorf("mibToGB(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestOrDefault(t *testing.T) {
	tests := []struct {
		s, def, want string
	}{
		{"", "fallback", "fallback"},
		{"value", "fallback", "value"},
	}
	for _, tt := range tests {
		if got := orDefault(tt.s, tt.def); got != tt.want {
			t.Errorf("orDefault(%q, %q) = %q, want %q", tt.s, tt.def, got, tt.want)
		}
	}
}
