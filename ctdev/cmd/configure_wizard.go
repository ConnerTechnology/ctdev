package cmd

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/setup"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
)

// slugDescriptions maps slugs to human-readable descriptions for the wizard header.
// New slugs don't need an entry — the raw slug is used as a fallback.
var slugDescriptions = map[string]string{
	"gpu":       "GPU & NVIDIA",
	"boot":      "Boot (GRUB)",
	"power":     "Power & Sleep",
	"keyboard":  "Keyboard",
	"mouse":     "Mouse & Pointer",
	"audio":     "Audio",
	"bluetooth": "Bluetooth",
	"desktop":   "Desktop",
	"network":   "Network & WiFi",
	"system":    "System",
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

var wizardLabelStyle = lipgloss.NewStyle().Foreground(styles.Subtle).Width(26)
var wizardValueStyle = lipgloss.NewStyle().Foreground(styles.Bright)
var wizardHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(styles.Orange)

// runCategoryWizard runs the interactive wizard for a single category slug.
func runCategoryWizard(ctx context.Context, slug string, showOnly bool) error {
	return runCategoryWizardOn(ctx, setup.Registry, slug, showOnly)
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

	states := setup.InitStates(settings)

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
		wasChanged, err := promptSetting(&states[i])
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
		fmt.Printf("  Apply? [Y/n]: ")
		input := promptLine()
		if input != "" && strings.ToLower(input) != "y" && strings.ToLower(input) != "yes" {
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
		marker = styles.Dimmed.Render(fmt.Sprintf(" (default: %s)", s.Default))
	}
	fmt.Printf("  %s %s%s\n", label, value, marker)
}

// promptSetting interactively prompts the user for a setting value.
// Returns true if the value was changed from the current value.
func promptSetting(state *setup.SettingState) (bool, error) {
	s := state.Setting

	fmt.Printf("  %s\n", wizardValueStyle.Render(s.Name))
	fmt.Printf("  %s\n", styles.Dimmed.Render(s.Description))
	fmt.Printf("  %s %s\n", styles.Dimmed.Render("Current:"), state.CurrentValue)

	var newValue string

	switch s.Control {
	case setup.ControlToggle:
		newValue = promptToggle(state)
	case setup.ControlPicker:
		newValue = promptPicker(state)
	case setup.ControlSlider:
		newValue = promptSlider(state)
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
func promptToggle(state *setup.SettingState) string {
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

	input := promptLine()
	if input == "" {
		return state.CurrentValue // keep current
	}

	switch input {
	case "1":
		return onVal
	case "2":
		return offVal
	default:
		return state.CurrentValue
	}
}

// promptPicker prompts for a picker setting with multiple choices.
func promptPicker(state *setup.SettingState) string {
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

	input := promptLine()
	if input == "" {
		return state.CurrentValue
	}

	n, err := strconv.Atoi(input)
	if err != nil || n < 1 || n > len(s.Choices) {
		return state.CurrentValue
	}
	return s.Choices[n-1].Value
}

// promptSlider prompts for a numeric slider setting.
func promptSlider(state *setup.SettingState) string {
	s := state.Setting
	r := s.Slider
	if r == nil {
		return state.CurrentValue
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

	input := promptLine()
	if input == "" {
		return state.CurrentValue
	}

	val, err := strconv.ParseFloat(input, 64)
	if err != nil {
		fmt.Printf("  %s\n", styles.Dimmed.Render("Invalid number, keeping current value."))
		return state.CurrentValue
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

	return formatSliderVal(val, r.Step)
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

