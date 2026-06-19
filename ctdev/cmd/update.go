package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/component"
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
	Use:   "update",
	Short: "Update system packages and components",
	Long:  "Check for and install available updates across system packages, components, and runtimes.",
	RunE:  runUpdate,
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

	return executeUpdates(ctx, items, selected)
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

func scanAll(ctx context.Context) []checklist.UpdateItem {
	var mu sync.Mutex
	var allItems []checklist.UpdateItem

	type namedScanner struct {
		name string
		fn   func(context.Context) ([]checklist.UpdateItem, error)
	}
	scanners := []namedScanner{
		{"apt", scanAPT},
		{"mintupdate", scanMintUpdate},
		{"flatpak", scanFlatpak},
		{"brew", scanBrew},
		{"brew-cask", scanBrewCask},
		{"oh-my-zsh", scanOhMyZsh},
		{"bun", scanBun},
		{"nodenv", scanNodeEnv},
		{"npm", scanNPMGlobals},
		{"ctdev", scanCtdev},
		{"go", scanGo},
		{"ruby", scanRuby},
		{"helm", scanHelm},
		{"kubectl", scanKubectl},
		{"terraform", scanTerraform},
	}

	// scanErrs collects failures so one broken source doesn't silently make the
	// machine look up to date. Protected by mu alongside allItems.
	var scanErrs []string

	total := len(scanners)
	done := make(chan struct{}, total)
	printerDone := make(chan struct{})

	go func() {
		defer close(printerDone)
		count := 0
		for range done {
			count++
			fmt.Printf("\r  %s", styles.Dimmed.Render(fmt.Sprintf("Scanning for updates... (%d/%d sources checked)", count, total)))
		}
		fmt.Println()
	}()

	var wg sync.WaitGroup
	for _, s := range scanners {
		s := s
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { done <- struct{}{} }()
			defer func() {
				if r := recover(); r != nil {
					// Record (don't crash on) a panicking scanner so one
					// failure doesn't take down the entire update check.
					mu.Lock()
					scanErrs = append(scanErrs, fmt.Sprintf("%s: panic: %v", s.name, r))
					mu.Unlock()
				}
			}()
			items, err := s.fn(ctx)
			if err != nil {
				mu.Lock()
				scanErrs = append(scanErrs, fmt.Sprintf("%s: %v", s.name, err))
				mu.Unlock()
				return
			}
			if len(items) > 0 {
				mu.Lock()
				allItems = append(allItems, items...)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	close(done)
	<-printerDone

	// Surface scanner failures so a rate-limited / broken source isn't mistaken
	// for "nothing to update". Detail is gated behind --verbose to keep the
	// common path quiet.
	if len(scanErrs) > 0 {
		if flagVerbose {
			for _, e := range scanErrs {
				fmt.Printf("  %s\n", styles.Warning.Render("update source failed — "+e))
			}
		} else {
			fmt.Printf("  %s\n", styles.Warning.Render(fmt.Sprintf("%d update source(s) failed to check (run with -v for detail)", len(scanErrs))))
		}
	}

	// Sort by source for grouped display
	sourceOrder := map[string]int{
		"apt": 0, "mintupdate": 1, "brew": 2, "brew-cask": 3, "flatpak": 4,
		"git": 5, "runtime": 6, "npm": 7,
		"cli": 8, "ctdev": 9,
	}
	sort.Slice(allItems, func(i, j int) bool {
		oi, oj := sourceOrder[allItems[i].Source], sourceOrder[allItems[j].Source]
		if oi != oj {
			return oi < oj
		}
		return allItems[i].Name < allItems[j].Name
	})

	return allItems
}

func parseAPTUpgradable(output string) []checklist.UpdateItem {
	var items []checklist.UpdateItem
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "[upgradable") {
			continue
		}
		parts := strings.SplitN(line, "/", 2)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
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
			Name: name, Source: "apt",
			CurrentVer: currentVer, NewVer: newVer,
		}
		if strings.HasPrefix(name, "linux-") {
			item.IsKernel = true
		}
		items = append(items, item)
	}
	return items
}

func scanAPT(ctx context.Context) ([]checklist.UpdateItem, error) {
	if _, err := exec.LookPath("apt"); err != nil {
		return nil, nil
	}
	out, err := exec.CommandContext(ctx, "apt", "list", "--upgradable").Output()
	if err != nil {
		return nil, err
	}
	return parseAPTUpgradable(string(out)), nil
}

func scanMintUpdate(ctx context.Context) ([]checklist.UpdateItem, error) {
	if _, err := exec.LookPath("mintupdate-cli"); err != nil {
		return nil, nil
	}
	out, err := exec.CommandContext(ctx, "mintupdate-cli", "list").Output()
	if err != nil {
		return nil, err
	}
	return parseMintUpdateList(string(out)), nil
}

func parseMintUpdateList(output string) []checklist.UpdateItem {
	var items []checklist.UpdateItem
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		updateType := fields[0]
		name := fields[1]
		newVer := fields[2]
		item := checklist.UpdateItem{
			Name:   name,
			Source: "mintupdate",
			NewVer: newVer,
		}
		if updateType == "kernel" {
			item.IsKernel = true
		}
		items = append(items, item)
	}
	return items
}

