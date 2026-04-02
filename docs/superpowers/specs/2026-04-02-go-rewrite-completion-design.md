# ctdev Go CLI Rewrite Completion — Design Spec

## Overview

Complete the Go CLI rewrite by fixing bugs, adding helper abstractions, porting remaining components, adding test coverage, and polishing the TUI. This builds on the existing architecture from the original Go CLI rewrite spec.

**Current state:** 22 of 35 components ported to Go. All commands functional. TUI working with Bubble Tea v2. Bash executor fallback handles unported components.

**Goal:** Production-ready CLI with reduced boilerplate, comprehensive tests, bug fixes, and polished UX.

## Phase 1: Foundation Fixes & Helpers

### 1.1 Cache platform.Detect()

`platform.Detect()` reads `/etc/os-release` and probes for package managers on every call. Every `install_*.go` file calls it independently.

**Fix:** Add `sync.Once` caching in `platform/detect.go`:

```go
var (
    cachedInfo Info
    detectOnce sync.Once
)

func Detect() Info {
    detectOnce.Do(func() {
        cachedInfo = detect()
    })
    return cachedInfo
}

func detect() Info { /* existing logic */ }
```

### 1.2 SimplePackageInstaller / SimplePackageUninstaller

Four files (`install_btop.go`, `install_jq.go`, `install_shellcheck.go`, `install_tmux.go`) are identical except for the package name.

**New functions in `component/helpers.go`:**

```go
func SimplePackageInstaller(name string) func(context.Context, ExecOpts) error
func SimplePackageUninstaller(name string) func(context.Context, ExecOpts) error
```

**Registry usage:**
```go
{Name: "btop", ..., GoInstall: SimplePackageInstaller("btop"), GoUninstall: SimplePackageUninstaller("btop")}
```

**Delete:** `install_btop.go`, `install_jq.go`, `install_shellcheck.go`, `install_tmux.go`

### 1.3 DownloadGitHubBinary helper

Six files (helm, doctl, kubectl, age, sops, git-spice) repeat the same download+verify+extract+install pattern (~40 lines each).

**New function in `sysutil/download.go`:**

```go
type GitHubBinaryOpts struct {
    Repo         string            // "helm/helm"
    ArchiveURL   func(version, os, arch string) string
    ChecksumURL  func(version, os, arch string) string // optional
    BinaryPath   func(os, arch string) string           // path within archive
    InstallDest  string                                  // e.g. "/usr/local/bin/helm"
    ArchFormat   string                                  // "tar.gz", "zip", or "" for raw binary
}

func DownloadGitHubBinary(o Opts, gb GitHubBinaryOpts) error
```

Each install file shrinks from ~40 lines to ~15 lines (just defining the opts struct).

### 1.4 Fix refreshAPTKeys no-op

`cmd/update.go:1105-1112` prints a message but does nothing.

**Fix:** Define a `var aptKeyRefreshers` map in `cmd/update.go` that maps component names to their keyring download functions. Each entry calls the same keyring download logic from the component's installer (e.g., `sysutil.AddAPTKeyring` for gh, vscode, 1password, terraform). The `refreshAPTKeys` function iterates this map and re-downloads each keyring. If `--refresh-keys` is passed with component names as args, only refresh those; otherwise refresh all.

### 1.5 Fix cleanup audit no-op

`cmd/cleanup.go:61-63` "Audit APT repositories" always returns nil.

**Fix:** Implement actual duplicate detection:
- Read all files in `/etc/apt/sources.list.d/`
- Parse source lines and detect duplicates (same URI + suite + component)
- Report findings to user

### 1.6 Replace hand-rolled JSON in update.go

- Replace `fetchLatestGitHubTag()` with `sysutil.GitHubLatestVersion()`
- Replace `fetchGitHubReleaseTags()` with proper `encoding/json` parsing
- Replace `parseNPMOutdated()` with `json.Unmarshal` into `map[string]struct{Current, Latest string}`
- Replace `extractJSONValue()` — delete entirely, all callers migrated
- Replace kubectl/terraform version JSON parsing with proper structs

## Phase 2: Port Remaining Components

### Priority order (by complexity and usage frequency):

**Tier 1 — Medium complexity (binary downloads):**
1. `chrome` — APT repo on Linux, brew cask on macOS
2. `ghostty` — APT PPA on Linux (zig.pm repo), brew cask on macOS
3. `tailscale` — official install script or APT repo
4. `claude-code` — npm global install
5. `codex` — npm global install
6. `bun` — curl installer
7. `dbeaver` — .deb download or brew cask

