package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"time"

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
		installed := make(map[string]bool)
		list, _ := markers.List()
		for _, name := range list {
			installed[name] = true
		}

		osType := comp.OS(executor.Platform.OS)
		p := tea.NewProgram(picker.New(comp.Registry, installed, osType))
		result, err := p.Run()
		if err != nil {
			return err
		}
		pickerResult := result.(picker.Model).GetResult()
		if pickerResult.Quit || len(pickerResult.Selected) == 0 {
			return nil
		}
		selected = pickerResult.Selected
	}

	selected = comp.ResolveDependencies(comp.Registry, selected)
	return runInstallWithProgress(executor, markers, selected)
}

func runInstallWithProgress(executor *comp.Executor, markers *state.MarkerStore, names []string) error {
	progressModel := progress.New(names)

	p := tea.NewProgram(progressModel)

	go func() {
		for _, name := range names {
			c := comp.FindByName(name)
			if c == nil {
				continue
			}

			p.Send(progress.InstallStartMsg{Name: name})
			start := time.Now()

			pr, pw, _ := os.Pipe()
			go func(name string) {
				scanner := bufio.NewScanner(pr)
				for scanner.Scan() {
					p.Send(progress.InstallOutputMsg{Name: name, Line: scanner.Text()})
				}
			}(name)

			result := executor.Install(context.Background(), c, comp.ExecOpts{
				Force:   flagForce,
				DryRun:  flagDryRun,
				Verbose: flagVerbose,
				Stdout:  pw,
				Stderr:  pw,
			})
			pw.Close()

			duration := time.Since(start)

			if result.Skipped {
				p.Send(progress.InstallSkipMsg{Name: name})
			} else if result.Err != nil {
				p.Send(progress.InstallFailMsg{Name: name, Error: result.Err.Error(), Duration: duration})
			} else {
				markers.Save(name, state.InstallMarker{
					InstalledAt: time.Now(),
					Version:     "unknown",
					UpdatedAt:   time.Now(),
				})
				p.Send(progress.InstallDoneMsg{Name: name, Duration: duration})
			}
		}
		p.Send(progress.AllDoneMsg{})
	}()

	_, err := p.Run()
	return err
}