func parseFlatpakUpdates(output string) []checklist.UpdateItem {
	var items []checklist.UpdateItem
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
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
			Name: name, Source: "flatpak", NewVer: newVer,
		})
	}
	return items
}

func scanFlatpak(ctx context.Context) ([]checklist.UpdateItem, error) {
	if _, err := exec.LookPath("flatpak"); err != nil {
		return nil, nil
	}
	out, err := exec.CommandContext(ctx, "flatpak", "remote-ls", "--updates", "--columns=application,version").Output()
	if err != nil {
		return nil, err
	}
	return parseFlatpakUpdates(string(out)), nil
}

func parseBrewOutdated(output string) []checklist.UpdateItem {
	var items []checklist.UpdateItem
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 4 {
			continue
		}
		items = append(items, checklist.UpdateItem{
			Name:       parts[0],
			Source:     "brew",
			CurrentVer: strings.Trim(parts[1], "()"),
			NewVer:     parts[3],
		})
	}
	return items
}

func scanBrew(ctx context.Context) ([]checklist.UpdateItem, error) {
	if _, err := exec.LookPath("brew"); err != nil {
		return nil, nil
	}
	out, err := exec.CommandContext(ctx, "brew", "outdated", "--verbose").Output()
	if err != nil {
		return nil, err
	}
	return parseBrewOutdated(string(out)), nil
}

// managedCasks maps brew cask names to ctdev component names.
// Only installed components are shown in update output.
var managedCasks = map[string]string{
	"1password":                     "1password",
	"chatgpt":                       "chatgpt",
	"cleanmymac":                    "cleanmymac",
	"claude":                        "claude-desktop",
	"dbeaver-community":             "dbeaver",
	"docker":                        "docker",
	"ghostty":                       "ghostty",
	"google-chrome":                 "chrome",
	"linear-linear":                 "linear",
	"logi-options+":                 "logi-options",
	"slack":                         "slack",
	"tailscale":                     "tailscale",
	"visual-studio-code":            "vscode",
	"font-fira-code-nerd-font":      "fonts",
	"font-jetbrains-mono-nerd-font": "fonts",
}

func scanBrewCask(ctx context.Context) ([]checklist.UpdateItem, error) {
	if _, err := exec.LookPath("brew"); err != nil {
		return nil, nil
	}
	out, err := exec.CommandContext(ctx, "brew", "outdated", "--cask", "--greedy", "--verbose").Output()
	if err != nil {
		return nil, err
	}
	return parseBrewCaskOutdated(string(out)), nil
}

func parseBrewCaskOutdated(output string) []checklist.UpdateItem {
	var items []checklist.UpdateItem
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		// Format: name (current_ver) != new_ver
		parts := strings.Fields(line)
		if len(parts) < 4 {
			continue
		}
		name := parts[0]
		compName, managed := managedCasks[name]
		if !managed {
			continue
		}
		comp := component.FindByName(compName)
		if comp == nil || !comp.IsInstalled() {
			continue
		}
		items = append(items, checklist.UpdateItem{
			Name:       name,
			Source:     "brew-cask",
			CurrentVer: strings.Trim(parts[1], "()"),
			NewVer:     parts[3],
		})
	}
	return items
}

func scanOhMyZsh(ctx context.Context) ([]checklist.UpdateItem, error) {
	omzDir := os.ExpandEnv("$HOME/.oh-my-zsh")
	if _, err := os.Stat(omzDir + "/.git"); err != nil {
		return nil, nil
	}
	// Fetch latest from remote
	fetch := exec.CommandContext(ctx, "git", "-C", omzDir, "fetch", "--quiet")
	if err := fetch.Run(); err != nil {
		return nil, fmt.Errorf("git fetch oh-my-zsh: %w", err)
	}
	// Check if behind
	out, err := exec.CommandContext(ctx, "git", "-C", omzDir, "rev-list", "--count", "HEAD..@{u}").Output()
	if err != nil {
		return nil, fmt.Errorf("oh-my-zsh rev-list: %w", err)
	}
	behind := strings.TrimSpace(string(out))
	if behind == "" || behind == "0" {
		return nil, nil
	}
	// Get current and remote short SHAs
	currentSHA, _ := exec.CommandContext(ctx, "git", "-C", omzDir, "rev-parse", "--short", "HEAD").Output()
	remoteSHA, _ := exec.CommandContext(ctx, "git", "-C", omzDir, "rev-parse", "--short", "@{u}").Output()
	return []checklist.UpdateItem{{
		Name:       "oh-my-zsh",
		Source:     "git",
		CurrentVer: strings.TrimSpace(string(currentSHA)),
		NewVer:     strings.TrimSpace(string(remoteSHA)) + " (" + behind + " commits)",
	}}, nil
}

