package cmd

import (
	"context"
	"fmt"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/gpu"
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
	checkPass := styles.Success.Render("✓")
	checkFail := styles.Error.Render("✗")

	fmt.Println(styles.Title.Render("GPU Status"))
	fmt.Println()
	fmt.Println(headerStyle.Render("Hardware"))

	gpu.ShowHardwareInfo(os.Stdout)

	fmt.Println()
	fmt.Println(headerStyle.Render("Signing Status"))

	for _, check := range gpu.GatherStatus() {
		icon := checkPass
		if !check.Pass {
			icon = checkFail
		}
		label := labelStyle.Render(check.Name + ":")
		detail := valueStyle.Render(check.Detail)
		fmt.Printf("  %s %s %s\n", icon, label, detail)
	}

	return nil
}

func runGPUSetup(cmd *cobra.Command, args []string) error {
	info := platform.Detect()
	if info.OS != platform.Linux {
		return fmt.Errorf("GPU setup is only supported on Linux")
	}

	opts := gpu.Opts{
		Stdout:  os.Stdout,
		Stdin:   os.Stdin,
		DryRun:  flagDryRun,
		Force:   flagForce,
		Verbose: flagVerbose,
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if flagGPURecover {
		return gpu.RunRecover(ctx, opts)
	}
	return gpu.RunSetup(ctx, opts)
}
