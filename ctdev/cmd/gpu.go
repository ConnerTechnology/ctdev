package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var flagGPURecover bool

var gpuCmd = &cobra.Command{
	Use:   "gpu",
	Short: "GPU management commands",
}

var gpuInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show GPU hardware info and signing status",
	RunE:  runGPUInfo,
}

var gpuSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure NVIDIA drivers and Secure Boot signing",
	RunE:  runGPUSetup,
}

func init() {
	gpuSetupCmd.Flags().BoolVar(&flagGPURecover, "recover", false, "re-enroll MOK after CMOS reset")
	gpuCmd.AddCommand(gpuInfoCmd)
	gpuCmd.AddCommand(gpuSetupCmd)
	rootCmd.AddCommand(gpuCmd)
}

func runGPUInfo(cmd *cobra.Command, args []string) error {
	info := platform.Detect()
	if info.OS != platform.Linux {
		return fmt.Errorf("GPU info is only supported on Linux")
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Orange)
	labelStyle := lipgloss.NewStyle().Foreground(styles.Subtle).Width(16)
	valueStyle := lipgloss.NewStyle().Foreground(styles.Bright)

	fmt.Println(styles.Title.Render("GPU Status"))
	fmt.Println()

	// GPU Model
	gpuModel := detectGPUModel()
	fmt.Printf("  %s %s\n", labelStyle.Render("Model:"), valueStyle.Render(gpuModel))

	// Driver version
	driverVer := detectNVIDIADriver()
	fmt.Printf("  %s %s\n", labelStyle.Render("Driver:"), valueStyle.Render(driverVer))

	// VRAM
	vram := detectVRAM()
	fmt.Printf("  %s %s\n", labelStyle.Render("VRAM:"), valueStyle.Render(vram))

	// Module signing
	moduleSigned := detectModuleSigning()
	fmt.Printf("  %s %s\n", labelStyle.Render("Module:"), formatStatus(moduleSigned))

	// Secure Boot
	secureBoot := detectSecureBootStatus()
	fmt.Printf("  %s %s\n", labelStyle.Render("Secure Boot:"), formatStatus(secureBoot))

	_ = headerStyle
	return nil
}

func runGPUSetup(cmd *cobra.Command, args []string) error {
	info := platform.Detect()
	if info.OS != platform.Linux {
		return fmt.Errorf("GPU setup is only supported on Linux")
	}

	if flagGPURecover {
		fmt.Println("Re-enrolling MOK after CMOS reset...")
		if flagDryRun {
			fmt.Println("  [dry-run] bash cmds/gpu-setup.sh --recover")
			return nil
		}
		script := fmt.Sprintf("%s/cmds/gpu-setup.sh", dotfilesRoot())
		return exec.Command("bash", script, "--recover").Run()
	}

	fmt.Println("Setting up NVIDIA GPU drivers...")
	if flagDryRun {
		fmt.Println("  [dry-run] bash cmds/gpu-setup.sh")
		return nil
	}
	script := fmt.Sprintf("%s/cmds/gpu-setup.sh", dotfilesRoot())
	c := exec.Command("bash", script)
	c.Stdout = nil
	c.Stderr = nil
	return c.Run()
}

func detectGPUModel() string {
	out, err := exec.Command("lspci").Output()
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "VGA") || strings.Contains(line, "3D") {
			parts := strings.SplitN(line, ": ", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "none detected"
}

func detectNVIDIADriver() string {
	out, err := exec.Command("nvidia-smi", "--query-gpu=driver_version", "--format=csv,noheader").Output()
	if err != nil {
		return "not installed"
	}
	return strings.TrimSpace(string(out))
}

func detectVRAM() string {
	out, err := exec.Command("nvidia-smi", "--query-gpu=memory.total", "--format=csv,noheader").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func detectModuleSigning() string {
	out, err := exec.Command("modinfo", "-F", "sig_id", "nvidia").Output()
	if err != nil {
		return "unknown"
	}
	if strings.TrimSpace(string(out)) != "" {
		return "signed"
	}
	return "unsigned"
}

func detectSecureBootStatus() string {
	out, err := exec.Command("mokutil", "--sb-state").Output()
	if err != nil {
		return "unknown"
	}
	if strings.Contains(string(out), "enabled") {
		return "enabled"
	}
	return "disabled"
}

func formatStatus(status string) string {
	switch status {
	case "signed", "enabled":
		return styles.Success.Render(status)
	case "unsigned", "disabled":
		return styles.Warning.Render(status)
	default:
		return styles.Dimmed.Render(status)
	}
}
