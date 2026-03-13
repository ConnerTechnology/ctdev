package component

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/internal/shell"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

type Executor struct {
	DotfilesRoot string
	Platform     platform.Info
}

func NewExecutor(dotfilesRoot string) *Executor {
	return &Executor{
		DotfilesRoot: dotfilesRoot,
		Platform:     platform.Detect(),
	}
}

type ExecResult struct {
	Component string
	Err       error
	ExitCode  int
	Skipped   bool // exit code 2 = unsupported OS
}

func (inst *Executor) Install(ctx context.Context, c *Component, opts ExecOpts) ExecResult {
	result := ExecResult{Component: c.Name}

	if c.GoInstall != nil {
		result.Err = c.GoInstall(ctx, opts)
		return result
	}

	err := inst.runBash(ctx, c.BashInstall, opts)
	result.ExitCode = shell.ExitCode(err)
	if result.ExitCode == 2 {
		result.Skipped = true
		return result
	}
	result.Err = err
	return result
}

func (inst *Executor) Uninstall(ctx context.Context, c *Component, opts ExecOpts) ExecResult {
	result := ExecResult{Component: c.Name}

	if c.GoUninstall != nil {
		result.Err = c.GoUninstall(ctx, opts)
		return result
	}

	err := inst.runBash(ctx, c.BashUninstall, opts)
	result.ExitCode = shell.ExitCode(err)
	if result.ExitCode == 2 {
		result.Skipped = true
		return result
	}
	result.Err = err
	return result
}

func (inst *Executor) runBash(ctx context.Context, script string, opts ExecOpts) error {
	scriptPath := filepath.Join(inst.DotfilesRoot, script)
	env := []string{
		shell.BoolEnv("FORCE", opts.Force),
		shell.BoolEnv("DRY_RUN", opts.DryRun),
		shell.BoolEnv("VERBOSE", opts.Verbose),
		fmt.Sprintf("DOTFILES_ROOT=%s", inst.DotfilesRoot),
	}
	return shell.Run(ctx, "bash", []string{scriptPath}, shell.RunOpts{
		Env:    env,
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
	})
}
