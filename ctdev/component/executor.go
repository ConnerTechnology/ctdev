package component

import (
	"context"
	"errors"
)

type ExecResult struct {
	Component string
	Err       error
	Skipped   bool
}

// Install runs a component's installer, mapping ErrUnsupportedOS to a Skipped
// result so unsupported platforms report cleanly instead of as failures.
func Install(ctx context.Context, c *Component, opts ExecOpts) ExecResult {
	return runComponent(c, c.GoInstall, ctx, opts)
}

// Uninstall runs a component's uninstaller with the same Skipped mapping.
func Uninstall(ctx context.Context, c *Component, opts ExecOpts) ExecResult {
	return runComponent(c, c.GoUninstall, ctx, opts)
}

func runComponent(c *Component, fn func(context.Context, ExecOpts) error, ctx context.Context, opts ExecOpts) ExecResult {
	result := ExecResult{Component: c.Name}
	result.Err = fn(ctx, opts)
	if errors.Is(result.Err, ErrUnsupportedOS) {
		result.Skipped = true
		result.Err = nil
	}
	return result
}
