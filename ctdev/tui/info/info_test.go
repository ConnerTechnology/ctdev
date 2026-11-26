package info

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderDiskBar(t *testing.T) {
	tests := []struct {
		name       string
		percent    int
		width      int
		wantFilled int
		wantEmpty  int
	}{
		{"0 percent", 0, 10, 0, 10},
		{"50 percent", 50, 10, 5, 5},
		{"70 percent yellow", 70, 10, 7, 3},
		{"90 percent red", 90, 10, 9, 1},
		{"100 percent", 100, 10, 10, 0},
		{"over 100 clamped", 200, 10, 10, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plain := ansi.Strip(renderDiskBar(tt.percent, tt.width))
			if got := strings.Count(plain, "█"); got != tt.wantFilled {
				t.Errorf("renderDiskBar(%d, %d) filled = %d, want %d (%q)", tt.percent, tt.width, got, tt.wantFilled, plain)
			}
			if got := strings.Count(plain, "░"); got != tt.wantEmpty {
				t.Errorf("renderDiskBar(%d, %d) empty = %d, want %d (%q)", tt.percent, tt.width, got, tt.wantEmpty, plain)
			}
		})
	}
}

func TestRenderComponentEntry(t *testing.T) {
	tests := []struct {
		name      string
		component ComponentInfo
		wantGlyph string
	}{
		{
			"installed component",
			ComponentInfo{Name: "docker", Category: "CLI", Installed: true},
			"✓",
		},
		{
			"not installed component",
			ComponentInfo{Name: "btop", Category: "CLI", Installed: false},
			"○",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plain := ansi.Strip(renderComponentEntry(tt.component, 20))
			if !strings.Contains(plain, tt.component.Name) {
				t.Errorf("rendered entry %q missing component name %q", plain, tt.component.Name)
			}
			if !strings.Contains(plain, tt.wantGlyph) {
				t.Errorf("rendered entry %q missing glyph %q", plain, tt.wantGlyph)
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

func TestParseDiskInfo(t *testing.T) {
	// Real `df -Pk` shape: EFI partition under the floor, root out of order.
	df := "Filesystem 1024-blocks Used Available Capacity Mounted on\n" +
		"/dev/sda1   479668556    633072  454632220       1% /mnt/Linux-Data\n" +
		"/dev/nvme0n1p1  523248      6248     517000       2% /boot/efi\n" +
		"/dev/mapper/vgmint-root 1917499848 305340248 1514682176 17% /\n" +
		"/dev/nvme0n1p2 1719132  380928  1237572      25% /boot\n"
	got := parseDiskInfo(df)
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3 (EFI partition filtered): %v", len(got), got)
	}
	if got[0].Mount != "/" {
		t.Errorf("root should sort first, got %q", got[0].Mount)
	}
	if got[1].Mount != "/boot" || got[2].Mount != "/mnt/Linux-Data" {
		t.Errorf("remaining mounts should be alphabetical, got %q then %q", got[1].Mount, got[2].Mount)
	}
	if got[0].Percent != 17 || got[0].TotalKB != 1917499848 {
		t.Errorf("root row = %+v, want 17%% and 1917499848 KB", got[0])
	}
}

func TestDiskLabelText(t *testing.T) {
	if got := diskLabelText("/"); got != "/ (system)" {
		t.Errorf("root label = %q, want %q", got, "/ (system)")
	}
	for _, m := range []string{"/boot", "/mnt/Timeshift"} {
		if got := diskLabelText(m); got != m {
			t.Errorf("diskLabelText(%q) = %q, want it unchanged", m, got)
		}
	}
}
