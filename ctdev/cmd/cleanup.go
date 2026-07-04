package cmd

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/cleanup"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/multiselect"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Reclaim disk space (caches, logs, package junk, trash)",
	Long: "Scan for reclaimable disk space and clean it up — package caches and orphans, " +
		"logs, snap/flatpak leftovers, Docker junk on Linux; Homebrew, Xcode, caches and " +
		"trash on macOS. Safe tasks are preselected; riskier ones are shown unchecked, and " +
		"user data (device backups) is only reported, never deleted.",
	RunE: runCleanup,
}

func init() {
	rootCmd.AddCommand(cleanupCmd)
}

func runCleanup(cmd *cobra.Command, args []string) error {
	info := platform.Detect()
	tasks := cleanup.Catalog(info)
	if len(tasks) == 0 {
		return fmt.Errorf("cleanup is not supported on %s", info.OS)
	}
	ctx := cmdContext(cmd)

	fmt.Println(styles.Dimmed.Render("Scanning for reclaimable space…"))
	results := cleanup.ScanAll(ctx, tasks)

	// Surface report-only findings (user data we never delete).
	printReportOnly(tasks, results)

	// Partition the actionable tasks that actually have something to do.
	var actionable []cleanup.Task
	for _, t := range tasks {
		if t.Risk != cleanup.ReportOnly && t.Run != nil && hasWork(results[t.ID]) {
			actionable = append(actionable, t)
		}
	}
	if len(actionable) == 0 {
		fmt.Println(styles.Success.Render("\nNothing to clean — you're tidy."))
		return nil
	}

	if flagDryRun {
		printActionable(actionable, results)
		fmt.Println(styles.Dimmed.Render("\n  [dry-run] No changes made."))
		return nil
	}

	// Select what to run: safe tier in batch, the picker interactively.
	var selectedIDs []string
	if isBatchMode() {
		for _, t := range actionable {
			if t.Risk == cleanup.Safe {
				selectedIDs = append(selectedIDs, t.ID)
			}
		}
		printActionable(actionable, results)
	} else {
		ids, quit, err := pickCleanupTasks(actionable, results)
		if err != nil {
			return err
		}
		if quit {
			return nil
		}
		selectedIDs = ids
	}

	if len(selectedIDs) == 0 {
		fmt.Println("Nothing selected.")
		return nil
	}

	selected := map[string]bool{}
	for _, id := range selectedIDs {
		selected[id] = true
	}

	// Enter in the picker lands here — deletion needs one explicit confirmation,
	// like the configure wizard's Apply step, so a reflexive Enter can't clean.
	if !isBatchMode() {
		var total int64
		fmt.Println()
		fmt.Println(styles.Dimmed.Render("  Will clean:"))
		for _, t := range actionable {
			if !selected[t.ID] {
				continue
			}
			fmt.Printf("    %s %s\n", styles.Value.Render(t.Name), styles.Dimmed.Render(sizeLabel(results[t.ID])))
			if b := results[t.ID].Bytes; b > 0 {
				total += b
			}
		}
		fmt.Printf("\n  Reclaims ≈ %s\n", cleanup.Humanize(total))
		yes, err := promptYesNoCtx(ctx, "Clean these?", true)
		if err != nil {
			return cancelToClean(err)
		}
		if !yes {
			fmt.Println(styles.Dimmed.Render("  Skipped — nothing was cleaned."))
			return nil
		}
	}

	if err := ensureSudo(); err != nil {
		return fmt.Errorf("sudo required for cleanup: %w", err)
	}

	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}
	var reclaimed int64
	for _, t := range actionable {
		if !selected[t.ID] {
			continue
		}
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Cleaning: %s…", t.Name)))
		if err := t.Run(ctx, o); err != nil {
			fmt.Printf("  %s\n", styles.Warning.Render(fmt.Sprintf("warning: %v", err)))
			continue
		}
		if b := results[t.ID].Bytes; b > 0 {
			reclaimed += b
		}
	}

	fmt.Println(styles.Success.Render(fmt.Sprintf("\nCleanup complete — reclaimed ≈ %s.", cleanup.Humanize(reclaimed))))
	return nil
}

