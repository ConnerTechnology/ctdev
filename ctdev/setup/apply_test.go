package setup

import (
	"strings"
	"testing"
)

func TestGrubVarCommand(t *testing.T) {
	args := grubVarArgs("GRUB_TIMEOUT", "10")
	if len(args) == 0 {
		t.Error("expected non-empty args")
	}
}

func TestDconfWriteArgs(t *testing.T) {
	args := dconfWriteArgs("/org/cinnamon/desktop/sound/event-sounds", "false")
	if args[0] != "dconf" {
		t.Errorf("expected dconf, got %s", args[0])
	}
}

func TestContainsGrubVar(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		varName  string
		expected bool
	}{
		{"present uncommented", "GRUB_TIMEOUT=10\n", "GRUB_TIMEOUT", true},
		{"commented out", "# GRUB_TIMEOUT=10\n", "GRUB_TIMEOUT", false},
		{"absent", "GRUB_DEFAULT=0\n", "GRUB_TIMEOUT", false},
		{"target in middle", "GRUB_DEFAULT=0\nGRUB_TIMEOUT=5\nGRUB_CMDLINE=\"quiet\"\n", "GRUB_TIMEOUT", true},
		{"hash-commented line", "#GRUB_TIMEOUT=10\n", "GRUB_TIMEOUT", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsGrubVar(tt.content, tt.varName); got != tt.expected {
				t.Errorf("containsGrubVar(%q, %q) = %v, want %v", tt.content, tt.varName, got, tt.expected)
			}
		})
	}
}

func TestEmbeddedUdevConfigs(t *testing.T) {
	files := []struct {
		path     string
		contains string
	}{
		{"configs/udev/99-logitech-kvm-fix.rules", "046d"},
		{"configs/udev/99-hide-drives.rules", "UDISKS_IGNORE"},
		{"configs/udev/solaar-restart.service", "solaar"},
	}
	for _, f := range files {
		t.Run(f.path, func(t *testing.T) {
			content, err := Configs.ReadFile(f.path)
			if err != nil {
				t.Fatalf("failed to read embedded file %s: %v", f.path, err)
			}
			if len(content) == 0 {
				t.Errorf("embedded file %s is empty", f.path)
			}
			if !strings.Contains(string(content), f.contains) {
				t.Errorf("embedded file %s missing expected content %q", f.path, f.contains)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty string", "", nil},
		{"single line no newline", "hello", []string{"hello"}},
		{"trailing newline", "hello\n", []string{"hello"}},
		{"multiple lines", "a\nb\nc", []string{"a", "b", "c"}},
		{"multiple lines trailing newline", "a\nb\n", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("splitLines(%q) = %v (len %d), want %v (len %d)", tt.input, got, len(got), tt.expected, len(tt.expected))
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("index %d: got %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}
