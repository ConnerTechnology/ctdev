package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	updateCmd.Flags().BoolVar(&flagNoRefresh, "no-refresh", false, "skip refreshing the package index (APT / Homebrew) before scanning")
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
	refreshBrew(ctx)

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

	if !flagDryRun && updateNeedsRoot(selected) {
		if err := ensureSudo(ctx); err != nil {
			return fmt.Errorf("sudo required for updates: %w", err)
		}
	}

	return executeUpdates(ctx, selected)
}

// updateNeedsRoot reports whether applying these items will shell out as root,
// mirroring component.InstallNeedsRoot. It gates the up-front password prompt so
// a Mac upgrading only Homebrew formulae — which land in the brew prefix and
// never touch root — isn't asked for one.
//
// The classification tracks what buildUpdateSteps actually runs, not what the
// source "feels" like:
//   - apt shells out through SudoRun.
//   - brew formulae don't, but casks do: Homebrew escalates internally for pkg
//     payloads and system extensions.
//   - among the runtimes only go is privileged (sudo rm/mv into /usr/local/go);
//     bun, nodenv and rbenv all stay inside $HOME.
//   - the cli tools resolve their destination at run time via `which` and fall
//     back to /usr/local/bin, so assume root.
//   - docker stacks can rebuild through `sudo docker compose`.
//
// flatpak, npm, oh-my-zsh and the ctdev self-update run unprivileged (ctdev's
// install.sh targets ~/.local/bin and only ever uses `sudo -n`).
func updateNeedsRoot(items []checklist.UpdateItem) bool {
	for _, item := range items {
		switch item.Source {
		case "apt", "brew-cask", "docker", "cli":
			return true
		case "runtime":
			if item.Name == "go" {
				return true
			}
		}
	}
	return false
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
	if err := ensureSudo(ctx); err != nil {
		fmt.Printf("  %s\n", styles.Warning.Render(fmt.Sprintf("sudo unavailable; scanning against a possibly stale index: %v", err)))
		return
	}
	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}
	if err := sysutil.APTUpdate(ctx, o); err != nil {
		fmt.Printf("  %s\n", styles.Warning.Render(fmt.Sprintf("apt index refresh failed; results may be stale: %v", err)))
	}
}

// shouldRefreshBrew decides whether to refresh Homebrew's catalog before
// scanning. Mirrors shouldRefreshKeys: --dry-run and --no-refresh suppress it.
func shouldRefreshBrew(dryRun, noRefresh bool) bool {
	return !dryRun && !noRefresh
}

// refreshBrew runs `brew update` before scanning, the Homebrew counterpart to
// refreshAptIndex. ctdev sets HOMEBREW_NO_AUTO_UPDATE so brew never decides to
// refresh itself in the middle of a step; that makes an explicit refresh here
// necessary, or `brew outdated` would answer from an arbitrarily stale catalog
// and report "everything is up to date" while upgrades were pending.
//
// Failures are non-fatal — scanning a stale catalog beats aborting the update.
func refreshBrew(ctx context.Context) {
	if _, err := exec.LookPath("brew"); err != nil {
		return
	}
	if !shouldRefreshBrew(flagDryRun, flagNoRefresh) {
		return
	}
	fmt.Println(styles.Dimmed.Render("Refreshing Homebrew catalog..."))
	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}
	if err := sysutil.Run(ctx, o, "brew", "update", "--quiet"); err != nil {
		fmt.Printf("  %s\n", styles.Warning.Render(fmt.Sprintf("brew update failed; results may be stale: %v", err)))
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

// aptKeyRefresher describes one third-party APT repository whose signing key
// ctdev can re-download. KeyringPath is what this repo's installer writes and
// is only the fallback: the key is refreshed into whatever keyring the source
// files on disk actually name for RepoURL, because vendors rewrite those files
// themselves (vscode's package migrates vscode.list to a .sources file naming
// microsoft.gpg; GitHub's install instructions use /etc/apt/keyrings). Writing
// only the installer's path on such a machine leaves APT reading the stale key.
type aptKeyRefresher struct {
	KeyURL      string
	KeyringPath string
	RepoURL     string
}

var aptKeyRefreshers = map[string]aptKeyRefresher{
	"gh": {
		KeyURL:      "https://cli.github.com/packages/githubcli-archive-keyring.gpg",
		KeyringPath: "/usr/share/keyrings/githubcli-archive-keyring.gpg",
		RepoURL:     "https://cli.github.com/packages",
	},
	// KeyringPath must match what vscodeInstall writes.
	"vscode": {
		KeyURL:      "https://packages.microsoft.com/keys/microsoft.asc",
		KeyringPath: "/usr/share/keyrings/microsoft-archive-keyring.gpg",
		RepoURL:     "https://packages.microsoft.com/repos/code",
	},
	"1password": {
		KeyURL:      "https://downloads.1password.com/linux/keys/1password.asc",
		KeyringPath: "/usr/share/keyrings/1password-archive-keyring.gpg",
		RepoURL:     "https://downloads.1password.com/linux/debian",
	},
	"terraform": {
		KeyURL:      "https://apt.releases.hashicorp.com/gpg",
		KeyringPath: "/usr/share/keyrings/hashicorp-archive-keyring.gpg",
		RepoURL:     "https://apt.releases.hashicorp.com",
	},
	"tailscale": {
		KeyURL:      "https://pkgs.tailscale.com/stable/ubuntu/noble.noarmor.gpg",
		KeyringPath: "/usr/share/keyrings/tailscale-archive-keyring.gpg",
		RepoURL:     "https://pkgs.tailscale.com/stable",
	},
}

const aptSourcesDir = "/etc/apt/sources.list.d"

func refreshAPTKeys(ctx context.Context, components []string) {
	if _, err := exec.LookPath("apt"); err != nil {
		return
	}
	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}
	targets := aptKeyRefreshers
	if len(components) > 0 {
		targets = make(map[string]aptKeyRefresher)
		for _, name := range components {
			if r, ok := aptKeyRefreshers[name]; ok {
				targets[name] = r
			}
		}
	}
	for name, r := range targets {
		for _, keyring := range aptKeyringTargets(aptSourcesDir, r) {
			fmt.Println(styles.Dimmed.Render(fmt.Sprintf("  Refreshing %s key → %s", name, keyring)))
			if err := sysutil.AddAPTKeyring(ctx, o, r.KeyURL, keyring); err != nil {
				fmt.Printf("  %s\n", styles.Warning.Render(fmt.Sprintf("Warning: %s key refresh failed: %v", name, err)))
			}
		}
	}
}