func scanBun(ctx context.Context) ([]checklist.UpdateItem, error) {
	bunPath, err := exec.LookPath("bun")
	if err != nil {
		return nil, nil
	}
	_ = bunPath
	currentOut, err := exec.CommandContext(ctx, "bun", "--version").Output()
	if err != nil {
		return nil, nil
	}
	current := strings.TrimSpace(string(currentOut))

	// Check latest via bun's upgrade --dry-run (not supported), use GitHub API
	tag, err := sysutil.GitHubLatestVersion(ctx, "oven-sh/bun")
	if err != nil {
		return nil, fmt.Errorf("check latest bun: %w", err)
	}
	latest := strings.TrimPrefix(tag, "bun-v")
	latest = strings.TrimPrefix(latest, "v")
	if !versionNewer(latest, current) {
		return nil, nil
	}
	return []checklist.UpdateItem{{
		Name:       "bun",
		Source:     "runtime",
		CurrentVer: current,
		NewVer:     latest,
	}}, nil
}

func scanNodeEnv(ctx context.Context) ([]checklist.UpdateItem, error) {
	if _, err := exec.LookPath("nodenv"); err != nil {
		return nil, nil
	}
	// Get current version
	currentOut, err := exec.CommandContext(ctx, "nodenv", "version").Output()
	if err != nil {
		return nil, fmt.Errorf("nodenv version: %w", err)
	}
	fields := strings.Fields(strings.TrimSpace(string(currentOut)))
	if len(fields) == 0 {
		return nil, nil
	}
	current := fields[0]

	// fetchLatestNodeLTS returns "" when nodenv has no usable definition list,
	// which is a "can't determine", not a failure — stay silent in that case.
	latest := fetchLatestNodeLTS(ctx)
	if latest == "" || !versionNewer(latest, current) {
		return nil, nil
	}
	return []checklist.UpdateItem{{
		Name:       "node (nodenv)",
		Source:     "runtime",
		CurrentVer: current,
		NewVer:     latest,
	}}, nil
}

func scanNPMGlobals(ctx context.Context) ([]checklist.UpdateItem, error) {
	if _, err := exec.LookPath("npm"); err != nil {
		return nil, nil
	}
	// `npm outdated` exits non-zero precisely when there ARE outdated packages,
	// so a non-zero exit WITH output is the normal case. Only an error with no
	// output at all is a genuine failure worth surfacing.
	out, err := exec.CommandContext(ctx, "npm", "outdated", "-g", "--json").Output()
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("npm outdated -g: %w", err)
	}
	// npm outdated -g --json returns {} if nothing outdated, or {"pkg": {"current": "x", "wanted": "y", "latest": "z"}}
	// Parse manually to avoid encoding/json import overhead for simple case
	content := strings.TrimSpace(string(out))
	if content == "" || content == "{}" {
		return nil, nil
	}
	return parseNPMOutdatedJSON(content)
}

func parseNPMOutdatedJSON(content string) ([]checklist.UpdateItem, error) {
	var packages map[string]struct {
		Current string `json:"current"`
		Latest  string `json:"latest"`
	}
	if err := json.Unmarshal([]byte(content), &packages); err != nil {
		return nil, err
	}
	var items []checklist.UpdateItem
	for name, pkg := range packages {
		if pkg.Current != pkg.Latest {
			items = append(items, checklist.UpdateItem{
				Name:       name,
				Source:     "npm",
				CurrentVer: pkg.Current,
				NewVer:     pkg.Latest,
			})
		}
	}
	return items, nil
}

func scanCtdev(ctx context.Context) ([]checklist.UpdateItem, error) {
	if version == "" {
		return nil, nil
	}
	latest, err := sysutil.GitHubLatestVersion(ctx, "ConnerTechnology/dotfiles")
	if err != nil {
		return nil, fmt.Errorf("check latest ctdev: %w", err)
	}
	if latest == "" {
		return nil, nil
	}
	current := strings.TrimPrefix(version, "v")
	if !versionNewer(latest, current) {
		return nil, nil
	}
	return []checklist.UpdateItem{{
		Name:       "ctdev",
		Source:     "ctdev",
		CurrentVer: current,
		NewVer:     latest,
	}}, nil
}

func scanGo(ctx context.Context) ([]checklist.UpdateItem, error) {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return nil, nil
	}
	// Skip if Go is managed by apt (will be updated through apt scanner)
	if dpkgOut, err := exec.CommandContext(ctx, "dpkg", "-S", goPath).Output(); err == nil {
		if strings.Contains(string(dpkgOut), "golang") {
			return nil, nil
		}
	}

	out, err := exec.CommandContext(ctx, "go", "version").Output()
	if err != nil {
		return nil, fmt.Errorf("go version: %w", err)
	}
	// "go version go1.26.1 linux/amd64"
	fields := strings.Fields(string(out))
	if len(fields) < 3 {
		return nil, nil
	}
	current := strings.TrimPrefix(fields[2], "go")

	latest, err := fetchLatestGoVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("check latest go: %w", err)
	}
	if latest == "" || !versionNewer(latest, current) {
		return nil, nil
	}
	return []checklist.UpdateItem{{
		Name:       "go",
		Source:     "runtime",
		CurrentVer: current,
		NewVer:     latest,
	}}, nil
}

