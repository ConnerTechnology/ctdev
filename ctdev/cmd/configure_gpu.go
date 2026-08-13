package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var flagGPURecover bool

// configureGPUCmd replaces the old top-level `ctdev gpu` command. `gpu` is also a
// settings category in setup.Registry (NVIDIA suspend services, etc.), so this
// runs the category settings first and then the NVIDIA/MOK driver-signing setup
// — the signing flow only fires on an explicit `configure gpu`, never in the
// full `ctdev configure` sweep (runConfigureAll handles gpu as settings only).
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
		if err := showGPUStatus(); err != nil {
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
		return runGPUSigning(ctx, true)
	}

	// Apply the gpu settings category (batch when non-interactive), then run the
	// driver-signing setup.
	if isBatchMode() {
		if err := runCategoryBatch(ctx, "gpu"); err != nil {
			return err
		}
	} else if err := runCategoryWizard(ctx, "gpu", false); err != nil {
		return err
	}
	return runGPUSigning(ctx, false)
}
