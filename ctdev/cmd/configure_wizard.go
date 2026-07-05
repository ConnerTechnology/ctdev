package cmd

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"

	"github.com/ConnerTechnology/dotfiles/ctdev/setup"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
)

// slugDescriptions maps slugs to human-readable descriptions for the wizard header.
// New slugs don't need an entry — the raw slug is used as a fallback.
var slugDescriptions = map[string]string{
	"gpu":        "GPU & NVIDIA",
	"boot":       "Boot (GRUB)",
	"power":      "Power & Sleep",
	"keyboard":   "Keyboard",
	"mouse":      "Mouse & Pointer",
	"audio":      "Audio",
	"bluetooth":  "Bluetooth",
	"desktop":    "Desktop",
	"network":    "Network & WiFi",
	"system":     "System",
	"ssh":        "SSH",
	"ufw":        "Firewall (UFW)",
	"locale":     "Locale",
	"sleep":      "Sleep & Suspend",
	"linger":     "Service Lingering",
	"tunnel":     "VS Code Tunnel",
	"autoupdate": "Automatic Updates",
	"pihole":     "Pi-hole DNS",
	"caddy":      "Caddy reverse proxy",
}

// slugOrder is derived from setup.Registry so adding a setting with a brand
// new slug automatically yields a matching `ctdev configure <slug>` subcommand
// and wizard step — no hand-kept list to drift from.
var slugOrder = setup.Slugs(setup.Registry)

// slugDescription returns a human-readable label for a slug, falling back to
// the slug itself when no mapping exists.
func slugDescription(slug string) string {
	if d := slugDescriptions[slug]; d != "" {
		return d
	}
	return slug
}

var wizardLabelStyle = styles.Label(26)
var wizardValueStyle = styles.Value
var wizardHeaderStyle = styles.Header

// runCategoryWizard runs the interactive wizard for a single category slug.
func runCategoryWizard(ctx context.Context, slug string, showOnly bool) error {
	return runCategoryWizardOn(ctx, setup.Registry, slug, showOnly)
}

// runCategoryBatch applies a category's settings non-interactively at their
// recommended defaults. Used by bootstrap (`ctdev configure <slug> --batch`).
// Idempotent: applySettings skips settings already at their desired value.
func runCategoryBatch(ctx context.Context, slug string) error {
	settings := setup.FilterBySlug(setup.Registry, slug)
	settings = setup.FilterByHardware(settings)
	if len(settings) == 0 {
		fmt.Printf("  %s\n", styles.Dimmed.Render(fmt.Sprintf("No applicable %s settings on this hardware.", slugDescription(slug))))
		return nil
	}
	fmt.Println(wizardHeaderStyle.Render(slugDescription(slug) + " (batch — applying recommended values)"))
	states := setup.InitStates(ctx, settings)
	return applySettings(ctx, states, flagForce, flagDryRun, flagVerbose)
}

// runCategoryWizardOn is the testable core of runCategoryWizard — it takes an
// explicit registry so tests can supply a fixture without mutating global
// state.
func runCategoryWizardOn(ctx context.Context, registry []setup.Setting, slug string, showOnly bool) error {
	settings := setup.FilterBySlug(registry, slug)
	settings = setup.FilterByHardware(settings)
	if len(settings) == 0 {
		fmt.Printf("  %s\n\n",
			styles.Dimmed.Render(fmt.Sprintf("No applicable %s settings on this hardware.", slugDescription(slug))))
		return nil
	}

	states := setup.InitStates(ctx, settings)

	fmt.Println(wizardHeaderStyle.Render(slugDescription(slug)))
	fmt.Println()

	if showOnly {
		for i := range states {
			showSetting(&states[i])
		}
		fmt.Println()
		return nil
	}

	var changed []int
	for i := range states {
		wasChanged, err := promptSetting(ctx, &states[i])
		if err != nil {
			return err
		}
		if wasChanged {
			changed = append(changed, i)
		}
	}

	if len(changed) == 0 {
		fmt.Println(styles.Dimmed.Render("  No changes."))
		fmt.Println()
		return nil
	}

	// Show summary and confirm
	fmt.Println()
	fmt.Println(styles.Dimmed.Render("  Changes:"))
	for _, idx := range changed {
		s := &states[idx]
		fmt.Printf("    %s: %s → %s\n", s.Setting.Name,
			styles.Dimmed.Render(s.CurrentValue),
			wizardValueStyle.Render(s.DesiredValue))
	}
	fmt.Println()

	if !flagDryRun {
		yes, err := promptYesNoCtx(ctx, "Apply?", true)
		if err != nil {
			return err
		}
		if !yes {
			fmt.Println(styles.Dimmed.Render("  Skipped."))
			fmt.Println()
			return nil
		}
	}

	return applySettings(ctx, states, flagForce, flagDryRun, flagVerbose)
}