// aptKeyringTargets returns the keyrings to refresh for one repo: every
// signed-by path a source file names for it, or the installer's own path when
// no source file references the repo at all.
func aptKeyringTargets(sourcesDir string, r aptKeyRefresher) []string {
	if paths := aptSignedByPaths(sourcesDir, r.RepoURL); len(paths) > 0 {
		return paths
	}
	return []string{r.KeyringPath}
}

// aptSignedByPaths scans every .list and .sources file in sourcesDir and
// returns, sorted and deduplicated, the signed-by keyring paths of entries
// whose URI starts with repoURL. Commented-out lines are ignored, as is a
// deb822 Signed-By that embeds the key inline instead of naming a file.
func aptSignedByPaths(sourcesDir, repoURL string) []string {
	entries, err := os.ReadDir(sourcesDir)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sourcesDir, e.Name()))
		if err != nil {
			continue
		}
		var found []string
		switch filepath.Ext(e.Name()) {
		case ".list":
			found = oneLineSignedBy(string(data), repoURL)
		case ".sources":
			found = deb822SignedBy(string(data), repoURL)
		}
		for _, p := range found {
			seen[p] = true
		}
	}
	if len(seen) == 0 {
		return nil
	}
	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// oneLineSignedBy handles the classic format:
//
//	deb [arch=amd64 signed-by=/path.gpg] https://host/repo suite component
func oneLineSignedBy(content, repoURL string) []string {
	var paths []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "deb ") && !strings.HasPrefix(line, "deb-src ") {
			continue
		}
		lb, rb := strings.Index(line, "["), strings.Index(line, "]")
		if lb < 0 || rb < lb {
			continue
		}
		rest := strings.Fields(line[rb+1:])
		if len(rest) == 0 || !strings.HasPrefix(rest[0], repoURL) {
			continue
		}
		for _, opt := range strings.Fields(line[lb+1 : rb]) {
			if p, ok := strings.CutPrefix(opt, "signed-by="); ok && strings.HasPrefix(p, "/") {
				paths = append(paths, p)
			}
		}
	}
	return paths
}

// deb822SignedBy handles the stanza format: blank-line-separated blocks of
// "Field: value" lines, where URIs may list several and Signed-By may hold an
// inline key on continuation lines rather than a path.
func deb822SignedBy(content, repoURL string) []string {
	var paths []string
	for _, stanza := range strings.Split(content, "\n\n") {
		var matches bool
		var signedBy string
		for _, line := range strings.Split(stanza, "\n") {
			field, value, ok := strings.Cut(line, ":")
			if !ok || strings.HasPrefix(line, "#") || strings.HasPrefix(line, " ") {
				continue
			}
			value = strings.TrimSpace(value)
			switch strings.ToLower(field) {
			case "uris":
				for _, uri := range strings.Fields(value) {
					if strings.HasPrefix(uri, repoURL) {
						matches = true
					}
				}
			case "signed-by":
				signedBy = value
			}
		}
		if matches && strings.HasPrefix(signedBy, "/") {
			paths = append(paths, signedBy)
		}
	}
	return paths
}
