package component

import (
	"context"
	"errors"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

type Executor struct {
	Platform platform.Info
}

func NewExecutor() *Executor {
	return &Executor{
		Platform: platform.Detect(),
	}
}

type ExecResult struct {
	Component string
	Err       error
	Skipped   bool
}

func (inst *Executor) Install(ctx context.Context, c *Component, opts ExecOpts) ExecResult {
	result := ExecResult{Component: c.Name}
	result.Err = c.GoInstall(ctx, opts)
	if errors.Is(result.Err, ErrUnsupportedOS) {
		result.Skipped = true
		result.Err = nil
	}
	return result
}

func (inst *Executor) Uninstall(ctx context.Context, c *Component, opts ExecOpts) ExecResult {
	result := ExecResult{Component: c.Name}
	result.Err = c.GoUninstall(ctx, opts)
	if errors.Is(result.Err, ErrUnsupportedOS) {
		result.Skipped = true
		result.Err = nil
	}
	return result
}