func scanRuby(ctx context.Context) ([]checklist.UpdateItem, error) {
	if _, err := exec.LookPath("ruby"); err != nil {
		return nil, nil
	}
	out, err := exec.CommandContext(ctx, "ruby", "--version").Output()
	if err != nil {
		return nil, fmt.Errorf("ruby --version: %w", err)
	}
	// "ruby 3.4.1 (2024-12-25 revision 48d4efcb85) +PRISM [x86_64-linux]"
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return nil, nil
	}
	current := fields[1]

	// fetchLatestRubyVersion returns "" when ruby-build isn't present, which is a
	// legitimate "can't determine", not a failure — stay silent in that case.
	latest := fetchLatestRubyVersion(ctx)
	if latest == "" || !versionNewer(latest, current) {
		return nil, nil
	}
	return []checklist.UpdateItem{{
		Name:       "ruby",
		Source:     "runtime",
		CurrentVer: current,
		NewVer:     latest,
	}}, nil
}

func scanHelm(ctx context.Context) ([]checklist.UpdateItem, error) {
	if _, err := exec.LookPath("helm"); err != nil {
		return nil, nil
	}
	out, err := exec.CommandContext(ctx, "helm", "version", "--short").Output()
	if err != nil {
		return nil, nil
	}
	// "v3.16.1+g5a5011d"
	current := strings.TrimSpace(string(out))
	if idx := strings.Index(current, "+"); idx > 0 {
		current = current[:idx]
	}
	current = strings.TrimPrefix(current, "v")

	currentMajor := majorVersion(current)
	tags, err := fetchGitHubReleaseTags(ctx, "helm/helm")
	if err != nil {
		return nil, fmt.Errorf("check latest helm: %w", err)
	}

	var items []checklist.UpdateItem
	latestSameMajor := ""
	latestNewMajor := ""

	for _, tag := range tags {
		ver := strings.TrimPrefix(tag, "v")
		maj := majorVersion(ver)
		if maj == currentMajor {
			if latestSameMajor == "" {
				latestSameMajor = ver
			}
		} else if maj > currentMajor {
			if latestNewMajor == "" {
				latestNewMajor = ver
			}
		}
		if latestSameMajor != "" && latestNewMajor != "" {
			break
		}
	}

	if versionNewer(latestSameMajor, current) {
		items = append(items, checklist.UpdateItem{
			Name:       "helm",
			Source:     "cli",
			CurrentVer: current,
			NewVer:     latestSameMajor,
		})
	}
	if latestNewMajor != "" {
		items = append(items, checklist.UpdateItem{
			Name:       "helm",
			Source:     "cli",
			CurrentVer: current,
			NewVer:     latestNewMajor,
			IsMajor:    true,
		})
	}

	return items, nil
}

func scanKubectl(ctx context.Context) ([]checklist.UpdateItem, error) {
	if _, err := exec.LookPath("kubectl"); err != nil {
		return nil, nil
	}
	out, err := exec.CommandContext(ctx, "kubectl", "version", "--client", "--output=json").Output()
	if err != nil {
		return nil, nil
	}
	var kubectlVer struct {
		ClientVersion struct {
			GitVersion string `json:"gitVersion"`
		} `json:"clientVersion"`
	}
	if err := json.Unmarshal(out, &kubectlVer); err != nil {
		return nil, nil
	}
	current := strings.TrimPrefix(kubectlVer.ClientVersion.GitVersion, "v")

	// Fetch latest stable kubectl version
	latest, err := fetchLatestKubectlVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("check latest kubectl: %w", err)
	}
	if latest == "" || !versionNewer(latest, current) {
		return nil, nil
	}
	return []checklist.UpdateItem{{
		Name:       "kubectl",
		Source:     "cli",
		CurrentVer: current,
		NewVer:     latest,
	}}, nil
}

func scanTerraform(ctx context.Context) ([]checklist.UpdateItem, error) {
	if _, err := exec.LookPath("terraform"); err != nil {
		return nil, nil
	}
	out, err := exec.CommandContext(ctx, "terraform", "version", "-json").Output()
	if err != nil {
		return nil, nil
	}
	var tfVer struct {
		TerraformVersion string `json:"terraform_version"`
	}
	if err := json.Unmarshal(out, &tfVer); err != nil {
		return nil, nil
	}
	current := tfVer.TerraformVersion
	if current == "" {
		return nil, nil
	}

	latest, err := sysutil.GitHubLatestVersion(ctx, "hashicorp/terraform")
	if err != nil {
		return nil, fmt.Errorf("check latest terraform: %w", err)
	}
	if latest == "" || !versionNewer(latest, current) {
		return nil, nil
	}
	return []checklist.UpdateItem{{
		Name:       "terraform",
		Source:     "cli",
		CurrentVer: current,
		NewVer:     latest,
	}}, nil
}

