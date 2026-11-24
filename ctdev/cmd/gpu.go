package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/ConnerTechnology/ctdev/ctdev/gpu"
	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// GPU/NVIDIA driver signing lives under `ctdev configure gpu` (see
// configure_gpu.go) alongside the gpu settings category. These helpers carry the
// hardware/signing logic from the gpu package up to the command layer.

// showGPUStatus prints GPU hardware info and Secure Boot signing status — the
// read-only `ctdev configure gpu --show` view.
func showGPUStatus(ctx context.Context) error {
	info := platform.Detect()
	if info.OS != platform.Linux {
		return fmt.Errorf("GPU info is only supported on Linux")
	}

	fmt.Println(styles.Title.Render("GPU Status"))
	fmt.Println()
	fmt.Println(styles.Header.Render("Hardware"))

	gpu.ShowHardwareInfo(ctx, os.Stdout)

	fmt.Println()
	fmt.Println(styles.Header.Render("Signing Status"))
	printSigningStatus(gpu.GatherStatus(ctx))
	return nil
}

// printSigningStatus prints one row per check, the label column as wide as
// the longest label so none wraps.
func printSigningStatus(checks []gpu.StatusCheck) {
	width := 0
	for _, check := range checks {
		width = max(width, len(check.Name)+1)
	}
	labelStyle := styles.Label(width)
	for _, check := range checks {
		icon := styles.Success.Render("✓")
		if !check.Pass {
			icon = styles.Error.Render("✗")
		}
		fmt.Printf("  %s %s %s\n", icon, labelStyle.Render(check.Name+":"), styles.Value.Render(check.Detail))
	}
}

// runGPURecover re-enrolls the MOK after a CMOS reset.
func runGPURecover(ctx context.Context) error {
	if platform.Detect().OS != platform.Linux {
		return fmt.Errorf("GPU setup is only supported on Linux")
	}
	return gpu.RunRecover(ctx, gpu.Opts{
		Stdout:  os.Stdout,
		Stdin:   os.Stdin,
		DryRun:  flagDryRun,
		Verbose: flagVerbose,
	})
}
