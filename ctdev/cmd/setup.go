package cmd

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/setup"
	tuisetup "github.com/ConnerTechnology/dotfiles/ctdev/tui/setup"
	"github.com/spf13/cobra"
)

var (
	flagSetupShow  bool
	flagSetupReset bool
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure system settings",
	Long:  "Interactive system configuration. Linux Mint and macOS supported.",
	RunE:  runSetup,
}

func init() {
	setupCmd.Flags().BoolVar(&flagSetupShow, "show", false, "show current system configuration (read-only)")
	setupCmd.Flags().BoolVar(&flagSetupReset, "reset", false, "reset configuration to system defaults")
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	info := platform.Detect()

	// macOS setup
	if info.OS == platform.MacOS {
		if flagSetupShow {
			fmt.Println("macOS setup --show is not yet supported. Run 'ctdev setup' to apply defaults.")
			return nil
		}
		if flagSetupReset {
			fmt.Println("macOS setup --reset is not yet supported.")
			return nil
		}
		return runMacOSSetup()
	}

	// Filter settings by detected hardware
	settings := setup.FilterByHardware(setup.Registry)
	states := setup.InitStates(settings)

	if flagSetupReset {
		return runSetupReset()
	}

	mode := tuisetup.ModeInteractive
	if flagSetupShow {
		mode = tuisetup.ModeReadonly
	}

	if isBatchMode() && mode == tuisetup.ModeInteractive {
		return runBatchSetup(states)
	}

	m := tuisetup.New(states, mode)
	p := tea.NewProgram(&m)
	result, err := p.Run()
	resetTerminal()
	if err != nil {
		return err
	}

	model := result.(*tuisetup.Model)
	if !model.Applied() {
		return nil
	}

	return applySettings(model.States(), flagForce, flagDryRun, flagVerbose)
}

func applySettings(states []setup.SettingState, force, dryRun, verbose bool) error {
	if !dryRun {
		if err := ensureSudo(); err != nil {
			return fmt.Errorf("sudo required for setup: %w", err)
		}
	}

	appliedGroups := make(map[string]bool)
	var applied, failed int

	for i := range states {
		if !states[i].NeedsApply(force) {
			continue
		}
		s := states[i].Setting

		if dryRun {
			fmt.Printf("  [dry-run] %s: %s → %s\n", s.Name, states[i].CurrentValue, states[i].DesiredValue)
			applied++
			continue
		}

		if verbose {
			fmt.Printf("  Applying: %s (%s → %s)\n", s.Name, states[i].CurrentValue, states[i].DesiredValue)
		} else {
			fmt.Printf("  Applying: %s\n", s.Name)
		}

		if s.ApplyFunc != nil {
			if err := s.ApplyFunc(states[i].DesiredValue); err != nil {
				fmt.Printf("  ✗ %s: %v\n", s.Name, err)
				failed++
				continue
			}
		}
		applied++

		if s.ApplyGroup != "" {
			appliedGroups[s.ApplyGroup] = true
		}
	}

	// Run post-apply hooks
	if !dryRun {
		for group := range appliedGroups {
			if hook, ok := setup.PostApplyHooks[group]; ok {
				if err := hook(); err != nil {
					fmt.Printf("  ✗ post-apply hook %s: %v\n", group, err)
				}
			}
		}
	}

	fmt.Printf("\n  %d applied", applied)
	if failed > 0 {
		fmt.Printf(" · %d failed", failed)
	}
	fmt.Println()
	return nil
}

func runBatchSetup(states []setup.SettingState) error {
	if flagDryRun {
		fmt.Println("[dry-run] Would apply default settings:")
	} else {
		fmt.Println("Applying default settings...")
	}
	return applySettings(states, flagForce, flagDryRun, flagVerbose)
}

func runSetupReset() error {
	return setup.ResetLinuxDefaults(os.Stdout, flagDryRun)
}

func runMacOSSetup() error {
	return setup.ApplyMacOSDefaults(os.Stdout, flagDryRun)
}