// fetchGitHubReleaseTags returns release tags from newest to oldest (excludes pre-releases).
func fetchGitHubReleaseTags(ctx context.Context, repo string) ([]string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=50", repo)
	resp, err := httpGetJSON(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var releases []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	var tags []string
	for _, r := range releases {
		if !r.Prerelease {
			tags = append(tags, r.TagName)
		}
	}
	return tags, nil
}

func majorVersion(ver string) int {
	parts := strings.SplitN(ver, ".", 2)
	if len(parts) == 0 {
		return 0
	}
	var major int
	fmt.Sscanf(parts[0], "%d", &major)
	return major
}

// versionParts splits a dotted version into its leading numeric components,
// ignoring a leading "v" and any pre-release/build suffix.
func versionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	fields := strings.FieldsFunc(v, func(r rune) bool {
		return r == '.' || r == '-' || r == '+' || r == '_'
	})
	var parts []int
	for _, f := range fields {
		num, got := 0, false
		for _, r := range f {
			if r < '0' || r > '9' {
				break
			}
			num, got = num*10+int(r-'0'), true
		}
		if !got {
			break
		}
		parts = append(parts, num)
	}
	return parts
}

// versionNewer reports whether candidate is a strictly higher dotted-numeric
// version than current. It's used to avoid offering a "downgrade" when the
// locally installed tool is newer than the registry's notion of latest (e.g. a
// manually built Go or a pre-release). Equal versions return false.
func versionNewer(candidate, current string) bool {
	c, cur := versionParts(candidate), versionParts(current)
	n := len(c)
	if len(cur) > n {
		n = len(cur)
	}
	for i := 0; i < n; i++ {
		a, b := 0, 0
		if i < len(c) {
			a = c[i]
		}
		if i < len(cur) {
			b = cur[i]
		}
		if a != b {
			return a > b
		}
	}
	return false
}

func fetchLatestNodeLTS(ctx context.Context) string {
	// Use a simpler endpoint that returns just the LTS schedule
	// The full index.json is huge, so we use nodenv's definitions if available
	if _, err := exec.LookPath("nodenv"); err == nil {
		out, err := exec.CommandContext(ctx, "nodenv", "install", "--list").Output()
		if err == nil {
			// Find latest even-numbered major version (LTS)
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			for i := len(lines) - 1; i >= 0; i-- {
				v := strings.TrimSpace(lines[i])
				if v == "" || strings.Contains(v, "-") {
					continue
				}
				parts := strings.SplitN(v, ".", 2)
				if len(parts) < 2 {
					continue
				}
				major := 0
				fmt.Sscanf(parts[0], "%d", &major)
				if major > 0 && major%2 == 0 {
					return v
				}
			}
		}
	}
	return ""
}

// goRelease mirrors the subset of https://go.dev/dl/?mode=json we consume.
type goRelease struct {
	Version string `json:"version"`
	Files   []struct {
		Filename string `json:"filename"`
		OS       string `json:"os"`
		Arch     string `json:"arch"`
		Kind     string `json:"kind"`
		SHA256   string `json:"sha256"`
	} `json:"files"`
}

// httpGetJSON issues a GET with ctx so Ctrl-C cancels in-flight requests.
// Callers are expected to close the response body.
func httpGetJSON(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return sysutil.HTTPClient().Do(req)
}

// fetchGoReleases returns the list of published Go releases (newest first).
// Each release includes per-OS/arch archive metadata with SHA256 hashes.
func fetchGoReleases(ctx context.Context) ([]goRelease, error) {
	resp, err := httpGetJSON(ctx, "https://go.dev/dl/?mode=json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var releases []goRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	return releases, nil
}

func fetchLatestGoVersion(ctx context.Context) (string, error) {
	releases, err := fetchGoReleases(ctx)
	if err != nil {
		return "", err
	}
	if len(releases) == 0 {
		return "", nil
	}
	return strings.TrimPrefix(releases[0].Version, "go"), nil
}

// goArchiveSHA256 looks up the published sha256 for the archive tarball
// matching ver/goos/goarch. Returns "" if not found.
func goArchiveSHA256(releases []goRelease, ver, goos, goarch string) string {
	target := fmt.Sprintf("go%s.%s-%s.tar.gz", ver, goos, goarch)
	for _, r := range releases {
		if strings.TrimPrefix(r.Version, "go") != ver {
			continue
		}
		for _, f := range r.Files {
			if f.Filename == target {
				return f.SHA256
			}
		}
	}
	return ""
}

func fetchLatestRubyVersion(ctx context.Context) string {
	// Use ruby-build's definitions if available (from rbenv)
	if _, err := exec.LookPath("ruby-build"); err == nil {
		out, err := exec.CommandContext(ctx, "ruby-build", "--definitions").Output()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			// Walk backwards to find latest stable (numeric only, no -dev/-preview/-rc)
			for i := len(lines) - 1; i >= 0; i-- {
				v := strings.TrimSpace(lines[i])
				if v == "" {
					continue
				}
				// Skip non-release versions
				if strings.ContainsAny(v, "-") {
					continue
				}
				// Must start with a digit
				if v[0] >= '0' && v[0] <= '9' {
					return v
				}
			}
		}
	}
	return ""
}

