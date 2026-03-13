package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/checklist"
	"github.com/spf13/cobra"
)

var (
	flagYes         bool
	flagCheck       bool
	flagRefreshKeys bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update system packages and components",
	Long:  "Check for and install available updates across system packages, components, and runtimes.",
	RunE:  runUpdate,
}

func init() {
	updateCmd.Flags().BoolVarP(&flagYes, "yes", "y", false, "skip confirmation (install all updates)")
	updateCmd.Flags().BoolVar(&flagCheck, "check", false, "list available updates without installing")
	updateCmd.Flags().BoolVar(&flagRefreshKeys, "refresh-keys", false, "refresh APT GPG keys before updating")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	if flagRefreshKeys {
		fmt.Println("Refreshing APT GPG keys...")
		refreshAPTKeys(args)
	}

	fmt.Println("Scanning for updates...")
	items := scanAll(context.Background())

	if len(items) == 0 {
		fmt.Println("Everything is up to date.")
		return nil
	}

	if flagCheck {
		printUpdateList(items)
		return nil
	}

	var selected []checklist.UpdateItem
	if isBatchMode() || flagYes {
		selected = items
	} else {
		p := tea.NewProgram(checklist.New(items))
		result, err := p.Run()
		if err != nil {
			return err
		}
		checkResult := result.(checklist.Model).GetResult()
		if checkResult.Quit || len(checkResult.Selected) == 0 {
			return nil
		}
		selected = checkResult.Selected
	}

	return executeUpdates(selected)
}

func scanAll(ctx context.Context) []checklist.UpdateItem {
	var mu sync.Mutex
	var allItems []checklist.UpdateItem

	var wg sync.WaitGroup

	// Scan APT
	wg.Add(1)
	go func() {
		defer wg.Done()
		items, err := scanAPT(ctx)
		if err == nil && len(items) > 0 {
			mu.Lock()
			allItems = append(allItems, items...)
			mu.Unlock()
		}
	}()

	// Scan Flatpak
	wg.Add(1)
	go func() {
		defer wg.Done()
		items, err := scanFlatpak(ctx)
		if err == nil && len(items) > 0 {
			mu.Lock()
			allItems = append(allItems, items...)
			mu.Unlock()
		}
	}()

	// Scan Brew
	wg.Add(1)
	go func() {
		defer wg.Done()
		items, err := scanBrew(ctx)
		if err == nil && len(items) > 0 {
			mu.Lock()
			allItems = append(allItems, items...)
			mu.Unlock()
		}
	}()

	wg.Wait()
	return allItems
}

func scanAPT(ctx context.Context) ([]checklist.UpdateItem, error) {
	if _, err := exec.LookPath("apt"); err != nil {
		return nil, nil
	}
	out, err := exec.CommandContext(ctx, "apt", "list", "--upgradable").Output()
	if err != nil {
		return nil, err
	}
	var items []checklist.UpdateItem
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "[upgradable") {
			continue
		}
		parts := strings.SplitN(line, "/", 2)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		// Parse version info
		rest := parts[1]
		fields := strings.Fields(rest)
		newVer := ""
		currentVer := ""
		if len(fields) >= 2 {
			newVer = fields[1]
		}
		fromIdx := strings.Index(rest, "from: ")
		if fromIdx >= 0 {
			currentVer = strings.TrimRight(rest[fromIdx+6:], "]")
		}

		item := checklist.UpdateItem{
			Name:       name,
			Source:     "apt",
			CurrentVer: currentVer,
			NewVer:     newVer,
		}
		// Detect kernel updates
		if strings.HasPrefix(name, "linux-") {
			item.IsKernel = true
		}
		items = append(items, item)
	}
	return items, nil
}