// hasWork reports whether a scan found anything worth offering.
func hasWork(r cleanup.ScanResult) bool {
	if r.Bytes > 0 {
		return true
	}
	if r.Bytes < 0 { // unknown size — offer unless explicitly "none"
		return r.Note != "none"
	}
	return false
}

// sizeLabel renders a scan result as the dimmed detail next to a task.
func sizeLabel(r cleanup.ScanResult) string {
	switch {
	case r.Bytes > 0 && r.Note != "":
		return cleanup.Humanize(r.Bytes) + " · " + r.Note
	case r.Bytes > 0:
		return cleanup.Humanize(r.Bytes)
	case r.Note != "":
		return r.Note
	default:
		return "—"
	}
}

func printReportOnly(tasks []cleanup.Task, results map[string]cleanup.ScanResult) {
	var found []cleanup.Task
	for _, t := range tasks {
		if t.Risk == cleanup.ReportOnly && hasWork(results[t.ID]) {
			found = append(found, t)
		}
	}
	if len(found) == 0 {
		return
	}
	fmt.Println(styles.Header.Render("\nFound (not cleaned):"))
	for _, t := range found {
		fmt.Printf("  %s %s %s\n",
			styles.Warning.Render("•"),
			styles.Value.Render(t.Name),
			styles.Dimmed.Render(sizeLabel(results[t.ID])))
		if t.Detail != "" {
			fmt.Printf("    %s\n", styles.Dimmed.Render(t.Detail))
		}
	}
}

func printActionable(tasks []cleanup.Task, results map[string]cleanup.ScanResult) {
	var total int64
	fmt.Println()
	for _, t := range tasks {
		fmt.Printf("  %s %s\n",
			styles.Label(38).Render(t.Name),
			styles.Value.Render(sizeLabel(results[t.ID])))
		if b := results[t.ID].Bytes; b > 0 {
			total += b
		}
	}
	fmt.Printf("\n  %s ≈ %s\n", styles.Header.Render("Reclaimable"), cleanup.Humanize(total))
}

// pickCleanupTasks shows the grouped multi-select picker and returns the chosen
// task IDs. Safe tasks start checked; opt-in tasks start unchecked.
func pickCleanupTasks(tasks []cleanup.Task, results map[string]cleanup.ScanResult) (ids []string, quit bool, err error) {
	var order []string
	byGroup := map[string][]multiselect.Item{}
	for _, t := range tasks {
		if _, ok := byGroup[t.Group]; !ok {
			order = append(order, t.Group)
		}
		byGroup[t.Group] = append(byGroup[t.Group], multiselect.Item{
			ID:          t.ID,
			Primary:     t.Name,
			Secondary:   sizeLabel(results[t.ID]),
			Note:        t.Detail,
			Selectable:  true,
			Bulk:        true,
			NoPreselect: t.Risk == cleanup.OptIn,
		})
	}
	var groups []multiselect.Group
	for _, g := range order {
		groups = append(groups, multiselect.Group{Key: g, Title: g, Items: byGroup[g]})
	}

	m := multiselect.New(groups, multiselect.Options{
		Title:        "Reclaim disk space",
		StatusSuffix: "· space to toggle · enter to review & clean",
		PreselectAll: true,
	})
	wrapped := &cleanupPicker{ms: m}
	res, runErr := tea.NewProgram(wrapped).Run()
	resetTerminal()
	if runErr != nil {
		return nil, false, runErr
	}
	result := res.(*cleanupPicker).ms.Result()
	if result.Quit {
		return nil, true, nil
	}
	return result.Selected, false, nil
}

// cleanupPicker adapts multiselect.Model (whose Update returns only a Cmd) to
// the tea.Model interface for standalone use.
type cleanupPicker struct{ ms *multiselect.Model }

func (p *cleanupPicker) Init() tea.Cmd                           { return p.ms.Init() }
func (p *cleanupPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return p, p.ms.Update(msg) }
func (p *cleanupPicker) View() tea.View                          { return p.ms.View() }
