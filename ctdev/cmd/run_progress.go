package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	comp "github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/progress"
)

type progressOperation struct {
	mode  progress.Mode
	names []string
}

// msgSender is the subset of *tea.Program that we depend on, letting tests
// swap in a fake for runOneComponent without spinning up a real TUI.
type msgSender interface {
	Send(tea.Msg)
}

func runWithProgress(parent context.Context, op progressOperation) error {
	if isBatchMode() {
		return runWithProgressBatch(parent, op)
	}

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	progressModel := progress.New(op.names, op.mode)
	p := tea.NewProgram(&progressModel)

	// workerDone lets us block on the install loop after the TUI exits so we
	// never return (and let the parent process continue) while install
	// goroutines are still running.
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		defer p.Send(progress.AllDoneMsg{})
		for _, name := range op.names {
			if ctx.Err() != nil {
				return
			}
			runOneComponent(ctx, p, op, name)
		}
	}()

	_, err := p.Run()
	cancel()
	<-workerDone
	resetTerminal()
	return err
}

// runOneComponent runs install/uninstall for a single named component and
// streams its output through `send`. Pipe fds are cleaned up via defer so a
// panic in the install func can't leak file descriptors.
func runOneComponent(ctx context.Context, send msgSender, op progressOperation, name string) {
	c := comp.FindByName(name)
	if c == nil {
		return
	}

	send.Send(progress.InstallStartMsg{Name: name})
	start := time.Now()

	pr, pw, err := os.Pipe()
	if err != nil {
		send.Send(progress.InstallFailMsg{Name: name, Error: err.Error(), Duration: time.Since(start)})
		return
	}
	// pr is closed after the scanner goroutine returns; pw is closed once the
	// install function has finished so the scanner can drain and exit.
	defer pr.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			send.Send(progress.InstallOutputMsg{Name: name, Line: scanner.Text()})
		}
	}()

	opts := comp.ExecOpts{
		Force:   flagForce,
		DryRun:  flagDryRun,
		Verbose: flagVerbose,
		Stdout:  pw,
		Stderr:  pw,
	}

	var result comp.ExecResult
	if op.mode == progress.ModeUninstall {
		result = comp.Uninstall(ctx, c, opts)
	} else {
		result = comp.Install(ctx, c, opts)
	}
	pw.Close()
	wg.Wait()

	duration := time.Since(start)

	switch {
	case result.Skipped:
		send.Send(progress.InstallSkipMsg{Name: name})
	case result.Err != nil:
		send.Send(progress.InstallFailMsg{Name: name, Error: result.Err.Error(), Duration: duration})
	default:
		send.Send(progress.InstallDoneMsg{Name: name, Duration: duration})
	}
}

// runWithProgressBatch runs install/uninstall without a TUI (for CI/pipes).
func runWithProgressBatch(ctx context.Context, op progressOperation) error {
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
			result = comp.Uninstall(ctx, c, opts)
		} else {
			result = comp.Install(ctx, c, opts)
		}
		duration := time.Since(start)

		if result.Skipped {
			fmt.Fprintf(os.Stdout, "  %s skipped\n", name)
		} else if result.Err != nil {
			fmt.Fprintf(os.Stderr, "  %s failed (%s): %v\n", name, duration, result.Err)
			failed++
		} else {
			fmt.Fprintf(os.Stdout, "  %s done (%s)\n", name, duration)
		}
	}

	if failed > 0 {
		return fmt.Errorf("%d component(s) failed", failed)
	}
	return nil
}