// showSetting displays the current value of a setting (read-only).
func showSetting(state *setup.SettingState) {
	s := state.Setting
	label := wizardLabelStyle.Render(s.Name)
	value := wizardValueStyle.Render(state.CurrentValue)

	marker := ""
	if state.CurrentValue != s.Default {
		marker = styles.Dimmed.Render(fmt.Sprintf(" (recommended: %s)", s.Default))
	}
	fmt.Printf("  %s %s%s\n", label, value, marker)
}

// promptSetting interactively prompts the user for a setting value.
// Returns true if the value was changed from the current value.
func promptSetting(ctx context.Context, state *setup.SettingState) (bool, error) {
	s := state.Setting

	fmt.Printf("  %s\n", wizardValueStyle.Render(s.Name))
	fmt.Printf("  %s\n", styles.Dimmed.Render(s.Description))
	current := state.CurrentValue
	if s.Default != "" && s.Default != current {
		// Default is documented as "our recommended value" — say so; --batch
		// applies exactly these.
		fmt.Printf("  %s %s %s\n", styles.Dimmed.Render("Current:"), current,
			styles.Dimmed.Render(fmt.Sprintf("(recommended: %s)", s.Default)))
	} else {
		fmt.Printf("  %s %s\n", styles.Dimmed.Render("Current:"), current)
	}

	// One-way settings (install/enable actions with no off path) get a single
	// honest question instead of an enable/disable choice the apply layer
	// can't deliver on.
	if s.OneWay {
		if current == s.Default {
			fmt.Printf("  %s\n\n", styles.Dimmed.Render("Already "+current+" — nothing to do."))
			state.Enabled = false
			return false, nil
		}
		yes, err := promptYesNoCtx(ctx, fmt.Sprintf("Apply now (→ %s)?", s.Default), false)
		if err != nil {
			return false, err
		}
		fmt.Println()
		if !yes {
			state.Enabled = false
			return false, nil
		}
		state.DesiredValue = s.Default
		state.Enabled = true
		return true, nil
	}

	var newValue string
	var err error

	switch s.Control {
	case setup.ControlToggle:
		newValue, err = promptToggle(ctx, state)
	case setup.ControlPicker:
		newValue, err = promptPicker(ctx, state)
	case setup.ControlSlider:
		newValue, err = promptSlider(ctx, state)
	}
	if err != nil {
		return false, err
	}

	fmt.Println()

	if newValue == "" || newValue == state.CurrentValue {
		state.Enabled = false
		return false, nil
	}

	state.DesiredValue = newValue
	state.Enabled = true
	return true, nil
}

// promptToggle prompts for a toggle setting (true/false, enabled/disabled, etc).
func promptToggle(ctx context.Context, state *setup.SettingState) (string, error) {
	s := state.Setting

	// Determine the toggle pair from the default value.
	pairs := [][]string{
		{"true", "false"},
		{"false", "true"},
		{"enabled", "disabled"},
		{"disabled", "enabled"},
		{"installed", "not installed"},
		{"not installed", "installed"},
		{"signed", "unsigned"},
		{"unsigned", "signed"},
		{"active", "inactive"},
		{"inactive", "active"},
	}

	var onVal, offVal string
	for _, pair := range pairs {
		if s.Default == pair[0] || state.CurrentValue == pair[0] {
			onVal, offVal = pair[0], pair[1]
			break
		}
	}
	if onVal == "" {
		onVal, offVal = s.Default, "other"
	}

	currentIsOn := state.CurrentValue == onVal
	defaultChoice := "1"
	if !currentIsOn {
		defaultChoice = "2"
	}

	fmt.Printf("    1) %s\n", onVal)
	fmt.Printf("    2) %s\n", offVal)
	fmt.Printf("  Select [%s]: ", defaultChoice)

	input, ok := readLineCtx(ctx)
	if !ok {
		return "", errPromptCancelled
	}
	if input == "" {
		return state.CurrentValue, nil // keep current
	}

	switch input {
	case "1":
		return onVal, nil
	case "2":
		return offVal, nil
	default:
		fmt.Println(styles.Dimmed.Render("  Invalid choice, keeping current value."))
		return state.CurrentValue, nil
	}
}

