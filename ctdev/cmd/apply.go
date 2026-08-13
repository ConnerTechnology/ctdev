package cmd

import (
	"context"
	"fmt"
	"strings"

	comp "github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/profile"
	"github.com/ConnerTechnology/dotfiles/ctdev/setup"
	"github.com/ConnerTechnology/dotfiles/ctdev/state"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/progress"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var flagApplyYes bool

var applyCmd = &cobra.Command{
	Use:   "apply [profile]",
	Short: "Apply a machine profile (install components + batch-configure)",
	Long: "Realize a named machine profile: install its components (dependencies resolve " +
		"automatically) and apply its configure categories at their recommended values. " +
		"Run without arguments to list available profiles.\n\n" +
		"Profiles are built into the binary; drop a <name>.toml in ~/.config/ctdev/profiles/ " +
		"to add your own or override a built-in. Interactive wizards (restic, caddy, pihole, " +
		"git) are never run by apply — each profile's closing notes list them.",
	Args:              cobra.MaximumNArgs(1),
	RunE:              runApply,
	ValidArgsFunction: completeProfileNames,
}

var diffCmd = &cobra.Command{
	Use:   "diff <profile>",
	Short: "Show how this machine drifts from a profile",
	Long: "Compare this machine against a profile: which components are missing, and which " +
		"settings in the profile's configure categories differ from their recommended values. " +
		"Exits non-zero when there is drift, so it works as a cron/CI check.",
	Args:              cobra.ExactArgs(1),
	RunE:              runDiff,
	ValidArgsFunction: completeProfileNames,
}

func init() {
	applyCmd.Flags().BoolVarP(&flagApplyYes, "yes", "y", false, "skip the confirmation prompt")
	rootCmd.AddCommand(applyCmd, diffCmd)
}

func completeProfileNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return profile.Names(), cobra.ShellCompDirectiveNoFileComp
}

// validateProfile rejects names that don't resolve, so a typo fails before
// anything installs rather than midway through.
func validateProfile(p *profile.Profile) error {
	var bad []string
	for _, name := range p.Components {
		if comp.FindByName(name) == nil {
			bad = append(bad, "component "+name)
		}
	}
	slugs := map[string]bool{}
	for _, s := range slugOrder {
		slugs[s] = true
	}
	for _, s := range p.Configure {
		if !slugs[s] {
			bad = append(bad, "configure category "+s)
			continue
		}
		// The gpu category runs the interactive Secure Boot/MOK signing flow —
		// it can't be batch-applied from a profile.
		if s == "gpu" {
			bad = append(bad, "configure category gpu (run 'ctdev configure gpu' interactively)")
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("profile %s references unknown or unusable entries:\n  %s", p.Name, strings.Join(bad, "\n  "))
	}
	return nil
}

func runApply(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return listProfiles()
	}
	return cancelToClean(applyProfile(cmdContext(cmd), args[0]))
}

func listProfiles() error {
	fmt.Println(styles.Title.Render("Machine profiles"))
	fmt.Println()
	for _, p := range profile.List() {
		src := ""
		if p.Source != "built-in" {
			src = styles.Warning.Render(" (local override)")
		}
		fmt.Printf("  %s%s\n", styles.Value.Render(p.Name), src)
		fmt.Printf("    %s\n", styles.Dimmed.Render(p.Description))
		fmt.Printf("    %s\n", styles.Dimmed.Render(fmt.Sprintf("%d components · configure: %s",
			len(p.Components), strings.Join(p.Configure, ", "))))
	}
	fmt.Println()
	fmt.Println(styles.Dimmed.Render("  Apply one with: ctdev apply <name>   ·   check drift with: ctdev diff <name>"))
	fmt.Println(styles.Dimmed.Render("  Add your own in ~/.config/ctdev/profiles/<name>.toml"))
	return nil
}

