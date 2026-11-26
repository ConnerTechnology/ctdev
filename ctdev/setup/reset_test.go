package setup

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

func TestResetLinuxDefaultsDryRun(t *testing.T) {
	var buf bytes.Buffer
	err := ResetLinuxDefaults(context.Background(), sysutil.Opts{Stdout: &buf, DryRun: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "DRY-RUN") {
		t.Error("expected DRY-RUN in output")
	}
	// Should mention the major categories being reset
	for _, keyword := range []string{"Power", "Screensaver", "Keyboard", "boot settings"} {
		if !strings.Contains(output, keyword) {
			t.Errorf("expected %q in dry-run output", keyword)
		}
	}
}

func TestGrubRemoveParamArgs_RealSed(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GNU sed -i syntax test")
	}
	if _, err := exec.LookPath("sed"); err != nil {
		t.Skip("sed not installed")
	}

	tests := []struct {
		name  string
		input string
		param string
		want  string
	}{
		{
			name:  "first token followed by others",
			input: "GRUB_CMDLINE_LINUX_DEFAULT=\"pcie_aspm=off quiet splash\"\n",
			param: "pcie_aspm=off",
			want:  "GRUB_CMDLINE_LINUX_DEFAULT=\"quiet splash\"\n",
		},
		{
			name:  "middle token",
			input: "GRUB_CMDLINE_LINUX_DEFAULT=\"quiet pcie_aspm=off splash\"\n",
			param: "pcie_aspm=off",
			want:  "GRUB_CMDLINE_LINUX_DEFAULT=\"quiet splash\"\n",
		},
		{
			name:  "last token with dot in param",
			input: "GRUB_CMDLINE_LINUX_DEFAULT=\"quiet nvidia.NVreg_PreserveVideoMemoryAllocations=1\"\n",
			param: "nvidia.NVreg_PreserveVideoMemoryAllocations=1",
			want:  "GRUB_CMDLINE_LINUX_DEFAULT=\"quiet\"\n",
		},
		{
			name:  "sole token",
			input: "GRUB_CMDLINE_LINUX=\"pcie_aspm=off\"\n",
			param: "pcie_aspm=off",
			want:  "GRUB_CMDLINE_LINUX=\"\"\n",
		},
		{
			name:  "does not touch comments or unrelated lines",
			input: "# keep pcie_aspm=off in comments\nOTHER_VAR=\"pcie_aspm=off\"\nGRUB_CMDLINE_LINUX_DEFAULT=\"pcie_aspm=off quiet\"\n",
			param: "pcie_aspm=off",
			want:  "# keep pcie_aspm=off in comments\nOTHER_VAR=\"pcie_aspm=off\"\nGRUB_CMDLINE_LINUX_DEFAULT=\"quiet\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "grub")
			if err := os.WriteFile(path, []byte(tt.input), 0o644); err != nil {
				t.Fatal(err)
			}
			args := grubRemoveParamArgs(tt.param, path)
			out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
			if err != nil {
				t.Fatalf("sed failed: %v\n%s", err, out)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", string(got), tt.want)
			}
		})
	}
}

func TestResetLinuxDefaults_NonDryRunInvokesResetCommands(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only reset path")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	grubPath := filepath.Join(dir, "grub")
	if err := os.WriteFile(grubPath, []byte("GRUB_TIMEOUT_STYLE=hidden\nGRUB_TIMEOUT=0\nGRUB_DISABLE_OS_PROBER=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	origGrub := grubDefaultPath
	grubDefaultPath = grubPath
	t.Cleanup(func() { grubDefaultPath = origGrub })

	home := t.TempDir()
	t.Setenv("HOME", home)

	// Stub every external command ResetLinuxDefaults invokes so we exercise
	// the real non-dry-run control flow without touching the host.
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stub := "#!/bin/sh\necho \"${0##*/} $*\" >> \"" + logPath + "\"\nexit 0\n"
	sudoStub := "#!/bin/sh\necho \"sudo $*\" >> \"" + logPath + "\"\nexit 0\n"
	for _, name := range []string{
		"powerprofilesctl", "dconf", "gsettings", "xset", "lsmod",
		"cp", "sed", "udevadm", "systemctl", "update-grub", "loginctl", "ufw",
	} {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(stub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(binDir, "sudo"), []byte(sudoStub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	var out bytes.Buffer
	if err := ResetLinuxDefaults(context.Background(), sysutil.Opts{Stdout: &out}); err != nil {
		t.Fatalf("ResetLinuxDefaults: %v", err)
	}
	if !strings.Contains(out.String(), "Linux Mint defaults reset to system defaults") {
		t.Errorf("missing completion message in output:\n%s", out.String())
	}
	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logStr := string(calls)
	for _, want := range []string{
		"powerprofilesctl set balanced",
		"dconf reset /org/cinnamon/settings-daemon/plugins/power/sleep-display-ac",
		"gsettings reset org.cinnamon.desktop.peripherals.keyboard repeat",
		"xset r rate",
		"update-grub",
	} {
		if !strings.Contains(logStr, want) {
			t.Errorf("expected call %q in log:\n%s", want, logStr)
		}
	}
}