// promptPicker prompts for a picker setting with multiple choices.
func promptPicker(ctx context.Context, state *setup.SettingState) (string, error) {
	s := state.Setting

	defaultIdx := 1
	for i, c := range s.Choices {
		marker := " "
		if c.Value == state.CurrentValue {
			marker = "*"
			defaultIdx = i + 1
		}
		fmt.Printf("   %s%d) %s — %s\n", marker, i+1, c.Value, styles.Dimmed.Render(c.Description))
	}
	fmt.Printf("  Select [%d]: ", defaultIdx)

	input, ok := readLineCtx(ctx)
	if !ok {
		return "", errPromptCancelled
	}
	if input == "" {
		return state.CurrentValue, nil
	}

	n, err := strconv.Atoi(input)
	if err != nil || n < 1 || n > len(s.Choices) {
		fmt.Println(styles.Dimmed.Render("  Invalid choice, keeping current value."))
		return state.CurrentValue, nil
	}
	return s.Choices[n-1].Value, nil
}

// promptSlider prompts for a numeric slider setting.
func promptSlider(ctx context.Context, state *setup.SettingState) (string, error) {
	s := state.Setting
	r := s.Slider
	if r == nil {
		return state.CurrentValue, nil
	}

	unit := ""
	if r.Unit != "" {
		unit = r.Unit
	}

	fmt.Printf("  %s range: %s–%s%s (step %s)\n",
		styles.Dimmed.Render("Valid"),
		formatSliderVal(r.Min, r.Step), formatSliderVal(r.Max, r.Step), unit,
		formatSliderVal(r.Step, r.Step))
	fmt.Printf("  %s [%s]: ", styles.Dimmed.Render("Value"), state.CurrentValue)

	input, ok := readLineCtx(ctx)
	if !ok {
		return "", errPromptCancelled
	}
	if input == "" {
		return state.CurrentValue, nil
	}

	val, err := strconv.ParseFloat(input, 64)
	if err != nil {
		fmt.Printf("  %s\n", styles.Dimmed.Render("Invalid number, keeping current value."))
		return state.CurrentValue, nil
	}

	// Clamp to range
	if val < r.Min {
		val = r.Min
	}
	if val > r.Max {
		val = r.Max
	}
	// Snap to nearest step
	val = math.Round(val/r.Step) * r.Step

	return formatSliderVal(val, r.Step), nil
}

// formatSliderVal formats a float value, using integer format when the step is >= 1.
func formatSliderVal(val, step float64) string {
	if step >= 1 {
		return strconv.Itoa(int(val))
	}
	return strconv.FormatFloat(val, 'f', -1, 64)
}

// applySettings applies changed settings, running post-apply hooks for groups.
func applySettings(ctx context.Context, states []setup.SettingState, force, dryRun, verbose bool) error {
	o := sysutil.Opts{Stdout: os.Stdout, DryRun: dryRun}
	appliedGroups := make(map[string]bool)
	var applied, failed int

	for i := range states {
		if !states[i].NeedsApply(force) {
			continue
		}
		s := states[i].Setting

		if dryRun {
			fmt.Printf("  [dry-run] %s: %s → %s\n", s.Name, states[i].CurrentValue, states[i].DesiredValue)
			applied++
			continue
		}

		if verbose {
			fmt.Printf("  Applying: %s (%s → %s)\n", s.Name, states[i].CurrentValue, states[i].DesiredValue)
		} else {
			fmt.Printf("  Applying: %s\n", s.Name)
		}

		if s.ApplyFunc != nil {
			if err := s.ApplyFunc(ctx, o, states[i].DesiredValue); err != nil {
				fmt.Printf("  ✗ %s: %v\n", s.Name, err)
				failed++
				continue
			}
		}
		applied++

		if s.ApplyGroup != "" {
			appliedGroups[s.ApplyGroup] = true
		}
	}

	// Run post-apply hooks
	if !dryRun {
		for group := range appliedGroups {
			if hook, ok := setup.PostApplyHooks[group]; ok {
				if err := hook(ctx, o); err != nil {
					fmt.Printf("  ✗ post-apply hook %s: %v\n", group, err)
				}
			}
		}
	}

	fmt.Printf("\n  %d applied", applied)
	if failed > 0 {
		fmt.Printf(" · %d failed", failed)
	}
	fmt.Println()
	return nil
}
