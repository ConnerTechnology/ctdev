package cmd

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	comp "github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/state"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/picker"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/progress"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install [component...]",
	Short: "Install components",
	Long:  "Install one or more components. Run without arguments for interactive picker.",
	RunE:  runInstall,
}

func init() {
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	markers := state.DefaultMarkerStore()
	executor := comp.NewExecutor(dotfilesRoot())

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
		osType := comp.OS(executor.Platform.OS)
		m := picker.New(comp.Registry, installed, osType, picker.ModeInstall)
		p := tea.NewProgram(&m)
		result, err := p.Run()
		if err != nil {
			return err
		}
		pickerResult := result.(*picker.Model).GetResult()
		if pickerResult.Quit || len(pickerResult.Selected) == 0 {
			return nil
		}
		selected = pickerResult.Selected
	}

	selected = comp.ResolveDependencies(comp.Registry, selected)
	ensureSudo()
	return runWithProgress(progressOperation{
		mode:     progress.ModeInstall,
		executor: executor,
		markers:  markers,
		names:    selected,
	})
}
