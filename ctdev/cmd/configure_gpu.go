package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var flagGPURecover bool

// configureGPUCmd replaces the old top-level `ctdev gpu` command. Driver signing
// is the gpu category's "NVIDIA driver works with Secure Boot" setting, so the
// settings pass is the one place it runs; --recover is the only path that
// calls into gpu directly.
var configureGPUCmd = &cobra.Command{
	Use:   "gpu",
	Short: "Configure GPU settings and NVIDIA/Secure Boot driver signing",
	Long: "Apply the GPU settings category and configure NVIDIA drivers + Secure Boot (MOK) signing. " +
		"Use --show to see GPU hardware and signing status, or --recover to re-enroll the MOK after a CMOS reset.",
	RunE: runConfigureGPU,
}

func init() {
	configureGPUCmd.Flags().BoolVar(&flagGPURecover, "recover", false, "re-enroll MOK after CMOS reset")
	configureCmd.AddCommand(configureGPUCmd)
}

func runConfigureGPU(cmd *cobra.Command, args []string) error {
	ctx := cmdContext(cmd)

	// --show: read-only hardware + signing status, then the category's settings.
	if flagConfigShow {
		if err := showGPUStatus(ctx); err != nil {
			return err
		}
		fmt.Println()
		return runCategoryWizard(ctx, "gpu", true)
	}

	if !flagDryRun {
		if err := ensureSudo(cmdContext(cmd)); err != nil {
			return fmt.Errorf("sudo required: %w", err)
		}
	}

	// --recover is a targeted MOK re-enrollment; skip the settings pass.
	if flagGPURecover {
		return runGPURecover(ctx)
	}

	if isBatchMode() {
		return runCategoryBatch(ctx, "gpu")
	}
	return runCategoryWizard(ctx, "gpu", false)
}