**Tier 2 — Complex (version managers, multi-step setup):**
8. `fonts` — download and extract Nerd Fonts to ~/.local/share/fonts
9. `docker` — APT repo setup, group management, systemd
10. `git` — config templating, aliases, includes
11. `node` — nodenv + node-build, version management
12. `ruby` — rbenv + ruby-build, version management
13. `zsh` — oh-my-zsh, plugins, Pure prompt, config symlinks

**Stop criterion:** If Tier 2 components are taking disproportionate effort relative to the bash fallback quality, stop and leave them as bash. The executor handles them fine.

### Per-component pattern:

Each new Go installer follows the existing pattern:
- Check platform support (return `ErrUnsupportedOS` if not)
- Check if already installed (skip unless `--force`)
- Use helpers where applicable (`SimplePackageInstaller`, `DownloadGitHubBinary`)
- Use `sysutil` for APT operations, downloads, service management

## Phase 3: Test Coverage

### 3.1 Extract testable logic from cmd/

The update scanners contain parsing logic mixed with `exec.Command` calls. Extract the parsing into pure functions:

```go
// Testable: takes raw command output, returns parsed items
func parseAPTUpgradable(output string) []checklist.UpdateItem
func parseBrewOutdated(output string) []checklist.UpdateItem
func parseFlatpakUpdates(output string) []checklist.UpdateItem
func parseNPMOutdatedJSON(jsonStr string) []checklist.UpdateItem
func parseGitRevCount(behind string, currentSHA string, remoteSHA string) *checklist.UpdateItem
```

The scanner functions become thin wrappers: run command, pass output to parser.

### 3.2 Test files to add

| File | Tests |
|------|-------|
| `cmd/update_parse_test.go` | APT, brew, flatpak, npm, git output parsing |
| `cmd/cleanup_test.go` | APT duplicate detection logic |
| `sysutil/download_test.go` | Checksum verification, GitHub version parsing |
| `sysutil/apt_test.go` | Keyring path generation, sources list generation |
| `component/helpers_test.go` | SimplePackageInstaller behavior |
| `tui/progress/progress_test.go` | Uninstall mode messages (extend existing) |

### 3.3 Test approach

- Use table-driven tests with real-world command output samples
- No mocking of the filesystem or exec — test pure parsing functions
- Test edge cases: empty output, malformed lines, missing fields
- For sysutil/download: test checksum verification with temp files

## Phase 4: UX/UI Improvements

### 4.1 Picker: platform availability badges

Show components that don't support the current OS as dimmed with a platform badge:
```
  ✓ btop          Resource monitor
  ○ docker         Container runtime
  ⊘ cleanmymac     CleanMyMac system cleaner  (macOS only)
```

Currently unsupported components are hidden entirely by `FilterByOS`. Change the picker to receive the full registry and dim+disable items that don't support the current OS, rather than filtering them out. This lets users see everything that exists. Dimmed items skip when toggled.

### 4.2 Progress: elapsed time during install

Add a running clock next to the spinner for the current component:
```
  ⣾ Installing helm... (12s)
  ✓ kubectl    3.2s
  ✓ jq         0.8s
```

Implementation: add a `tea.Tick` that fires every second, update the display with `time.Since(start)`.

### 4.3 Update: scanner progress during scan phase

Replace the static "Scanning for updates..." with a live count:
```
  Scanning for updates... (7/13 sources checked)
```

Implementation: have each scanner goroutine send a message to a channel, count completions.

### 4.4 Info: terminal-width-aware layout

The two-column component layout hardcodes `colWidth := 20`. Detect terminal width and adjust:
- < 60 chars: single column
- 60-100 chars: two columns (current)
- > 100 chars: three columns

### 4.5 Setup: macOS note

`cmd/setup.go` delegates to bash for macOS. Add a clear message:
```
macOS setup uses the system configuration script.
```

This is not worth porting to Go — macOS setup is fundamentally different (no dconf, no GRUB, no systemd).

## Phase 5: Cleanup

- Remove `state/migrate.go` if marker migration is no longer called (was removed from PersistentPreRunE)
- Verify no dead code remains with `go vet ./...`
- Run `staticcheck` if available
- Final `go test ./...` pass
- Update CLAUDE.md component count if changed

## Out of Scope

- Plugin/extension system for components
- Logging framework (stdout is fine for a CLI tool)
- State versioning for markers
- Rollback on partial install failure
- Windows support