func fetchLatestKubectlVersion(ctx context.Context) (string, error) {
	resp, err := httpGetJSON(ctx, "https://dl.k8s.io/release/stable.txt")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(strings.TrimSpace(string(body)), "v"), nil
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

func executeUpdates(ctx context.Context, allItems, items []checklist.UpdateItem) error {
	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}

	// Group items by source
	bySource := make(map[string][]checklist.UpdateItem)
	for _, item := range items {
		bySource[item.Source] = append(bySource[item.Source], item)
	}
	allBySource := make(map[string][]checklist.UpdateItem)
	for _, item := range allItems {
		allBySource[item.Source] = append(allBySource[item.Source], item)
	}

	// APT
	if pkgs := bySource["apt"]; len(pkgs) > 0 {
		names := itemNames(pkgs)
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Updating %d apt packages...", len(names))))
		args := append([]string{"apt", "install", "--only-upgrade", "-y"}, names...)
		if err := sysutil.SudoRun(ctx, o, args[0], args[1:]...); err != nil {
			// Warn and continue like every other source, so one failing manager
			// doesn't strand the brew/flatpak/runtime/cli/npm updates the user
			// also selected.
			fmt.Printf("  apt upgrade warning: %v\n", err)
		}
	}

	// Mint Update Manager (kernel and other mintupdate-managed packages)
	if pkgs := bySource["mintupdate"]; len(pkgs) > 0 {
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Updating %d mintupdate packages...", len(pkgs))))
		selectedNames := make(map[string]bool)
		for _, p := range pkgs {
			selectedNames[p.Name] = true
		}
		var ignoreNames []string
		for _, p := range allBySource["mintupdate"] {
			if !selectedNames[p.Name] {
				ignoreNames = append(ignoreNames, p.Name)
			}
		}
		args := []string{"mintupdate-cli", "upgrade", "-r", "-y"}
		if len(ignoreNames) > 0 {
			args = append(args, "-i", strings.Join(ignoreNames, ","))
		}
		if err := sysutil.SudoRun(ctx, o, args[0], args[1:]...); err != nil {
			fmt.Printf("  mintupdate upgrade warning: %v\n", err)
		}
	}

	// Flatpak
	for _, item := range bySource["flatpak"] {
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Updating flatpak: %s...", item.Name)))
		if err := sysutil.Run(ctx, o, "flatpak", "update", "-y", item.Name); err != nil {
			fmt.Printf("  flatpak update warning (%s): %v\n", item.Name, err)
		}
	}

	// Brew
	if pkgs := bySource["brew"]; len(pkgs) > 0 {
		names := itemNames(pkgs)
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Updating %d brew packages...", len(names))))
		args := append([]string{"upgrade"}, names...)
		if err := sysutil.Run(ctx, o, "brew", args...); err != nil {
			fmt.Printf("  brew upgrade warning: %v\n", err)
		}
	}

	// Brew Cask
	if pkgs := bySource["brew-cask"]; len(pkgs) > 0 {
		names := itemNames(pkgs)
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Updating %d brew cask apps...", len(names))))
		args := append([]string{"upgrade", "--cask"}, names...)
		if err := sysutil.Run(ctx, o, "brew", args...); err != nil {
			fmt.Printf("  brew cask upgrade warning: %v\n", err)
		}
	}

	// Oh My Zsh
	for range bySource["git"] {
		fmt.Println(styles.Dimmed.Render("Updating Oh My Zsh..."))
		omzDir := os.ExpandEnv("$HOME/.oh-my-zsh")
		if err := sysutil.Run(ctx, o, "git", "-C", omzDir, "pull", "--rebase", "--quiet"); err != nil {
			fmt.Printf("  oh-my-zsh update warning: %v\n", err)
		}
		break // only one oh-my-zsh
	}

	// Runtimes (bun / node / go / ruby)
	for _, item := range bySource["runtime"] {
		switch item.Name {
		case "bun":
			fmt.Println(styles.Dimmed.Render("Updating bun..."))
			if err := sysutil.Run(ctx, o, "bun", "upgrade"); err != nil {
				fmt.Printf("  bun upgrade warning: %v\n", err)
			}
		case "node (nodenv)":
			fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Updating node to %s via nodenv...", item.NewVer)))
			if err := sysutil.Run(ctx, o, "nodenv", "install", "--skip-existing", item.NewVer); err != nil {
				fmt.Printf("  nodenv install warning: %v\n", err)
				continue
			}
			if err := sysutil.Run(ctx, o, "nodenv", "global", item.NewVer); err != nil {
				fmt.Printf("  nodenv global warning: %v\n", err)
			}
		case "go":
			fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Updating Go %s → %s...", item.CurrentVer, item.NewVer)))
			if o.DryRun {
				fmt.Fprintf(o.Stdout, "  [dry-run] download and install go%s\n", item.NewVer)
			} else if err := updateGo(ctx, o, item.NewVer); err != nil {
				fmt.Printf("  go update warning: %v\n", err)
			}
		case "ruby":
			fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Ruby %s available (current: %s)", item.NewVer, item.CurrentVer)))
			if _, err := exec.LookPath("rbenv"); err != nil {
				fmt.Println(styles.Help.Render("  Install manually or via rbenv"))
				continue
			}
			if err := sysutil.Run(ctx, o, "rbenv", "install", "--skip-existing", item.NewVer); err != nil {
				fmt.Printf("  rbenv install warning: %v\n", err)
				continue
			}
			if err := sysutil.Run(ctx, o, "rbenv", "global", item.NewVer); err != nil {
				fmt.Printf("  rbenv global warning: %v\n", err)
			}
		}
	}

	// NPM globals
	if pkgs := bySource["npm"]; len(pkgs) > 0 {
		names := itemNames(pkgs)
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Updating %d npm global packages...", len(names))))
		args := append([]string{"update", "-g"}, names...)
		if err := sysutil.Run(ctx, o, "npm", args...); err != nil {
			fmt.Printf("  npm update -g warning: %v\n", err)
		}
	}

	// ctdev self-update
	for _, item := range bySource["ctdev"] {
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Updating ctdev %s → %s...", item.CurrentVer, item.NewVer)))
		if err := sysutil.Run(ctx, o, "bash", "-c",
			"curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/install.sh | bash"); err != nil {
			fmt.Printf("  ctdev update warning: %v\n", err)
		}
	}

	// CLI tools (helm, kubectl, terraform)
	for _, item := range bySource["cli"] {
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Updating %s %s → %s...", item.Name, item.CurrentVer, item.NewVer)))
		switch item.Name {
		case "helm":
			if o.DryRun {
				fmt.Fprintf(o.Stdout, "  [dry-run] download helm v%s and install to /usr/local/bin\n", item.NewVer)
			} else if err := updateHelm(ctx, o, item.NewVer); err != nil {
				fmt.Printf("  helm update warning: %v\n", err)
			}
		case "kubectl":
			if o.DryRun {
				fmt.Fprintf(o.Stdout, "  [dry-run] download kubectl v%s and install to /usr/local/bin\n", item.NewVer)
			} else if err := updateKubectl(ctx, o, item.NewVer); err != nil {
				fmt.Printf("  kubectl update warning: %v\n", err)
			}
		case "terraform":
			if o.DryRun {
				fmt.Fprintf(o.Stdout, "  [dry-run] download terraform %s and install\n", item.NewVer)
			} else if err := updateTerraform(ctx, o, item.NewVer); err != nil {
				fmt.Printf("  terraform update warning: %v\n", err)
			}
		}
	}

	fmt.Println(styles.Success.Render("Updates complete."))
	return nil
}

