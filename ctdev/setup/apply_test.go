package setup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/gpu"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

func TestDconfWriteArgs(t *testing.T) {
	got := dconfWriteArgs("/org/cinnamon/desktop/sound/event-sounds", "false")
	want := []string{"dconf", "write", "/org/cinnamon/desktop/sound/event-sounds", "false"}
	if len(got) != len(want) {
		t.Fatalf("got %d args, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
		}
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

func TestCpsToIntervalMs(t *testing.T) {
	tests := []struct {
		rate string
		want string
	}{
		{"25", "40"},   // 1000/25 = 40ms
		{"30", "33"},   // 1000/30 = 33ms (integer division)
		{"1", "1000"},  // 1000/1 = 1000ms
		{"50", "20"},   // 1000/50 = 20ms
		{"0", "0"},     // zero rate — return as-is
		{"-5", "-5"},   // negative — return as-is
		{"abc", "abc"}, // non-numeric — return as-is
		{"1000", "1"},  // 1000/1000 = 1ms
		{"2000", "0"},  // 1000/2000 = 0ms (integer division)
	}
	for _, tt := range tests {
		t.Run(tt.rate, func(t *testing.T) {
			got := cpsToIntervalMs(tt.rate)
			if got != tt.want {
				t.Errorf("cpsToIntervalMs(%q) = %q, want %q", tt.rate, got, tt.want)
			}
		})
	}
}

func TestGrubVarArgsSedPattern(t *testing.T) {
	// Verify the sed pattern structure for known safe values
	args := grubVarArgs("GRUB_TIMEOUT", "10")
	if len(args) != 4 {
		t.Fatalf("expected 4 args, got %d", len(args))
	}
	if args[0] != "sed" {
		t.Errorf("expected sed, got %s", args[0])
	}
	if args[3] != "/etc/default/grub" {
		t.Errorf("expected /etc/default/grub, got %s", args[3])
	}
	// Pattern uses | as delimiter to avoid conflicts with / in values.
	expected := "s|^GRUB_TIMEOUT=.*|GRUB_TIMEOUT=10|"
	if args[2] != expected {
		t.Errorf("got pattern %q, want %q", args[2], expected)
	}
}

func TestSedEscape(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"GRUB_TIMEOUT", "GRUB_TIMEOUT"},
		{"nvidia.NVreg_PreserveVideoMemoryAllocations=1", `nvidia\.NVreg_PreserveVideoMemoryAllocations=1`},
		{"simple", "simple"},
		{"a.b*c[d]", `a\.b\*c\[d\]`},
		{"^start$end", `\^start\$end`},
		{`back\slash`, `back\\slash`},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sedEscape(tt.input)
			if got != tt.want {
				t.Errorf("sedEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAppendGrubLine(t *testing.T) {
	tests := []struct {
		name    string
		content string
		varName string
		value   string
		want    string
	}{
		{
			name:    "content ends with newline",
			content: "GRUB_TIMEOUT=5\n",
			varName: "GRUB_CMDLINE_LINUX",
			value:   `""`,
			want:    "GRUB_TIMEOUT=5\nGRUB_CMDLINE_LINUX=\"\"\n",
		},
		{
			name:    "content missing trailing newline gets one",
			content: "GRUB_TIMEOUT=5",
			varName: "GRUB_CMDLINE_LINUX",
			value:   `""`,
			want:    "GRUB_TIMEOUT=5\nGRUB_CMDLINE_LINUX=\"\"\n",
		},
		{
			name:    "empty content does not add leading newline",
			content: "",
			varName: "GRUB_TIMEOUT",
			value:   "10",
			want:    "GRUB_TIMEOUT=10\n",
		},
		{
			name:    "value with quotes preserved",
			content: "GRUB_DEFAULT=0\n",
			varName: "GRUB_CMDLINE_LINUX",
			value:   `"quiet splash"`,
			want:    "GRUB_DEFAULT=0\nGRUB_CMDLINE_LINUX=\"quiet splash\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendGrubLine(tt.content, tt.varName, tt.value)
			if got != tt.want {
				t.Errorf("appendGrubLine = %q, want %q", got, tt.want)
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

func TestApplyNvidiaSigning_DryRunDoesNotExecuteRealSetup(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only GPU signing setup")
	}
	gpu.SetEFIDirForTest(t, t.TempDir())
	binDir := t.TempDir()
	writeStub := func(name, body string) {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeStub("mokutil", "#!/bin/sh\necho 'SecureBoot enabled'\n")
	writeStub("dkms", "#!/bin/sh\necho 'nvidia/570.00, 6.8.0-test, x86_64: installed'\n")
	writeStub("uname", "#!/bin/sh\necho '6.8.0-test'\n")
	// If DryRun were dropped, gpu.RunSetup would invoke sudo to generate MOK keys.
	writeStub("sudo", "#!/bin/sh\necho 'sudo invoked during dry-run' >&2\nexit 1\n")
	t.Setenv("PATH", binDir)

	var out bytes.Buffer
	err := applyNvidiaSigning(context.Background(), sysutil.Opts{Stdout: &out, DryRun: true})
	if err != nil {
		t.Fatalf("applyNvidiaSigning(DryRun: true) returned error: %v\noutput:\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "[dry-run]") {
		t.Errorf("expected [dry-run] output from forwarded DryRun flag, got:\n%s", out.String())
	}
}

func TestApplyNvidiaSuspendServices_EnablesDisabledSkipsStaticAndMissing(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("systemd is Linux-only")
	}
	dir := t.TempDir()
	// Stub systemctl:
	//   nvidia-suspend.service   -> "disabled" + exit 1 (must be enabled)
	//   nvidia-resume.service    -> "static"   + exit 0 (must be skipped)
	//   nvidia-hibernate.service -> ""         + exit 4 (missing unit, must be skipped)
	script := `#!/bin/sh
if [ "$1" = "is-enabled" ]; then
  case "$2" in
    nvidia-suspend.service) echo "disabled"; exit 1 ;;
    nvidia-resume.service) echo "static"; exit 0 ;;
    nvidia-hibernate.service) exit 4 ;;
  esac
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	var out bytes.Buffer
	if err := applyNvidiaSuspendServices(context.Background(), sysutil.Opts{Stdout: &out, DryRun: true}); err != nil {
		t.Fatalf("applyNvidiaSuspendServices: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "systemctl enable nvidia-suspend.service") {
		t.Errorf("disabled nvidia-suspend.service was not enabled; got:\n%s", got)
	}
	if strings.Contains(got, "nvidia-resume.service") {
		t.Errorf("static nvidia-resume.service should be skipped; got:\n%s", got)
	}
	if strings.Contains(got, "nvidia-hibernate.service") {
		t.Errorf("missing nvidia-hibernate.service should be skipped; got:\n%s", got)
	}
}
