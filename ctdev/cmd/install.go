package cmd

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	comp "github.com/ConnerTechnology/ctdev/ctdev/component"
	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/picker"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/progress"
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
	var requested []string
	var installedBefore map[string]bool
	batch := isBatchMode()
	configureAfter := !batch && !flagDryRun

	if len(args) > 0 {
		if configureAfter {
			installedBefore = make(map[string]bool, len(args))
		}
		for _, name := range args {
			c := comp.FindByName(name)
			if c == nil {
				return fmt.Errorf("unknown component: %s", name)
			}
			if configureAfter {
				installedBefore[name] = c.IsInstalled()
			}
		}
		requested = args
	} else if batch {
		return fmt.Errorf("no components specified (batch mode requires arguments)")
	} else {
		installedBefore = comp.InstalledSet()
		osType := comp.OS(platform.Detect().OS)
		m := picker.New(comp.Registry, installedBefore, osType, picker.ModeInstall, flagDryRun)
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
		requested = pickerResult.Selected
	}

	resolved := comp.ResolveDependencies(comp.Registry, requested)

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

	ctx := cmdContext(cmd)
	if err := ensureSudoForInstall(ctx, resolved, flagForce); err != nil {
		return err
	}
	installErr := runWithProgress(ctx, progressOperation{
		mode:  progress.ModeInstall,
		names: resolved,
	})
	// claude-code's drift review runs whenever it was part of this install, in
	// every mode: it's how --dry-run shows the diff and --force says where the
	// backup went, which the progress screen above can't.
	if err := reviewClaudeCodeAfterInstall(ctx, resolved); err != nil {
		return cancelToClean(err)
	}
	if installErr != nil {
		return installErr
	}

	// install = install + configure: after installing, run each requested
	// component's configuration step when it has one. `ctdev configure <x>`
	// alone still configures without installing, except pihole, which has no
	// subcommand (CTD-171) and is reached by re-running install. Skipped in
	// batch/dry-run because the wizards are interactive.
	if !configureAfter {
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
			if err := runComponentConfigure(ctx, name); err != nil {
				fmt.Printf("  configure %s: %v\n", name, err)
			}
		}
	}
	return nil
}

// ensureSudoForInstall caches a sudo password before the progress TUI takes
// the terminal, but only when something in this install will use it — a
// dotfiles-only install (zsh configs, claude-code, fonts) works fine in a
// container that has no sudo at all.
func ensureSudoForInstall(ctx context.Context, names []string, force bool) error {
	if flagDryRun || !comp.InstallNeedsRoot(platform.Detect().PackageManager, names, force) {
		return nil
	}
	if err := ensureSudo(ctx); err != nil {
		return fmt.Errorf("sudo required for install: %w", err)
	}
	return nil
}

// componentWizards are the components whose configuration step is a dedicated
// wizard rather than a setup.Registry category from setup.Registry.
var componentWizards = map[string]func(context.Context) error{
	"aws":    configureAWS,
	"caddy":  configureCaddy,
	"git":    configureGit,
	"nodenv": configureNodeAfterInstall,
	// ssh's settings category is one entry in its menu, which `configure
	// ssh` opens, so installing it opens the same menu.
	"ssh": runSSHMenu,
}

// componentHasConfigure reports whether a component has a configuration step —
// a dedicated wizard or a setup.Registry category sharing its name.
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
// wizard or its setup.Registry category.
func runComponentConfigure(ctx context.Context, name string) error {
	if wizard := componentWizards[name]; wizard != nil {
		return wizard(ctx)
	}
	return runCategoryWizard(ctx, name, false)
}