func scanFlatpak(ctx context.Context) ([]checklist.UpdateItem, error) {
	if _, err := exec.LookPath("flatpak"); err != nil {
		return nil, nil
	}
	out, err := exec.CommandContext(ctx, "flatpak", "remote-ls", "--updates", "--columns=application,version").Output()
	if err != nil {
		return nil, err
	}
	var items []checklist.UpdateItem
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		name := fields[0]
		newVer := ""
		if len(fields) >= 2 {
			newVer = fields[1]
		}
		items = append(items, checklist.UpdateItem{
			Name:   name,
			Source: "flatpak",
			NewVer: newVer,
		})
	}
	return items, nil
}

func scanBrew(ctx context.Context) ([]checklist.UpdateItem, error) {
	if _, err := exec.LookPath("brew"); err != nil {
		return nil, nil
	}
	out, err := exec.CommandContext(ctx, "brew", "outdated", "--verbose").Output()
	if err != nil {
		return nil, err
	}
	var items []checklist.UpdateItem
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		// Format: "pkg (installed) < available"
		parts := strings.Fields(line)
		if len(parts) < 4 {
			continue
		}
		name := parts[0]
		currentVer := strings.Trim(parts[1], "()")
		newVer := parts[3]
		items = append(items, checklist.UpdateItem{
			Name:       name,
			Source:     "brew",
			CurrentVer: currentVer,
			NewVer:     newVer,
		})
	}
	return items, nil
}

func printUpdateList(items []checklist.UpdateItem) {
	currentSource := ""
	for _, item := range items {
		if item.Source != currentSource {
			currentSource = item.Source
			fmt.Printf("\n%s:\n", strings.ToUpper(currentSource))
		}
		fmt.Printf("  %-30s %s → %s", item.Name, item.CurrentVer, item.NewVer)
		if item.IsMajor {
			fmt.Print(" [MAJOR]")
		}
		if item.IsKernel {
			fmt.Print(" [KERNEL]")
		}
		fmt.Println()
	}
	fmt.Printf("\n%d updates available\n", len(items))
}

func executeUpdates(items []checklist.UpdateItem) error {
	// Group by source and execute
	aptPkgs := []string{}
	for _, item := range items {
		switch item.Source {
		case "apt":
			aptPkgs = append(aptPkgs, item.Name)
		}
	}

	if len(aptPkgs) > 0 {
		fmt.Printf("Updating %d apt packages...\n", len(aptPkgs))
		args := append([]string{"apt", "upgrade", "-y"}, aptPkgs...)
		cmd := exec.Command("sudo", args...)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if flagDryRun {
			fmt.Printf("  [dry-run] sudo apt upgrade -y %s\n", strings.Join(aptPkgs, " "))
		} else {
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("apt upgrade failed: %w", err)
			}
		}
	}

	// Flatpak updates
	hasFlatpak := false
	for _, item := range items {
		if item.Source == "flatpak" {
			hasFlatpak = true
			break
		}
	}
	if hasFlatpak {
		fmt.Println("Updating flatpak packages...")
		if flagDryRun {
			fmt.Println("  [dry-run] flatpak update -y")
		} else {
			cmd := exec.Command("flatpak", "update", "-y")
			if err := cmd.Run(); err != nil {
				fmt.Printf("  flatpak update warning: %v\n", err)
			}
		}
	}

	// Brew updates
	hasBrew := false
	for _, item := range items {
		if item.Source == "brew" {
			hasBrew = true
			break
		}
	}
	if hasBrew {
		fmt.Println("Updating brew packages...")
		if flagDryRun {
			fmt.Println("  [dry-run] brew upgrade")
		} else {
			cmd := exec.Command("brew", "upgrade")
			if err := cmd.Run(); err != nil {
				fmt.Printf("  brew upgrade warning: %v\n", err)
			}
		}
	}

	fmt.Println("Updates complete.")
	return nil
}

func refreshAPTKeys(components []string) {
	if _, err := exec.LookPath("apt"); err != nil {
		return
	}
	// Shell out to existing key refresh scripts
	fmt.Println("Refreshing APT GPG keys...")
	// This will be wired to bash scripts that handle key refreshing
}
