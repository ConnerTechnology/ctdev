package cmd

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	comp "github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/picker"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/progress"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:               "install [component...]",
	Short:             "Install components",
	Long:              "Install one or more components. Run without arguments for interactive picker.",
	RunE:              runInstall,
	ValidArgsFunction: completeComponentNames,
}

func init() {
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	var selected []string

	if len(args) > 0 {
		for _, name := range args {
			if comp.FindByName(name) == nil {
				return fmt.Errorf("unknown component: %s", name)
			}
		}
		selected = args
	} else if isBatchMode() {
		return fmt.Errorf("no components specified (batch mode requires arguments)")
	} else {
		installed := comp.InstalledSet()
		osType := comp.OS(platform.Detect().OS)
		m := picker.New(comp.Registry, installed, osType, picker.ModeInstall, flagDryRun)
		p := tea.NewProgram(&m)
		result, err := p.Run()
		resetTerminal()
		if err != nil {
			return err
		}
		pickerResult := result.(*picker.Model).GetResult()
		if pickerResult.Quit {
			return nil
		}
		if len(pickerResult.Selected) == 0 {
			fmt.Println("No components selected.")
			return nil
		}
		selected = pickerResult.Selected
	}

	// Track what the user actually asked for (vs. auto-pulled dependencies) so
	// only those get a configuration step, and note what was already installed.
	requested := append([]string(nil), selected...)
	installedBefore := comp.InstalledSet()

	resolved := comp.ResolveDependencies(comp.Registry, selected)

	// Say up front what dependency resolution added, so the first time the user
	// learns docker is coming isn't when it appears in the progress list.
	requestedSet := map[string]bool{}
	for _, name := range requested {
		requestedSet[name] = true
	}
	var deps []string
	for _, name := range resolved {
		if !requestedSet[name] {
			deps = append(deps, name)
		}
	}
	if len(deps) > 0 {
		fmt.Printf("Installing %d selected + %d dependencies: %s\n",
			len(requested), len(deps), strings.Join(deps, ", "))
	}

	// Only ask for a password when something in this run will use it — a
	// dotfiles-only install (zsh configs, claude-code, fonts) works fine in a
	// container that has no sudo at all.
	if !flagDryRun && comp.InstallNeedsRoot(platform.Detect().PackageManager, resolved, flagForce) {
		if err := ensureSudo(cmdContext(cmd)); err != nil {
			return fmt.Errorf("sudo required for install: %w", err)
		}
	}
	if err := runWithProgress(cmd.Context(), progressOperation{
		mode:  progress.ModeInstall,
		names: resolved,
	}); err != nil {
		return err
	}

	// install = install + configure: after installing, run each requested
	// component's configuration step when it has one. `ctdev configure <x>`
	// alone still configures without installing. Skipped in batch/dry-run
	// because the wizards are interactive.
	if flagDryRun || isBatchMode() {
		return nil
	}
	for _, name := range requested {
		hasCfg := componentHasConfigure(name)
		if installedBefore[name] {
			if hasCfg {
				fmt.Printf("\n%s is already installed — opening its configuration.\n", name)
			} else {
				fmt.Printf("\n%s is already installed.\n", name)
			}
		}
		if hasCfg {
			if err := runComponentConfigure(cmd.Context(), name); err != nil {
				fmt.Printf("  configure %s: %v\n", name, err)
			}
		}
	}
	return nil
}

// componentWizards are the components whose configuration step is a dedicated
// wizard rather than a `configure <name>` category from setup.Registry.
var componentWizards = map[string]func(context.Context) error{
	"brain":            configureBrain,
	"caddy":            configureCaddy,
	"mcp-email-server": configureMCPEmailServer,
}

// componentHasConfigure reports whether a component has a configuration step —
// a dedicated wizard or a `configure <name>` category sharing its name.
func componentHasConfigure(name string) bool {
	if componentWizards[name] != nil {
		return true
	}
	for _, slug := range slugOrder {
		if slug == name {
			return true
		}
	}
	return false
}

// runComponentConfigure runs a component's configuration step — its dedicated
// wizard or its `configure <name>` category.
func runComponentConfigure(ctx context.Context, name string) error {
	if wizard := componentWizards[name]; wizard != nil {
		return wizard(ctx)
	}
	return runCategoryWizard(ctx, name, false)
}
