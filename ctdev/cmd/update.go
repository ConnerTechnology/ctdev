package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/checklist"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var (
	flagYes         bool
	flagCheck       bool
	flagRefreshKeys bool
	flagNoRefresh   bool
)

var updateCmd = &cobra.Command{
	Use:   "update [component...]",
	Short: "Update system packages and components",
	Long: "Check for and install available updates across system packages, components, and runtimes.\n\n" +
		"Positional component names only narrow which APT keys --refresh-keys refreshes " +
		"(e.g. 'ctdev update --refresh-keys vscode'); they don't filter the update scan.",
	RunE: runUpdate,
}

func init() {
	updateCmd.Flags().BoolVarP(&flagYes, "yes", "y", false, "skip confirmation (install all updates)")
	updateCmd.Flags().BoolVar(&flagCheck, "check", false, "list available updates without installing")
	updateCmd.Flags().BoolVar(&flagRefreshKeys, "refresh-keys", false, "refresh APT GPG keys before updating")
	updateCmd.Flags().BoolVar(&flagNoRefresh, "no-refresh", false, "skip refreshing the APT package index before scanning")
	rootCmd.AddCommand(updateCmd)
}

// shouldRefreshKeys decides whether to run the (write-performing) APT key
// refresh. --check is read-only so it must suppress refresh even when the
// user also passed --refresh-keys.
func shouldRefreshKeys(refreshKeys, check bool) bool {
	return refreshKeys && !check
}

func runUpdate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if shouldRefreshKeys(flagRefreshKeys, flagCheck) {
		fmt.Println(styles.Dimmed.Render("Refreshing APT GPG keys..."))
		refreshAPTKeys(ctx, args)
	}

	// Refresh the APT index before scanning so `apt list --upgradable` reflects
	// reality. Without this, an update run on a machine with a stale index can
	// report "everything is up to date" while security updates are pending.
	refreshAptIndex(ctx)

	items := scanAll(ctx)

	if len(items) == 0 {
		fmt.Println(styles.Success.Render("Everything is up to date."))
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
		m := checklist.New(items)
		p := tea.NewProgram(&m)
		result, err := p.Run()
		resetTerminal()
		if err != nil {
			return err
		}
		checkResult := result.(*checklist.Model).GetResult()
		if checkResult.Quit || len(checkResult.Selected) == 0 {
			return nil
		}
		selected = checkResult.Selected
	}

	if !flagDryRun {
		if err := ensureSudo(); err != nil {
			return fmt.Errorf("sudo required for updates: %w", err)
		}
	}

	return executeUpdates(ctx, selected)
}

// refreshAptIndex runs `apt-get update` before scanning so the upgradable list
// is accurate. It is a no-op on non-apt systems, in dry-run, or when
// --no-refresh is passed. Failures (including an unavailable sudo) are
// non-fatal: we warn and let the scan proceed against the existing index rather
// than abort the whole update.
func refreshAptIndex(ctx context.Context) {
	if _, err := exec.LookPath("apt-get"); err != nil {
		return
	}
	if flagDryRun || flagNoRefresh {
		return
	}
	fmt.Println(styles.Dimmed.Render("Refreshing APT package index..."))
	if err := ensureSudo(); err != nil {
		fmt.Printf("  %s\n", styles.Warning.Render(fmt.Sprintf("sudo unavailable; scanning against a possibly stale index: %v", err)))
		return
	}
	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}
	if err := sysutil.APTUpdate(ctx, o); err != nil {
		fmt.Printf("  %s\n", styles.Warning.Render(fmt.Sprintf("apt index refresh failed; results may be stale: %v", err)))
	}
}

func printUpdateList(items []checklist.UpdateItem) {
	headerStyle := styles.Header
	labelStyle := styles.Label(30)
	valueStyle := styles.Value

	currentSource := ""
	for _, item := range items {
		if item.Source != currentSource {
			currentSource = item.Source
			fmt.Printf("\n%s\n", headerStyle.Render(strings.ToUpper(currentSource)+":"))
		}
		line := fmt.Sprintf("  %s %s", labelStyle.Render(item.Name), valueStyle.Render(item.CurrentVer+" → "+item.NewVer))
		if item.IsMajor {
			line += " " + styles.Warning.Render("[MAJOR]")
		}
		if item.IsKernel {
			line += " " + styles.Warning.Render("[KERNEL]")
		}
		fmt.Println(line)
	}
	fmt.Printf("\n%s\n", styles.Success.Render(fmt.Sprintf("%d updates available", len(items))))
}

func itemNames(items []checklist.UpdateItem) []string {
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = item.Name
	}
	return names
}

var aptKeyRefreshers = map[string]struct {
	KeyURL      string
	KeyringPath string
}{
	"gh": {KeyURL: "https://cli.github.com/packages/githubcli-archive-keyring.gpg", KeyringPath: "/usr/share/keyrings/githubcli-archive-keyring.gpg"},
	// KeyringPath must match what vscodeInstall writes.
	"vscode":    {KeyURL: "https://packages.microsoft.com/keys/microsoft.asc", KeyringPath: "/usr/share/keyrings/microsoft-archive-keyring.gpg"},
	"1password": {KeyURL: "https://downloads.1password.com/linux/keys/1password.asc", KeyringPath: "/usr/share/keyrings/1password-archive-keyring.gpg"},
	"terraform": {KeyURL: "https://apt.releases.hashicorp.com/gpg", KeyringPath: "/usr/share/keyrings/hashicorp-archive-keyring.gpg"},
	"tailscale": {KeyURL: "https://pkgs.tailscale.com/stable/ubuntu/noble.noarmor.gpg", KeyringPath: "/usr/share/keyrings/tailscale-archive-keyring.gpg"},
}

func refreshAPTKeys(ctx context.Context, components []string) {
	if _, err := exec.LookPath("apt"); err != nil {
		return
	}
	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}
	targets := aptKeyRefreshers
	if len(components) > 0 {
		targets = make(map[string]struct{ KeyURL, KeyringPath string })
		for _, name := range components {
			if r, ok := aptKeyRefreshers[name]; ok {
				targets[name] = r
			}
		}
	}
	for name, r := range targets {
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("  Refreshing %s key...", name)))
		if err := sysutil.AddAPTKeyring(ctx, o, r.KeyURL, r.KeyringPath); err != nil {
			fmt.Printf("  %s\n", styles.Warning.Render(fmt.Sprintf("Warning: %s key refresh failed: %v", name, err)))
		}
	}
}
