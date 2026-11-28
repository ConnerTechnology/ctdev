// Update scanners: scanAll and the per-source scanners (apt, brew, flatpak,
// runtimes, CLIs, ...) plus the output-parsing helpers they use.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/component"
	"github.com/ConnerTechnology/ctdev/ctdev/nodenv"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/checklist"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// scanAll gathers every update ctdev can offer. `ctdev update` upgrades whatever
// a package manager installed (APT, Homebrew, Flatpak, npm and bun globals) on
// its own. A component that bypasses one, such as a downloaded .deb, an upstream
// install.sh or a release tarball, is invisible to those scans and needs its own
// scanner here unless the software updates itself (a cask marked auto_updates
// needs none). A scanner compares the installed version with the vendor's latest
// stable release (sysutil.GitHubLatestVersion for GitHub) and returns a
// checklist.UpdateItem with Source "cli" when it is behind; add the item's name
// to cliUpdateStep in update_apply.go, which prefers the tool's own updater and
// otherwise re-runs the install. updateNeedsRoot in update.go assumes a "cli"
// item needs root, so exempt the name there when its update stays inside $HOME.
// A component installed as a git clone goes in gitRepos instead.
func scanAll(ctx context.Context) ([]checklist.UpdateItem, error) {
	type namedScanner struct {
		name string
		fn   func(context.Context) ([]checklist.UpdateItem, error)
	}
	scanners := []namedScanner{
		{"apt", scanAPT},
		{"flatpak", scanFlatpak},
		{"brew", scanBrew},
		{"bun", scanBun},
		{"npm", scanNPMGlobals},
		{"ctdev", scanCtdev},
		{"docker", scanDocker},
		{"go", scanGo},
		{"ruby", scanRuby},
		{"pyenv", scanPyenv},
		{"helm", scanHelm},
		{"kubectl", scanKubectl},
		{"terraform", scanTerraform},
		{"ctops", scanCTops},
		{"t3code", scanT3Code},
		{"orca", scanOrca},
	}

	for _, r := range gitRepos() {
		scanners = append(scanners, namedScanner{r.name, func(ctx context.Context) ([]checklist.UpdateItem, error) {
			return scanGitRepo(ctx, r.name, r.dir)
		}})
	}

	// A source can discover updates before one of its probes fails. Deliver both
	// together so a failed probe never drops the items that can be updated.
	type result struct {
		source string
		items  []checklist.UpdateItem
		err    error
	}
	done := make(chan result)
	for _, scanner := range scanners {
		go func() {
			r := result{source: scanner.name}
			defer func() {
				if p := recover(); p != nil {
					r.err = fmt.Errorf("panic: %v", p)
				}
				done <- r
			}()
			r.items, r.err = scanner.fn(ctx)
		}()
	}
	var allItems []checklist.UpdateItem
	var failures []string
	for i := range scanners {
		r := <-done
		fmt.Printf("\r  %s", styles.Dimmed.Render(fmt.Sprintf("Scanning for updates... (%d/%d sources checked)", i+1, len(scanners))))
		allItems = append(allItems, r.items...)
		if r.err != nil {
			failure := r.source
			if flagVerbose {
				failure += ": " + r.err.Error()
			}
			failures = append(failures, failure)
		}
	}
	fmt.Println()

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

	if len(failures) > 0 {
		sort.Strings(failures)
		return allItems, fmt.Errorf("update scan incomplete for %s", strings.Join(failures, "; "))
	}
	return allItems, nil
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

// brewManages reports whether Homebrew owns this formula on this machine.
//
// Homebrew updates the runtimes it owns. Offering the upstream Go archive
// or rbenv path as well would update a different installation and list the
// same tool twice.
//
// `brew list --versions` exits non-zero when the formula isn't installed, which
// is exactly the question being asked.
func brewManages(ctx context.Context, formula string) bool {
	if _, err := exec.LookPath("brew"); err != nil {
		return false
	}
	return exec.CommandContext(ctx, "brew", "list", "--versions", formula).Run() == nil
}

// brewOutdated mirrors the shape of `brew outdated --json=v2`. Both arrays carry
// the same fields (see Homebrew's cmd/outdated.rb and cask/cask.rb).
//
// The JSON form exists because the human-readable one can't be parsed reliably:
// `brew outdated --verbose` prints `openssl@3 (3.3.2, 3.4.0) < 3.4.1` when two
// kegs are installed, and the current version may itself be multi-word
// ("latest HEAD") or carry a trailing "[pinned at X]".
type brewOutdated struct {
	Formulae []brewOutdatedEntry `json:"formulae"`
	Casks    []brewOutdatedEntry `json:"casks"`
}

type brewOutdatedEntry struct {
	Name              string   `json:"name"`
	InstalledVersions []string `json:"installed_versions"`
	CurrentVersion    string   `json:"current_version"`
	Pinned            bool     `json:"pinned"`
}

// parseBrewOutdatedJSON turns `brew outdated --json=v2` output into update items
// for both the "brew" and "brew-cask" sources.
//
// Pinned entries are dropped: `brew upgrade` errors out on a pinned formula, and
// the apply step batches every name into a single command, so one pinned formula
// would fail the whole batch.
func parseBrewOutdatedJSON(data []byte) ([]checklist.UpdateItem, error) {
	var out brewOutdated
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse brew outdated: %w", err)
	}

	var items []checklist.UpdateItem
	for _, f := range out.Formulae {
		if f.Pinned || f.Name == "" {
			continue
		}
		items = append(items, checklist.UpdateItem{
			Name:       f.Name,
			Source:     "brew",
			CurrentVer: strings.Join(f.InstalledVersions, ", "),
			NewVer:     f.CurrentVersion,
		})
	}
	for _, c := range out.Casks {
		if c.Pinned || c.Name == "" {
			continue
		}
		// Only surface casks ctdev manages and that are actually installed.
		compName, managed := managedCasks[c.Name]
		if !managed {
			continue
		}
		comp := component.FindByName(compName)
		if comp == nil || !comp.IsInstalled() {
			continue
		}
		items = append(items, checklist.UpdateItem{
			Name:       c.Name,
			Source:     "brew-cask",
			CurrentVer: strings.Join(c.InstalledVersions, ", "),
			NewVer:     c.CurrentVersion,
		})
	}
	return items, nil
}