func applyProfile(ctx context.Context, name string) error {
	p, err := profile.Load(name)
	if err != nil {
		return err
	}
	if err := validateProfile(p); err != nil {
		return err
	}

	installed := comp.InstalledSet()
	resolved := comp.ResolveDependencies(comp.Registry, p.Components)

	// The plan, before anything happens.
	fmt.Println(styles.Title.Render("Apply profile: " + p.Name))
	fmt.Println(styles.Dimmed.Render("  " + p.Description))
	fmt.Println()
	toInstall := 0
	for _, c := range resolved {
		if installed[c] {
			fmt.Printf("  %s %s %s\n", styles.Success.Render("✓"), c, styles.Dimmed.Render("installed (configs re-synced)"))
		} else {
			toInstall++
			fmt.Printf("  %s %s\n", styles.Value.Render("→"), c)
		}
	}
	if len(p.Configure) > 0 {
		fmt.Printf("\n  Configure at recommended values: %s\n", styles.Value.Render(strings.Join(p.Configure, ", ")))
	}
	fmt.Println()

	if flagDryRun {
		fmt.Println(styles.Dimmed.Render("  [dry-run] stopping at the plan — components would install and categories would batch-apply"))
		return nil
	}
	if !flagApplyYes && !isBatchMode() {
		yes, err := promptYesNoCtx(ctx, fmt.Sprintf("Apply %s (%d to install)?", p.Name, toInstall), true)
		if err != nil {
			return err
		}
		if !yes {
			fmt.Println(styles.Dimmed.Render("  Cancelled — nothing was changed."))
			return nil
		}
	}

	// The configure categories all write system state, so any profile with them
	// needs root regardless of what its components need.
	if len(p.Configure) > 0 || comp.InstallNeedsRoot(platform.Detect().PackageManager, resolved, flagForce) {
		if err := ensureSudo(ctx); err != nil {
			return fmt.Errorf("sudo required: %w", err)
		}
	}
	if err := runWithProgress(ctx, progressOperation{mode: progress.ModeInstall, names: resolved}); err != nil {
		return err
	}

	// Batch-configure, continuing past a failing category so one bad apply
	// doesn't strand the rest; the exit code still reports it.
	var failed []string
	for _, slug := range p.Configure {
		fmt.Println()
		if err := runCategoryBatch(ctx, slug); err != nil {
			fmt.Printf("  %s\n", styles.Error.Render(fmt.Sprintf("✗ %s: %v", slug, err)))
			failed = append(failed, slug)
		}
	}

	if notes := strings.TrimSpace(p.Notes); notes != "" {
		fmt.Println()
		fmt.Println(styles.Header.Render("Profile notes"))
		for _, line := range strings.Split(notes, "\n") {
			fmt.Println("  " + line)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d configure categor(y/ies) failed: %s", len(failed), strings.Join(failed, ", "))
	}
	// Remember what this machine was built from so `ctdev info` can name it.
	// A failure here costs nothing but the label, so it never fails the apply.
	if err := state.RecordAppliedProfile(p.Name); err != nil {
		fmt.Printf("  %s\n", styles.Dimmed.Render(fmt.Sprintf("note: could not record applied profile: %v", err)))
	}
	fmt.Println()
	fmt.Println(styles.Success.Render(fmt.Sprintf("Profile %s applied.", p.Name)))
	return nil
}

func runDiff(cmd *cobra.Command, args []string) error {
	ctx := cmdContext(cmd)
	p, err := profile.Load(args[0])
	if err != nil {
		return err
	}
	if err := validateProfile(p); err != nil {
		return err
	}

	fmt.Println(styles.Title.Render("Drift from profile: " + p.Name))
	fmt.Println()

	drift := 0

	installed := comp.InstalledSet()
	present := 0
	for _, c := range p.Components {
		if installed[c] {
			present++
			continue
		}
		drift++
		fmt.Printf("  %s component %s %s\n", styles.Error.Render("✗"), c, styles.Dimmed.Render("not installed"))
	}
	fmt.Printf("  %s\n", styles.Dimmed.Render(fmt.Sprintf("%d/%d components installed", present, len(p.Components))))

	for _, slug := range p.Configure {
		settings := setup.FilterByHardware(setup.FilterBySlug(setup.Registry, slug))
		if len(settings) == 0 {
			continue
		}
		states := setup.InitStates(ctx, settings)
		for i := range states {
			s := states[i].Setting
			if s.Default == "" || states[i].CurrentValue == s.Default {
				continue
			}
			drift++
			fmt.Printf("  %s %s: %s %s\n", styles.Warning.Render("≠"), s.Name,
				states[i].CurrentValue,
				styles.Dimmed.Render(fmt.Sprintf("(recommended: %s)", s.Default)))
		}
	}

	fmt.Println()
	if drift > 0 {
		return fmt.Errorf("%d item(s) drift from profile %s (fix with: ctdev apply %s)", drift, p.Name, p.Name)
	}
	fmt.Println(styles.Success.Render("No drift — machine matches the profile."))
	return nil
}
