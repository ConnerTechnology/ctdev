package cmd

import (
	"context"
	"fmt"

	comp "github.com/ConnerTechnology/ctdev/ctdev/component"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/progress"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// toolInstalled and installTool are vars so tests can fake the tool a
// configure category needs, instead of installing it on the test machine.
var (
	toolInstalled = func(name string) bool {
		c := comp.FindByName(name)
		return c != nil && c.IsInstalled()
	}
	installTool = func(ctx context.Context, name string) error {
		resolved := comp.ResolveDependencies(comp.Registry, []string{name})
		if err := ensureSudoForInstall(ctx, resolved, false); err != nil {
			return err
		}
		return runWithProgress(ctx, progressOperation{mode: progress.ModeInstall, names: resolved})
	}
)

// ensureTool installs the component a configure category drives (the AWS CLI
// for `configure aws`, git for `configure git`) when it's missing: in batch
// mode without asking, otherwise after a [Y/n]. ok is false when the user said
// no, and the caller stops there.
func ensureTool(ctx context.Context, name, label string, batch bool) (ok bool, err error) {
	if toolInstalled(name) {
		return true, nil
	}
	if !batch {
		yes, err := promptYesNoCtx(ctx, fmt.Sprintf("%s isn't installed. Install it now?", label), true)
		if err != nil {
			return false, err
		}
		if !yes {
			fmt.Println(styles.Dimmed.Render(fmt.Sprintf("  Skipped. Install it later with: ctdev install %s", name)))
			return false, nil
		}
	} else {
		fmt.Printf("%s isn't installed. Installing it first.\n", label)
	}
	if err := installTool(ctx, name); err != nil {
		return false, fmt.Errorf("install %s: %w", name, err)
	}
	if !flagDryRun && !toolInstalled(name) {
		return false, fmt.Errorf("installed %s, but it isn't on PATH yet; open a new shell and run this again", name)
	}
	return true, nil
}
