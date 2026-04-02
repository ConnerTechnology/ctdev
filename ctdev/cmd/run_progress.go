package cmd

import (
	"bufio"
	"context"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	comp "github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/state"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/progress"
)

type progressOperation struct {
	mode     progress.Mode
	executor *comp.Executor
	markers  *state.MarkerStore
	names    []string
}

func runWithProgress(op progressOperation) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	progressModel := progress.New(op.names, op.mode)
	p := tea.NewProgram(&progressModel)

	go func() {
		for _, name := range op.names {
			if ctx.Err() != nil {
				break
			}
			c := comp.FindByName(name)
			if c == nil {
				continue
			}

			p.Send(progress.InstallStartMsg{Name: name})
			start := time.Now()

			pr, pw, err := os.Pipe()
			if err != nil {
				p.Send(progress.InstallFailMsg{Name: name, Error: err.Error(), Duration: time.Since(start)})
				continue
			}
			go func(name string) {
				scanner := bufio.NewScanner(pr)
				for scanner.Scan() {
					p.Send(progress.InstallOutputMsg{Name: name, Line: scanner.Text()})
				}
			}(name)

			opts := comp.ExecOpts{
				Force:   flagForce,
				DryRun:  flagDryRun,
				Verbose: flagVerbose,
				Stdout:  pw,
				Stderr:  pw,
			}

			var result comp.ExecResult
			if op.mode == progress.ModeUninstall {
				result = op.executor.Uninstall(ctx, c, opts)
			} else {
				result = op.executor.Install(ctx, c, opts)
			}
			pw.Close()
			pr.Close()

			duration := time.Since(start)

			if result.Skipped {
				p.Send(progress.InstallSkipMsg{Name: name})
			} else if result.Err != nil {
				p.Send(progress.InstallFailMsg{Name: name, Error: result.Err.Error(), Duration: duration})
			} else {
				if op.mode == progress.ModeUninstall {
					op.markers.Remove(name)
				} else {
					op.markers.Save(name, state.InstallMarker{
						InstalledAt: time.Now(),
						Version:     "unknown",
						UpdatedAt:   time.Now(),
					})
				}
				p.Send(progress.InstallDoneMsg{Name: name, Duration: duration})
			}
		}
		p.Send(progress.AllDoneMsg{})
	}()

	_, err := p.Run()
	cancel()
	resetTerminal()
	return err
}