// scanBrew reports both outdated formulae and outdated casks from a single
// `brew outdated` call — one invocation instead of the two the text-scraping
// version needed. --greedy keeps self-updating casks (Chrome, Slack) in view;
// managedCasks narrows the result to what ctdev installed.
func scanBrew(ctx context.Context) ([]checklist.UpdateItem, error) {
	if _, err := exec.LookPath("brew"); err != nil {
		return nil, nil
	}
	out, err := exec.CommandContext(ctx, "brew", "outdated", "--json=v2", "--greedy").Output()
	if err != nil {
		return nil, err
	}
	return parseBrewOutdatedJSON(out)
}

// managedCasks maps brew cask names to ctdev component names.
// Only installed components are shown in update output.
var managedCasks = map[string]string{
	"1password":                     "1password",
	"cleanmymac":                    "cleanmymac",
	"claude":                        "claude-desktop",
	"dbeaver-community":             "dbeaver",
	"docker":                        "docker",
	"google-chrome":                 "chrome",
	"linear-linear":                 "linear",
	"logi-options+":                 "logi-options",
	"slack":                         "slack",
	"tailscale":                     "tailscale",
	"visual-studio-code":            "vscode",
	"font-fira-code-nerd-font":      "fonts",
	"font-jetbrains-mono-nerd-font": "fonts",
}

// gitRepos are the tools ctdev installs as a git clone, which only a pull
// updates. Homebrew's nodenv has no clone, and brew updates it instead.
func gitRepos() []struct{ name, dir string } {
	home, _ := os.UserHomeDir()
	return []struct{ name, dir string }{
		{"oh-my-zsh", filepath.Join(home, ".oh-my-zsh")},
		{"nodenv", nodenv.Root()},
		{"node-build", filepath.Join(nodenv.Root(), "plugins", "node-build")},
	}
}

func gitRepoDir(name string) string {
	for _, r := range gitRepos() {
		if r.name == name {
			return r.dir
		}
	}
	return ""
}

// scanGitRepo reports the clone at dir when it is behind its upstream.
func scanGitRepo(ctx context.Context, name, dir string) ([]checklist.UpdateItem, error) {
	if _, err := os.Stat(dir + "/.git"); err != nil {
		return nil, nil
	}
	if err := exec.CommandContext(ctx, "git", "-C", dir, "fetch", "--quiet").Run(); err != nil {
		return nil, fmt.Errorf("git fetch %s: %w", name, err)
	}
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-list", "--count", "HEAD..@{u}").Output()
	if err != nil {
		return nil, fmt.Errorf("%s rev-list: %w", name, err)
	}
	behind := strings.TrimSpace(string(out))
	if behind == "" || behind == "0" {
		return nil, nil
	}
	currentSHA, _ := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--short", "HEAD").Output()
	remoteSHA, _ := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--short", "@{u}").Output()
	return []checklist.UpdateItem{{
		Name:       name,
		Source:     "git",
		CurrentVer: strings.TrimSpace(string(currentSHA)),
		NewVer:     strings.TrimSpace(string(remoteSHA)) + " (" + behind + " commits)",
	}}, nil
}

