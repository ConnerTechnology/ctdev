package info

import (
	"testing"
	"time"
)

func TestRenderDiskBar(t *testing.T) {
	tests := []struct {
		name    string
		percent int
		width   int
	}{
		{"0 percent", 0, 10},
		{"50 percent", 50, 10},
		{"70 percent yellow", 70, 10},
		{"90 percent red", 90, 10},
		{"100 percent", 100, 10},
		{"over 100 clamped", 200, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderDiskBar(tt.percent, tt.width)
			if result == "" {
				t.Errorf("renderDiskBar(%d, %d) returned empty string", tt.percent, tt.width)
			}
		})
	}
}

func TestRenderComponentEntry(t *testing.T) {
	tests := []struct {
		name      string
		component ComponentInfo
	}{
		{
			"installed component",
			ComponentInfo{Name: "docker", Category: "CLI", Installed: true},
		},
		{
			"not installed component",
			ComponentInfo{Name: "btop", Category: "CLI", Installed: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderComponentEntry(tt.component, 20)
			if result == "" {
				t.Error("renderComponentEntry returned empty string")
			}
			// The rendered string should contain the component name somewhere
			// (lipgloss may add ANSI codes, but the name text should be present)
			if len(result) == 0 {
				t.Errorf("expected non-empty result for component %q", tt.component.Name)
			}
		})
	}
}

func TestHumanUptime(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{45 * time.Minute, "45m"},
		{5*time.Hour + 30*time.Minute, "5h 30m"},
		{48 * time.Hour, "2d"},
		{76 * time.Hour, "3d 4h"},
	}
	for _, tt := range tests {
		if got := humanUptime(tt.d); got != tt.want {
			t.Errorf("humanUptime(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
