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
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/progress"
)

type progressOperation struct {
	mode     progress.Mode
	executor *comp.Executor
	markers  *state.MarkerStore
	names    []string
}

func runWithProgress(op progressOperation) error {
	if isBatchMode() {
		return runWithProgressBatch(op)
	}

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

// runWithProgressBatch runs install/uninstall without a TUI (for CI/pipes).
func runWithProgressBatch(op progressOperation) error {
	ctx := context.Background()
	var failed int

	for _, name := range op.names {
		c := comp.FindByName(name)
		if c == nil {
			continue
		}

		fmt.Fprintf(os.Stdout, "Processing %s...\n", name)
		start := time.Now()

		opts := comp.ExecOpts{
			Force:   flagForce,
			DryRun:  flagDryRun,
			Verbose: flagVerbose,
			Stdout:  os.Stdout,
			Stderr:  os.Stderr,
		}

		var result comp.ExecResult
		if op.mode == progress.ModeUninstall {
			result = op.executor.Uninstall(ctx, c, opts)
		} else {
			result = op.executor.Install(ctx, c, opts)
		}
		duration := time.Since(start)

		if result.Skipped {
			fmt.Fprintf(os.Stdout, "  %s skipped\n", name)
		} else if result.Err != nil {
			fmt.Fprintf(os.Stderr, "  %s failed (%s): %v\n", name, duration, result.Err)
			failed++
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
			fmt.Fprintf(os.Stdout, "  %s done (%s)\n", name, duration)
		}
	}

	if failed > 0 {
		return fmt.Errorf("%d component(s) failed", failed)
	}
	return nil
}
