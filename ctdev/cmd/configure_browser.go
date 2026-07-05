package cmd

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/setup"
	tuisettings "github.com/ConnerTechnology/dotfiles/ctdev/tui/settings"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
)

// runSettingsBrowser is the interactive path behind a bare `ctdev configure`:
// every applicable setting in one full-screen browser instead of a sequential
// category walk. Queued changes come back here for the standard summary +
// confirm + apply. The gpu category stays out — its driver-signing apply runs
// the interactive Secure Boot/MOK flow (`ctdev configure gpu`).
func runSettingsBrowser(ctx context.Context) error {
	var settings []setup.Setting
	for _, slug := range slugOrder {
		if slug == "gpu" {
			continue
		}
		settings = append(settings, setup.FilterByHardware(setup.FilterBySlug(setup.Registry, slug))...)
	}
	if len(settings) == 0 {
		fmt.Println(styles.Dimmed.Render("No applicable settings on this machine."))
		return nil
	}

	fmt.Println(styles.Dimmed.Render("Detecting current settings…"))
	states := setup.InitStates(ctx, settings)
	// The browser starts with nothing queued: InitStates seeds DesiredValue
	// with the recommended default (the --batch contract), but interactively a
	// change only exists once the user makes one.
	for i := range states {
		states[i].DesiredValue = states[i].CurrentValue
		states[i].Enabled = false
	}

	m := tuisettings.New(states)
	if _, err := tea.NewProgram(m).Run(); err != nil {
		resetTerminal()
		return err
	}
	resetTerminal()

	if !m.Applied() {
		fmt.Println(styles.Dimmed.Render("No changes applied."))
		return nil
	}

	var changed []int
	for i := range states {
		if states[i].Enabled && states[i].DesiredValue != states[i].CurrentValue {
			changed = append(changed, i)
		}
	}
	if len(changed) == 0 {
		fmt.Println(styles.Dimmed.Render("No changes queued."))
		return nil
	}

	fmt.Println()
	fmt.Println(styles.Dimmed.Render("  Changes:"))
	for _, idx := range changed {
		s := &states[idx]
		fmt.Printf("    %s: %s → %s\n", s.Setting.Name,
			styles.Dimmed.Render(s.CurrentValue),
			styles.Value.Render(s.DesiredValue))
	}
	fmt.Println()

	if !flagDryRun {
		yes, err := promptYesNoCtx(ctx, "Apply?", true)
		if err != nil {
			return err
		}
		if !yes {
			fmt.Println(styles.Dimmed.Render("  Skipped."))
			return nil
		}
		if err := ensureSudo(); err != nil {
			return fmt.Errorf("sudo required: %w", err)
		}
	}

	return applySettings(ctx, states, flagForce, flagDryRun, flagVerbose)
}
