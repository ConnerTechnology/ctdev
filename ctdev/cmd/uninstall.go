package cmd

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	comp "github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/picker"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/progress"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:               "uninstall [component...]",
	Short:             "Uninstall components",
	Long:              "Uninstall one or more components. Run without arguments for interactive picker.",
	RunE:              runUninstall,
	ValidArgsFunction: completeComponentNames,
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}

func runUninstall(cmd *cobra.Command, args []string) error {
	var selected []string

	if len(args) > 0 {
		for _, name := range args {
			if comp.FindByName(name) == nil {
				return fmt.Errorf("unknown component: %s", name)
			}
		}
		selected = args
	} else if isBatchMode() {
		return fmt.Errorf("no components specified (batch mode requires arguments)")
	} else {
		installed := comp.InstalledSet()
		var installedComps []comp.Component
		for i := range comp.Registry {
			if installed[comp.Registry[i].Name] {
				installedComps = append(installedComps, comp.Registry[i])
			}
		}
		if len(installedComps) == 0 {
			fmt.Println(styles.Dimmed.Render("No components installed."))
			return nil
		}
		osType := comp.OS(platform.Detect().OS)
		m := picker.New(installedComps, installed, osType, picker.ModeUninstall, flagDryRun)
		p := tea.NewProgram(&m)
		result, err := p.Run()
		resetTerminal()
		if err != nil {
			return err
		}
		pickerResult := result.(*picker.Model).GetResult()
		if pickerResult.Quit {
			return nil
		}
		if len(pickerResult.Selected) == 0 {
			fmt.Println("No components selected.")
			return nil
		}
		selected = pickerResult.Selected
	}

	if !flagDryRun && comp.UninstallNeedsRoot(selected) {
		if err := ensureSudo(); err != nil {
			return fmt.Errorf("sudo required for uninstall: %w", err)
		}
	}
	return runWithProgress(cmd.Context(), progressOperation{
		mode:  progress.ModeUninstall,
		names: selected,
	})
}
