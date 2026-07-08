// Update scanners: scanAll and the per-source scanners (apt, brew, flatpak,
// runtimes, CLIs, ...) plus the output-parsing helpers they use.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/checklist"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
)

func scanAll(ctx context.Context) []checklist.UpdateItem {
	var mu sync.Mutex
	var allItems []checklist.UpdateItem

	type namedScanner struct {
		name string
		fn   func(context.Context) ([]checklist.UpdateItem, error)
	}
	scanners := []namedScanner{
		{"apt", scanAPT},
		{"flatpak", scanFlatpak},
		{"brew", scanBrew},
		{"brew-cask", scanBrewCask},
		{"oh-my-zsh", scanOhMyZsh},
		{"bun", scanBun},
		{"nodenv", scanNodeEnv},
		{"npm", scanNPMGlobals},
		{"ctdev", scanCtdev},
		{"docker", scanDocker},
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
		"apt": 0, "brew": 2, "brew-cask": 3, "flatpak": 4,
		"git": 5, "runtime": 6, "npm": 7,
		"cli": 8, "ctdev": 9, "docker": 10,
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
	// Compare against the global version — that's what the update step sets.
	// `nodenv version` resolves the cwd's .node-version, so a project pin would
	// make an already-applied update reappear forever.
	currentOut, err := exec.CommandContext(ctx, "nodenv", "global").Output()
	if err != nil {
		return nil, fmt.Errorf("nodenv global: %w", err)
	}
	current := strings.TrimSpace(string(currentOut))
	if current == "" || current[0] < '0' || current[0] > '9' {
		// "system" or unset — not nodenv-managed, nothing for us to update
		return nil, nil
	}

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
	// "dev" (a from-source build) parses as 0.0.0 and would flag an update on
	// every run.
	if version == "" || version == "dev" {
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

	// GOTOOLCHAIN=local: inside a module with a newer `toolchain` directive,
	// `go version` would report the auto-downloaded toolchain instead of the
	// installed one, hiding a real update.
	cmd := exec.CommandContext(ctx, "go", "version")
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	out, err := cmd.Output()
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
	current := ""
	// Prefer rbenv's global version — that's what the update step sets. Going
	// through `ruby --version` hits the rbenv shim, which resolves the cwd's
	// .ruby-version, so a project pin would make an already-applied update
	// reappear forever.
	if _, err := exec.LookPath("rbenv"); err == nil {
		out, err := exec.CommandContext(ctx, "rbenv", "global").Output()
		if err != nil {
			return nil, fmt.Errorf("rbenv global: %w", err)
		}
		v := strings.TrimSpace(string(out))
		if v != "" && v[0] >= '0' && v[0] <= '9' {
			current = v
		}
	}
	if current == "" {
		out, err := exec.CommandContext(ctx, "ruby", "--version").Output()
		if err != nil {
			return nil, fmt.Errorf("ruby --version: %w", err)
		}
		// "ruby 3.4.1 (2024-12-25 revision 48d4efcb85) +PRISM [x86_64-linux]"
		fields := strings.Fields(string(out))
		if len(fields) < 2 {
			return nil, nil
		}
		current = fields[1]
	}

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

	// One "helm" row at a time: two identically-named rows collide in the
	// checklist and both map to the same updater (last install would win).
	// Offer the same-major update first; once taken, the next scan offers the
	// major bump.
	if versionNewer(latestSameMajor, current) {
		items = append(items, checklist.UpdateItem{
			Name:       "helm",
			Source:     "cli",
			CurrentVer: current,
			NewVer:     latestSameMajor,
		})
	} else if latestNewMajor != "" {
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
