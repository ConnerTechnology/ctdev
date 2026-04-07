package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
		fmt.Println(styles.Dimmed.Render("Refreshing APT GPG keys..."))
		refreshAPTKeys(args)
	}

	items := scanAll(context.Background())

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

	return executeUpdates(items, selected)
}

func scanAll(ctx context.Context) []checklist.UpdateItem {
	var mu sync.Mutex
	var allItems []checklist.UpdateItem

	type scanner func(context.Context) ([]checklist.UpdateItem, error)
	scanners := []scanner{
		scanAPT,
		scanMintUpdate,
		scanFlatpak,
		scanBrew,
		scanBrewCask,
		scanOhMyZsh,
		scanBun,
		scanNodeEnv,
		scanNPMGlobals,
		scanCtdev,
		scanGo,
		scanRuby,
		scanHelm,
		scanKubectl,
		scanTerraform,
	}

	total := len(scanners)
	done := make(chan struct{}, total)

	go func() {
		count := 0
		for range done {
			count++
			fmt.Printf("\r  %s", styles.Dimmed.Render(fmt.Sprintf("Scanning for updates... (%d/%d sources checked)", count, total)))
		}
		fmt.Println()
	}()

	var wg sync.WaitGroup
	for _, fn := range scanners {
		fn := fn
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { done <- struct{}{} }()
			items, err := fn(ctx)
			if err == nil && len(items) > 0 {
				mu.Lock()
				allItems = append(allItems, items...)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	close(done)

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
	"1password":                      "1password",
	"chatgpt":                        "chatgpt",
	"cleanmymac":                     "cleanmymac",
	"claude":                         "claude-desktop",
	"dbeaver-community":              "dbeaver",
	"docker":                         "docker",
	"ghostty":                        "ghostty",
	"google-chrome":                  "chrome",
	"linear-linear":                  "linear",
	"logi-options+":                  "logi-options",
	"slack":                          "slack",
	"tailscale":                      "tailscale",
	"visual-studio-code":             "vscode",
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
		return nil, nil
	}
	// Check if behind
	out, err := exec.CommandContext(ctx, "git", "-C", omzDir, "rev-list", "--count", "HEAD..@{u}").Output()
	if err != nil {
		return nil, nil
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
	tag, err := sysutil.GitHubLatestVersion("oven-sh/bun")
	if err != nil {
		return nil, nil
	}
	latest := strings.TrimPrefix(tag, "bun-v")
	latest = strings.TrimPrefix(latest, "v")
	if latest == current {
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
		return nil, nil
	}
	current := strings.Fields(strings.TrimSpace(string(currentOut)))[0]

	// Get latest available LTS
	latest := fetchLatestNodeLTS(ctx)
	if latest == "" || latest == current {
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
	out, err := exec.CommandContext(ctx, "npm", "outdated", "-g", "--json").Output()
	if err != nil && len(out) == 0 {
		return nil, nil
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
	latest, err := sysutil.GitHubLatestVersion("ConnerTechnology/dotfiles")
	if err != nil || latest == "" {
		return nil, nil
	}
	current := strings.TrimPrefix(version, "v")
	if latest == current {
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
		return nil, nil
	}
	// "go version go1.26.1 linux/amd64"
	fields := strings.Fields(string(out))
	if len(fields) < 3 {
		return nil, nil
	}
	current := strings.TrimPrefix(fields[2], "go")

	latest := fetchLatestGoVersion(ctx)
	if latest == "" || latest == current {
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
		return nil, nil
	}
	// "ruby 3.4.1 (2024-12-25 revision 48d4efcb85) +PRISM [x86_64-linux]"
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return nil, nil
	}
	current := fields[1]

	latest := fetchLatestRubyVersion(ctx)
	if latest == "" || latest == current {
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
	tags := fetchGitHubReleaseTags("helm/helm")

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

	if latestSameMajor != "" && latestSameMajor != current {
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
	latest := fetchLatestKubectlVersion(ctx)
	if latest == "" || latest == current {
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

	latest, err := sysutil.GitHubLatestVersion("hashicorp/terraform")
	if err != nil || latest == "" {
		return nil, nil
	}
	if latest == current {
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
func fetchGitHubReleaseTags(repo string) []string {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=50", repo)
	resp, err := sysutil.HTTPClient().Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var releases []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil
	}
	var tags []string
	for _, r := range releases {
		if !r.Prerelease {
			tags = append(tags, r.TagName)
		}
	}
	return tags
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

func fetchLatestGoVersion(ctx context.Context) string {
	resp, err := sysutil.HTTPClient().Get("https://go.dev/dl/?mode=json")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var releases []struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil || len(releases) == 0 {
		return ""
	}
	return strings.TrimPrefix(releases[0].Version, "go")
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

func fetchLatestKubectlVersion(ctx context.Context) string {
	resp, err := sysutil.HTTPClient().Get("https://dl.k8s.io/release/stable.txt")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(string(body)), "v")
}

func printUpdateList(items []checklist.UpdateItem) {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Orange)
	labelStyle := lipgloss.NewStyle().Foreground(styles.Subtle).Width(30)
	valueStyle := lipgloss.NewStyle().Foreground(styles.Bright)

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

func executeUpdates(allItems, items []checklist.UpdateItem) error {
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
		if flagDryRun {
			fmt.Printf("  [dry-run] sudo apt install --only-upgrade -y %s\n", strings.Join(names, " "))
		} else {
			args := append([]string{"apt", "install", "--only-upgrade", "-y"}, names...)
			cmd := exec.Command("sudo", args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("apt upgrade failed: %w", err)
			}
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
		if flagDryRun {
			fmt.Printf("  [dry-run] sudo %s\n", strings.Join(args, " "))
		} else {
			cmd := exec.Command("sudo", args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Printf("  mintupdate upgrade warning: %v\n", err)
			}
		}
	}

	// Flatpak
	for _, item := range bySource["flatpak"] {
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Updating flatpak: %s...", item.Name)))
		if flagDryRun {
			fmt.Printf("  [dry-run] flatpak update -y %s\n", item.Name)
		} else {
			cmd := exec.Command("flatpak", "update", "-y", item.Name)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Printf("  flatpak update warning (%s): %v\n", item.Name, err)
			}
		}
	}

	// Brew
	if pkgs := bySource["brew"]; len(pkgs) > 0 {
		names := itemNames(pkgs)
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Updating %d brew packages...", len(names))))
		if flagDryRun {
			fmt.Printf("  [dry-run] brew upgrade %s\n", strings.Join(names, " "))
		} else {
			args := append([]string{"upgrade"}, names...)
			cmd := exec.Command("brew", args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Printf("  brew upgrade warning: %v\n", err)
			}
		}
	}

	// Brew Cask
	if pkgs := bySource["brew-cask"]; len(pkgs) > 0 {
		names := itemNames(pkgs)
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Updating %d brew cask apps...", len(names))))
		if flagDryRun {
			fmt.Printf("  [dry-run] brew upgrade --cask %s\n", strings.Join(names, " "))
		} else {
			args := append([]string{"upgrade", "--cask"}, names...)
			cmd := exec.Command("brew", args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Printf("  brew cask upgrade warning: %v\n", err)
			}
		}
	}

	// Oh My Zsh
	for range bySource["git"] {
		fmt.Println(styles.Dimmed.Render("Updating Oh My Zsh..."))
		omzDir := os.ExpandEnv("$HOME/.oh-my-zsh")
		if flagDryRun {
			fmt.Printf("  [dry-run] git -C %s pull --rebase\n", omzDir)
		} else {
			cmd := exec.Command("git", "-C", omzDir, "pull", "--rebase", "--quiet")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Printf("  oh-my-zsh update warning: %v\n", err)
			}
		}
		break // only one oh-my-zsh
	}

	// Bun
	for _, item := range bySource["runtime"] {
		switch item.Name {
		case "bun":
			fmt.Println(styles.Dimmed.Render("Updating bun..."))
			if flagDryRun {
				fmt.Println("  [dry-run] bun upgrade")
			} else {
				cmd := exec.Command("bun", "upgrade")
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					fmt.Printf("  bun upgrade warning: %v\n", err)
				}
			}
		case "node (nodenv)":
			fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Updating node to %s via nodenv...", item.NewVer)))
			if flagDryRun {
				fmt.Printf("  [dry-run] nodenv install %s && nodenv global %s\n", item.NewVer, item.NewVer)
			} else {
				cmd := exec.Command("nodenv", "install", item.NewVer)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					fmt.Printf("  nodenv install warning: %v\n", err)
				} else {
					cmd = exec.Command("nodenv", "global", item.NewVer)
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					if err := cmd.Run(); err != nil {
						fmt.Printf("  nodenv global warning: %v\n", err)
					}
				}
			}
		case "go":
			fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Updating Go %s → %s...", item.CurrentVer, item.NewVer)))
			if flagDryRun {
				fmt.Printf("  [dry-run] download and install go%s\n", item.NewVer)
			} else {
				if err := updateGo(item.NewVer); err != nil {
					fmt.Printf("  go update warning: %v\n", err)
				}
			}
		case "ruby":
			fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Ruby %s available (current: %s)", item.NewVer, item.CurrentVer)))
			if _, err := exec.LookPath("rbenv"); err == nil {
				if flagDryRun {
					fmt.Printf("  [dry-run] rbenv install %s && rbenv global %s\n", item.NewVer, item.NewVer)
				} else {
					cmd := exec.Command("rbenv", "install", item.NewVer)
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					if err := cmd.Run(); err != nil {
						fmt.Printf("  rbenv install warning: %v\n", err)
					} else {
						cmd = exec.Command("rbenv", "global", item.NewVer)
						cmd.Stdout = os.Stdout
						cmd.Stderr = os.Stderr
						if err := cmd.Run(); err != nil {
							fmt.Printf("  rbenv global warning: %v\n", err)
						}
					}
				}
			} else {
				fmt.Println(styles.Help.Render("  Install manually or via rbenv"))
			}
		}
	}

	// NPM globals
	if pkgs := bySource["npm"]; len(pkgs) > 0 {
		names := itemNames(pkgs)
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Updating %d npm global packages...", len(names))))
		if flagDryRun {
			fmt.Printf("  [dry-run] npm update -g %s\n", strings.Join(names, " "))
		} else {
			args := append([]string{"update", "-g"}, names...)
			cmd := exec.Command("npm", args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Printf("  npm update -g warning: %v\n", err)
			}
		}
	}

	// ctdev self-update
	for _, item := range bySource["ctdev"] {
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Updating ctdev %s → %s...", item.CurrentVer, item.NewVer)))
		if flagDryRun {
			fmt.Println("  [dry-run] curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/install.sh | bash")
		} else {
			cmd := exec.Command("bash", "-c", "curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/install.sh | bash")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Printf("  ctdev update warning: %v\n", err)
			}
		}
	}

	// CLI tools (helm, kubectl, terraform)
	for _, item := range bySource["cli"] {
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Updating %s %s → %s...", item.Name, item.CurrentVer, item.NewVer)))
		switch item.Name {
		case "helm":
			if flagDryRun {
				fmt.Printf("  [dry-run] download helm v%s and install to /usr/local/bin\n", item.NewVer)
			} else {
				if err := updateHelm(item.NewVer); err != nil {
					fmt.Printf("  helm update warning: %v\n", err)
				}
			}
		case "kubectl":
			if flagDryRun {
				fmt.Printf("  [dry-run] download kubectl v%s and install to /usr/local/bin\n", item.NewVer)
			} else {
				if err := updateKubectl(item.NewVer); err != nil {
					fmt.Printf("  kubectl update warning: %v\n", err)
				}
			}
		case "terraform":
			if flagDryRun {
				fmt.Printf("  [dry-run] download terraform %s and install\n", item.NewVer)
			} else {
				if err := updateTerraform(item.NewVer); err != nil {
					fmt.Printf("  terraform update warning: %v\n", err)
				}
			}
		}
	}

	fmt.Println(styles.Success.Render("Updates complete."))
	return nil
}

func updateHelm(ver string) error {
	goos := detectUpdateOS()
	goarch := detectUpdateArch()
	archive := fmt.Sprintf("helm-v%s-%s-%s.tar.gz", ver, goos, goarch)
	url := fmt.Sprintf("https://get.helm.sh/%s", archive)

	tmpDir, err := os.MkdirTemp("", "ctdev-helm-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	tmpFile := tmpDir + "/" + archive

	dl := exec.Command("curl", "-fsSL", "-o", tmpFile, url)
	dl.Stdout = os.Stdout
	dl.Stderr = os.Stderr
	if err := dl.Run(); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	tar := exec.Command("tar", "-xzf", tmpFile, "-C", tmpDir)
	tar.Stdout = os.Stdout
	tar.Stderr = os.Stderr
	if err := tar.Run(); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}
	installPath, _ := exec.Command("which", "helm").Output()
	dest := strings.TrimSpace(string(installPath))
	if dest == "" {
		dest = "/usr/local/bin/helm"
	}
	mv := exec.Command("sudo", "mv", fmt.Sprintf("%s/%s-%s/helm", tmpDir, goos, goarch), dest)
	mv.Stdout = os.Stdout
	mv.Stderr = os.Stderr
	return mv.Run()
}

func updateKubectl(ver string) error {
	goos := detectUpdateOS()
	goarch := detectUpdateArch()
	url := fmt.Sprintf("https://dl.k8s.io/release/v%s/bin/%s/%s/kubectl", ver, goos, goarch)

	tmpFile, err := os.CreateTemp("", "ctdev-kubectl-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)
	dl := exec.Command("curl", "-fsSL", "-o", tmpPath, url)
	dl.Stdout = os.Stdout
	dl.Stderr = os.Stderr
	if err := dl.Run(); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	if err := exec.Command("chmod", "+x", tmpPath).Run(); err != nil {
		return err
	}
	installPath, _ := exec.Command("which", "kubectl").Output()
	dest := strings.TrimSpace(string(installPath))
	if dest == "" {
		dest = "/usr/local/bin/kubectl"
	}
	mv := exec.Command("sudo", "mv", tmpPath, dest)
	mv.Stdout = os.Stdout
	mv.Stderr = os.Stderr
	return mv.Run()
}

func updateTerraform(ver string) error {
	goos := detectUpdateOS()
	goarch := detectUpdateArch()
	url := fmt.Sprintf("https://releases.hashicorp.com/terraform/%s/terraform_%s_%s_%s.zip", ver, ver, goos, goarch)

	tmpDir, err := os.MkdirTemp("", "ctdev-terraform-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	tmpZip := tmpDir + "/terraform.zip"

	dl := exec.Command("curl", "-fsSL", "-o", tmpZip, url)
	dl.Stdout = os.Stdout
	dl.Stderr = os.Stderr
	if err := dl.Run(); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	unzip := exec.Command("unzip", "-o", tmpZip, "-d", tmpDir)
	unzip.Stdout = os.Stdout
	unzip.Stderr = os.Stderr
	if err := unzip.Run(); err != nil {
		return fmt.Errorf("unzip failed: %w", err)
	}
	installPath, _ := exec.Command("which", "terraform").Output()
	dest := strings.TrimSpace(string(installPath))
	if dest == "" {
		dest = "/usr/local/bin/terraform"
	}
	mv := exec.Command("sudo", "mv", tmpDir+"/terraform", dest)
	mv.Stdout = os.Stdout
	mv.Stderr = os.Stderr
	return mv.Run()
}

func updateGo(ver string) error {
	goos := detectUpdateOS()
	goarch := detectUpdateArch()
	url := fmt.Sprintf("https://go.dev/dl/go%s.%s-%s.tar.gz", ver, goos, goarch)

	tmpFile, err := os.CreateTemp("", "ctdev-go-*.tar.gz")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)
	dl := exec.Command("curl", "-fsSL", "-o", tmpPath, url)
	dl.Stdout = os.Stdout
	dl.Stderr = os.Stderr
	if err := dl.Run(); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	// Remove old and extract new
	rm := exec.Command("sudo", "rm", "-rf", "/usr/local/go")
	rm.Stdout = os.Stdout
	rm.Stderr = os.Stderr
	if err := rm.Run(); err != nil {
		return fmt.Errorf("remove old go failed: %w", err)
	}
	tar := exec.Command("sudo", "tar", "-C", "/usr/local", "-xzf", tmpPath)
	tar.Stdout = os.Stdout
	tar.Stderr = os.Stderr
	if err := tar.Run(); err != nil {
		return fmt.Errorf("extract failed: %w", err)
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
	"gh":        {KeyURL: "https://cli.github.com/packages/githubcli-archive-keyring.gpg", KeyringPath: "/usr/share/keyrings/githubcli-archive-keyring.gpg"},
	"vscode":    {KeyURL: "https://packages.microsoft.com/keys/microsoft.asc", KeyringPath: "/usr/share/keyrings/packages.microsoft.gpg"},
	"1password": {KeyURL: "https://downloads.1password.com/linux/keys/1password.asc", KeyringPath: "/usr/share/keyrings/1password-archive-keyring.gpg"},
	"terraform": {KeyURL: "https://apt.releases.hashicorp.com/gpg", KeyringPath: "/usr/share/keyrings/hashicorp-archive-keyring.gpg"},
	"tailscale": {KeyURL: "https://pkgs.tailscale.com/stable/ubuntu/noble.noarmor.gpg", KeyringPath: "/usr/share/keyrings/tailscale-archive-keyring.gpg"},
}

func refreshAPTKeys(components []string) {
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
		if err := sysutil.AddAPTKeyring(o, r.KeyURL, r.KeyringPath); err != nil {
			fmt.Printf("  %s\n", styles.Warning.Render(fmt.Sprintf("Warning: %s key refresh failed: %v", name, err)))
		}
	}
}