// resolveInstallDest returns the existing install path for bin (from `which`)
// or a sensible default under /usr/local/bin.
func resolveInstallDest(ctx context.Context, bin, fallback string) string {
	out, _ := exec.CommandContext(ctx, "which", bin).Output()
	dest := strings.TrimSpace(string(out))
	if dest == "" {
		return fallback
	}
	return dest
}

// downloadVerifiedArchive downloads archiveURL into a fresh temp dir, fetches
// checksumURL, verifies the archive against the hash that pickHash extracts
// from the checksum file, then calls install with the temp dir and archive
// path. The temp dir is always cleaned up. Each caller keeps its own checksum
// format (bare hash, "hash file", or SHA256SUMS) and extraction step, so this
// only collapses the shared download/verify boilerplate.
func downloadVerifiedArchive(ctx context.Context, archiveURL, checksumURL string, pickHash func(checksumContents string) (string, error), install func(tmpDir, archivePath string) error) error {
	tmpDir, err := os.MkdirTemp("", "ctdev-dl-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, filepath.Base(archiveURL))
	if err := sysutil.DownloadFile(ctx, archiveURL, archivePath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	csPath := filepath.Join(tmpDir, "checksum")
	if err := sysutil.DownloadFile(ctx, checksumURL, csPath); err != nil {
		return fmt.Errorf("download checksum failed: %w", err)
	}
	csData, err := os.ReadFile(csPath)
	if err != nil {
		return fmt.Errorf("read checksum: %w", err)
	}
	expected, err := pickHash(string(csData))
	if err != nil {
		return err
	}
	if err := sysutil.VerifyChecksum(archivePath, expected); err != nil {
		return fmt.Errorf("checksum mismatch: %w", err)
	}
	return install(tmpDir, archivePath)
}

func updateHelm(ctx context.Context, o sysutil.Opts, ver string) error {
	goos := detectUpdateOS()
	goarch := detectUpdateArch()
	url := fmt.Sprintf("https://get.helm.sh/helm-v%s-%s-%s.tar.gz", ver, goos, goarch)
	return downloadVerifiedArchive(ctx, url, url+".sha256sum",
		func(cs string) (string, error) {
			// helm publishes the bare hash (optionally "hash  filename").
			parts := strings.Fields(strings.TrimSpace(cs))
			if len(parts) == 0 {
				return "", fmt.Errorf("empty checksum file")
			}
			return parts[0], nil
		},
		func(tmpDir, archivePath string) error {
			if err := sysutil.Run(ctx, o, "tar", "-xzf", archivePath, "-C", tmpDir); err != nil {
				return fmt.Errorf("extract failed: %w", err)
			}
			dest := resolveInstallDest(ctx, "helm", "/usr/local/bin/helm")
			return sysutil.SudoRun(ctx, o, "mv", fmt.Sprintf("%s/%s-%s/helm", tmpDir, goos, goarch), dest)
		})
}

func updateKubectl(ctx context.Context, o sysutil.Opts, ver string) error {
	goos := detectUpdateOS()
	goarch := detectUpdateArch()
	url := fmt.Sprintf("https://dl.k8s.io/release/v%s/bin/%s/%s/kubectl", ver, goos, goarch)
	return downloadVerifiedArchive(ctx, url, url+".sha256",
		func(cs string) (string, error) { return strings.TrimSpace(cs), nil },
		func(tmpDir, archivePath string) error {
			if err := os.Chmod(archivePath, 0755); err != nil {
				return err
			}
			dest := resolveInstallDest(ctx, "kubectl", "/usr/local/bin/kubectl")
			return sysutil.SudoRun(ctx, o, "mv", archivePath, dest)
		})
}

func updateTerraform(ctx context.Context, o sysutil.Opts, ver string) error {
	goos := detectUpdateOS()
	goarch := detectUpdateArch()
	zipName := fmt.Sprintf("terraform_%s_%s_%s.zip", ver, goos, goarch)
	url := fmt.Sprintf("https://releases.hashicorp.com/terraform/%s/%s", ver, zipName)
	csURL := fmt.Sprintf("https://releases.hashicorp.com/terraform/%s/terraform_%s_SHA256SUMS", ver, ver)
	return downloadVerifiedArchive(ctx, url, csURL,
		func(cs string) (string, error) {
			// SHA256SUMS has lines like "hash  terraform_1.2.3_linux_amd64.zip".
			for _, line := range strings.Split(cs, "\n") {
				if strings.Contains(line, zipName) {
					if parts := strings.Fields(line); len(parts) >= 1 {
						return parts[0], nil
					}
				}
			}
			return "", fmt.Errorf("checksum not found for %s", zipName)
		},
		func(tmpDir, archivePath string) error {
			if err := sysutil.Run(ctx, o, "unzip", "-o", archivePath, "-d", tmpDir); err != nil {
				return fmt.Errorf("unzip failed: %w", err)
			}
			dest := resolveInstallDest(ctx, "terraform", "/usr/local/bin/terraform")
			return sysutil.SudoRun(ctx, o, "mv", filepath.Join(tmpDir, "terraform"), dest)
		})
}

func updateGo(ctx context.Context, o sysutil.Opts, ver string) error {
	goos := detectUpdateOS()
	goarch := detectUpdateArch()
	url := fmt.Sprintf("https://go.dev/dl/go%s.%s-%s.tar.gz", ver, goos, goarch)

	// Fetch the release manifest first so we can verify the tarball checksum
	// before touching the existing /usr/local/go install.
	releases, err := fetchGoReleases(ctx)
	if err != nil {
		return fmt.Errorf("fetch go release manifest: %w", err)
	}
	expectedSHA := goArchiveSHA256(releases, ver, goos, goarch)
	if expectedSHA == "" {
		return fmt.Errorf("no published sha256 for go%s %s/%s", ver, goos, goarch)
	}

	tmpFile, err := os.CreateTemp("", "ctdev-go-*.tar.gz")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)
	if err := sysutil.DownloadFile(ctx, url, tmpPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	if err := sysutil.VerifyChecksum(tmpPath, expectedSHA); err != nil {
		return fmt.Errorf("go checksum mismatch: %w", err)
	}
	// Extract to a temp directory first so we don't destroy the existing install
	// if the archive is corrupted or extraction fails.
	tmpDir, err := os.MkdirTemp("", "ctdev-go-extract-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	if err := sysutil.Run(ctx, o, "tar", "-C", tmpDir, "-xzf", tmpPath); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}
	// Verify the extraction produced a go directory with a binary
	if _, err := os.Stat(filepath.Join(tmpDir, "go", "bin", "go")); err != nil {
		return fmt.Errorf("extracted archive missing go binary: %w", err)
	}
	// Safe to replace now
	if err := sysutil.SudoRun(ctx, o, "rm", "-rf", "/usr/local/go"); err != nil {
		return fmt.Errorf("remove old go failed: %w", err)
	}
	if err := sysutil.SudoRun(ctx, o, "mv", filepath.Join(tmpDir, "go"), "/usr/local/go"); err != nil {
		return fmt.Errorf("install new go failed: %w", err)
	}
	return nil
}

func detectUpdateOS() string {
	return runtime.GOOS
}

func detectUpdateArch() string {
	return runtime.GOARCH
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
