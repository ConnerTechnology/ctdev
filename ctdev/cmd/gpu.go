package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/ConnerTechnology/dotfiles/ctdev/gpu"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
)

// GPU/NVIDIA driver signing lives under `ctdev configure gpu` (see
// configure_gpu.go) alongside the gpu settings category. These helpers carry the
// hardware/signing logic from the gpu package up to the command layer.

// showGPUStatus prints GPU hardware info and Secure Boot signing status — the
// read-only `ctdev configure gpu --show` view.
func showGPUStatus() error {
	info := platform.Detect()
	if info.OS != platform.Linux {
		return fmt.Errorf("GPU info is only supported on Linux")
	}

	headerStyle := styles.Header
	labelStyle := styles.Label(16)
	valueStyle := styles.Value
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

// runGPUSigning configures NVIDIA drivers and Secure Boot (MOK) signing. When
// recover is set it re-enrolls the MOK after a CMOS reset instead.
func runGPUSigning(ctx context.Context, recover bool) error {
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
	if ctx == nil {
		ctx = context.Background()
	}
	if recover {
		return gpu.RunRecover(ctx, opts)
	}
	return gpu.RunSetup(ctx, opts)
}
