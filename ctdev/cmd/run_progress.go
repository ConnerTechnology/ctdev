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
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
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

	progressModel := progress.New(op.names, op.mode, flagDryRun)
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
	if err != nil {
		return err
	}

	printSummaryReport(&progressModel)

	// The summary screen already showed the tally; make the exit code match.
	_, failed, _, notRun := progressModel.Counts()
	if notRun > 0 {
		fmt.Printf("Cancelled — %d of %d not run.\n", notRun, len(op.names))
	}
	if failed > 0 {
		return fmt.Errorf("%d component(s) failed", failed)
	}
	return nil
}

// printSummaryReport writes the per-item results to stdout after the TUI has
// exited. It deliberately does not go through Bubble Tea: the inline renderer
// truncates any frame taller than the terminal (dropping lines from the top), so
// a run with a few dozen steps would lose most of its results. Printed as
// ordinary text it scrolls and lands in scrollback intact.
func printSummaryReport(m *progress.Model) {
	report := m.SummaryReport()
	if report == "" {
		return
	}
	fmt.Print(report)
}

// streamThrough runs fn with a pipe writer whose lines are forwarded to send as
// InstallOutputMsg for name, waiting for the scanner to drain before returning.
// Pipe fds are cleaned up via defer so a panic in fn can't leak file
// descriptors.
func streamThrough(send msgSender, name string, fn func(pw *os.File)) error {
	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}
	// pr is closed after the scanner goroutine returns; pw is closed once fn
	// has finished so the scanner can drain and exit.
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

	fn(pw)
	pw.Close()
	wg.Wait()
	return nil
}

// runOneComponent runs install/uninstall for a single named component and
// streams its output through `send`.
func runOneComponent(ctx context.Context, send msgSender, op progressOperation, name string) {
	c := comp.FindByName(name)
	if c == nil {
		return
	}

	send.Send(progress.InstallStartMsg{Name: name})
	start := time.Now()

	var result comp.ExecResult
	if err := streamThrough(send, name, func(pw *os.File) {
		opts := comp.ExecOpts{
			Force:   flagForce,
			DryRun:  flagDryRun,
			Verbose: flagVerbose,
			Stdout:  pw,
			Stderr:  pw,
			// The TUI owns the terminal for the duration of this call, so a sudo
			// prompt here would be invisible and unanswerable. ensureSudo has
			// already cached the credential; if it somehow didn't, fail loudly.
			NoSudoPrompt: true,
		}
		if op.mode == progress.ModeUninstall {
			result = comp.Uninstall(ctx, c, opts)
		} else {
			result = comp.Install(ctx, c, opts)
		}
	}); err != nil {
		send.Send(progress.InstallFailMsg{Name: name, Error: err.Error(), Duration: time.Since(start)})
		return
	}

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

// runOneStep runs a single update step, streaming its output through `send`.
func runOneStep(ctx context.Context, send msgSender, step updateStep) {
	send.Send(progress.InstallStartMsg{Name: step.name})
	start := time.Now()

	var runErr error
	err := streamThrough(send, step.name, func(pw *os.File) {
		// NoSudoPrompt: the TUI holds the terminal — see runOneComponent.
		runErr = step.run(ctx, sysutil.Opts{Stdout: pw, DryRun: flagDryRun, NoSudoPrompt: true})
	})
	if err == nil {
		err = runErr
	}
	duration := time.Since(start)
	if err != nil {
		send.Send(progress.InstallFailMsg{Name: step.name, Error: err.Error(), Duration: duration})
		return
	}
	send.Send(progress.InstallDoneMsg{Name: step.name, Duration: duration})
}

// runUpdateSteps drives the update apply phase through the same progress TUI
// (and batch fallback) that install/uninstall use, so failures are visible and
// the exit code reflects them.
func runUpdateSteps(parent context.Context, steps []updateStep) error {
	if isBatchMode() {
		return runUpdateStepsBatch(parent, steps)
	}

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	names := make([]string, len(steps))
	for i, s := range steps {
		names[i] = s.name
	}
	progressModel := progress.New(names, progress.ModeUpdate, flagDryRun)
	p := tea.NewProgram(&progressModel)

	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		defer p.Send(progress.AllDoneMsg{})
		for _, s := range steps {
			if ctx.Err() != nil {
				return
			}
			runOneStep(ctx, p, s)
		}
	}()

	_, err := p.Run()
	cancel()
	<-workerDone
	resetTerminal()
	if err != nil {
		return err
	}

	printSummaryReport(&progressModel)

	_, failed, _, notRun := progressModel.Counts()
	if notRun > 0 {
		fmt.Printf("Cancelled — %d of %d not run.\n", notRun, len(steps))
	}
	if failed > 0 {
		return fmt.Errorf("%d update step(s) failed", failed)
	}
	return nil
}

// runUpdateStepsBatch applies update steps without a TUI (for CI/pipes),
// continuing past failures but reporting them in the exit code.
func runUpdateStepsBatch(ctx context.Context, steps []updateStep) error {
	var failed int
	for _, s := range steps {
		// Ctrl-C should stop cleanly, not cascade a failure per remaining step.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		fmt.Fprintf(os.Stdout, "Updating %s...\n", s.name)
		o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}
		if err := s.run(ctx, o); err != nil {
			fmt.Fprintf(os.Stderr, "  %s failed: %v\n", s.name, err)
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d update step(s) failed", failed)
	}
	return nil
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