func scanBun(ctx context.Context) ([]checklist.UpdateItem, error) {
	if _, err := exec.LookPath("bun"); err != nil {
		return nil, nil
	}
	currentOut, err := exec.CommandContext(ctx, "bun", "--version").Output()
	if err != nil {
		return nil, fmt.Errorf("bun --version: %w", err)
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

func scanPyenv(ctx context.Context) ([]checklist.UpdateItem, error) {
	if _, err := exec.LookPath("pyenv"); err != nil {
		return nil, nil
	}
	// Compare against the global version: `pyenv version` resolves the cwd's
	// .python-version, so a project pin would make an applied update reappear.
	// `pyenv global` can list several versions, one per line; the first is the
	// one `python` runs.
	currentOut, err := exec.CommandContext(ctx, "pyenv", "global").Output()
	if err != nil {
		return nil, fmt.Errorf("pyenv global: %w", err)
	}
	current, _, _ := strings.Cut(strings.TrimSpace(string(currentOut)), "\n")
	current = strings.TrimSpace(current)
	if current == "" || current[0] < '0' || current[0] > '9' {
		// "system" or unset — not pyenv-managed, nothing for us to update
		return nil, nil
	}

	latest, err := fetchLatestPythonPatch(ctx, current)
	if err != nil {
		return nil, fmt.Errorf("pyenv install --list: %w", err)
	}
	if latest == "" || !versionNewer(latest, current) {
		return nil, nil
	}
	return []checklist.UpdateItem{{
		Name:       "python (pyenv)",
		Source:     "runtime",
		CurrentVer: current,
		NewVer:     latest,
	}}, nil
}

// globalNpm is how to run npm against the global Node: through nodenv with
// NODENV_VERSION set, so a project's .node-version in the current folder
// can't redirect it (npm components install into the global Node), else the
// npm on PATH. ok is false when nodenv has no global of its own ("system"
// included): npm components install only into a nodenv Node, so there is no
// machine-wide npm for ctdev to update.
func globalNpm() (env []string, name string, prefix []string, ok bool) {
	if v := nodenv.GlobalSelector(); v != "" && v != nodenv.System {
		return []string{"NODENV_VERSION=" + v}, nodenv.Bin(), []string{"exec", "npm"}, true
	}
	if nodenv.Available() {
		return nil, "", nil, false
	}
	return nil, "npm", nil, true
}

func scanNPMGlobals(ctx context.Context) ([]checklist.UpdateItem, error) {
	env, name, prefix, ok := globalNpm()
	if !ok {
		return nil, nil
	}
	if _, err := exec.LookPath(name); err != nil {
		return nil, nil
	}
	// npm exits 1 when it finds outdated packages, but errors such as a
	// registry failure can also produce JSON. Check both the exit and payload.
	c := exec.CommandContext(ctx, name, append(prefix, "outdated", "-g", "--json")...)
	c.Env = append(os.Environ(), env...)
	out, err := c.Output()
	content := strings.TrimSpace(string(out))
	if err != nil {
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 || content == "" || content == "{}" {
			return nil, fmt.Errorf("npm outdated -g: %w", err)
		}
	}
	if content == "" {
		return nil, nil
	}
	return parseNPMOutdatedJSON(content)
}

func parseNPMOutdatedJSON(content string) ([]checklist.UpdateItem, error) {
	var packages map[string]struct {
		Current string `json:"current"`
		Latest  string `json:"latest"`
		Code    string `json:"code"` // npm error envelope
	}
	if err := json.Unmarshal([]byte(content), &packages); err != nil {
		return nil, err
	}
	if failure := packages["error"]; failure.Code != "" {
		return nil, fmt.Errorf("npm outdated failed: %s", failure.Code)
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
	latest, err := sysutil.GitHubLatestVersion(ctx, "ConnerTechnology/ctdev")
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
	// Same for Homebrew — the brew scanner already covers it.
	if brewManages(ctx, "go") {
		return nil, nil
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
		return nil, fmt.Errorf("could not read go version")
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
	// Homebrew's ruby is upgraded by the brew scanner; rbenv would fight it.
	if brewManages(ctx, "ruby") {
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
			return nil, fmt.Errorf("could not read ruby version")
		}
		current = fields[1]
	}

	// fetchLatestRubyVersion returns "" when ruby-build isn't present, which is a
	// legitimate "can't determine", not a failure — stay silent in that case.
	latest, err := fetchLatestRubyVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("ruby-build --definitions: %w", err)
	}
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
		return nil, fmt.Errorf("helm version: %w", err)
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
		return nil, fmt.Errorf("kubectl version: %w", err)
	}
	var kubectlVer struct {
		ClientVersion struct {
			GitVersion string `json:"gitVersion"`
		} `json:"clientVersion"`
	}
	if err := json.Unmarshal(out, &kubectlVer); err != nil {
		return nil, fmt.Errorf("parse kubectl version: %w", err)
	}
	current := strings.TrimPrefix(kubectlVer.ClientVersion.GitVersion, "v")
	if current == "" {
		return nil, fmt.Errorf("kubectl returned no client version")
	}

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
		return nil, fmt.Errorf("terraform version: %w", err)
	}
	var tfVer struct {
		TerraformVersion string `json:"terraform_version"`
	}
	if err := json.Unmarshal(out, &tfVer); err != nil {
		return nil, fmt.Errorf("parse terraform version: %w", err)
	}
	current := tfVer.TerraformVersion
	if current == "" {
		return nil, fmt.Errorf("terraform returned no version")
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

// scanCTops uses gh for the private release repo, sharing tag selection with
// install so the checklist offers the same stable channel on Linux and macOS.
func scanCTops(ctx context.Context) ([]checklist.UpdateItem, error) {
	// A stalled gh must not hold up the whole scan; ctdev's HTTP checks give up after a minute too.
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	current := component.CTopsVersion(ctx)
	if current == "" {
		return nil, nil
	}
	latest, err := component.CTopsLatestVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("check latest ctops: %w", err)
	}
	if !versionNewer(latest, current) {
		return nil, nil
	}
	return []checklist.UpdateItem{{Name: "ctops", Source: "cli", CurrentVer: current, NewVer: latest}}, nil
}

// scanT3Code offers the T3 Code CLI, and the Linux desktop .deb, when either is
// behind the latest stable release. Both install outside a package manager's
// upgrade path; the macOS desktop app updates itself, so it isn't scanned.
func scanT3Code(ctx context.Context) ([]checklist.UpdateItem, error) {
	cli := component.T3CodeVersion(ctx)
	desktop := ""
	if runtime.GOOS == "linux" {
		desktop = component.T3CodeDesktopVersion(ctx)
	}
	if cli == "" && desktop == "" {
		return nil, nil
	}
	latest, err := sysutil.GitHubLatestVersion(ctx, component.T3CodeRepo)
	if err != nil {
		return nil, fmt.Errorf("check latest T3 Code: %w", err)
	}
	return t3codeUpdateItems(cli, desktop, latest), nil
}

func t3codeUpdateItems(cli, desktop, latest string) []checklist.UpdateItem {
	var items []checklist.UpdateItem
	if cli != "" && versionNewer(latest, cli) {
		items = append(items, checklist.UpdateItem{Name: "t3code", Source: "cli", CurrentVer: cli, NewVer: latest})
	}
	if desktop != "" && versionNewer(latest, desktop) {
		items = append(items, checklist.UpdateItem{Name: "t3code-desktop", Source: "cli", CurrentVer: desktop, NewVer: latest})
	}
	return items
}

// scanOrca offers the Orca .deb on Linux when it is behind the latest release.
// The macOS cask updates itself, so it isn't scanned.
func scanOrca(ctx context.Context) ([]checklist.UpdateItem, error) {
	if runtime.GOOS != "linux" {
		return nil, nil
	}
	current := component.OrcaVersion(ctx)
	if current == "" {
		return nil, nil
	}
	latest, err := sysutil.GitHubLatestVersion(ctx, component.OrcaRepo)
	if err != nil {
		return nil, fmt.Errorf("check latest Orca: %w", err)
	}
	if !versionNewer(latest, current) {
		return nil, nil
	}
	return []checklist.UpdateItem{{Name: "orca", Source: "cli", CurrentVer: current, NewVer: latest}}, nil
}
