# ctdev Go CLI Rewrite — Design Spec

## Overview

Rewrite the ctdev dotfiles management CLI from bash scripts to a Go CLI with a rich TUI. The Go binary lives in the same repo alongside the existing bash scripts. It uses Cobra + Viper for CLI/config and Bubble Tea + Bubbles + Lip Gloss for the TUI.

**Priority:** TUI experience first, then feature parity, extensibility, cross-platform robustness.

**Approach:** TUI-First with Bash Bridge. The Go CLI owns all user interaction. Existing bash install/uninstall scripts continue to do the actual work via `exec.Command`. Components are incrementally ported to pure Go over time.

## Architecture

### Directory Layout

```
ctdev/                    <- Go module root (inside existing repo)
├── main.go               <- Entry point
├── cmd/                  <- Cobra commands
│   ├── root.go           <- Root command + global flags
│   ├── install.go
│   ├── uninstall.go
│   ├── update.go
│   ├── setup.go
│   ├── info.go
│   ├── cleanup.go
│   ├── configure.go
│   └── gpu.go
├── tui/                  <- Bubble Tea models
│   ├── picker/           <- Multi-select component picker
│   ├── checklist/        <- Update checklist
│   ├── wizard/           <- Setup wizard (step-by-step)
│   ├── progress/         <- Progress bars + spinners
│   ├── info/             <- System info display
│   └── styles/           <- Shared Lip Gloss styles
├── component/            <- Component registry + logic
│   ├── registry.go       <- All 36 component definitions
│   ├── component.go      <- Component struct + types
│   └── executor.go       <- Bash bridge + Go executor
├── platform/             <- OS/arch detection
│   ├── detect.go
│   └── packages.go
├── state/                <- XDG state management
│   ├── config.go         <- Viper-backed user config
│   ├── markers.go        <- Install markers
│   └── paths.go          <- XDG path helpers
└── internal/             <- Shared internal utilities
    ├── shell/            <- exec.Command helpers
    └── logging/          <- Structured logging
```

### Data Flow

```
User -> Cobra CLI (parse flags, route) -> Bubble Tea TUI (interactive UI)
  -> Component Registry (Go structs with metadata)
  -> Executor (bash bridge OR pure Go logic)
  -> XDG State (~/.config/ctdev, ~/.local/state/ctdev, ~/.cache/ctdev)
```

### Dual Mode: TUI + Batch

Every command works in two modes:
- **Interactive (default):** Full Bubble Tea TUI when running in a terminal (stdin is a TTY)
- **Batch:** Non-interactive mode when stdin is not a TTY, or when `--batch` global flag is passed. Equivalent to answering "yes" to all prompts and using defaults. Individual commands may also support `-y`/`--yes` as an alias for `--batch`.

## Component Model

### Component Struct

```go
type Category string

const (
    CategoryCLI      Category = "CLI Tools"
    CategoryDesktop  Category = "Desktop Applications"
    CategoryRuntime  Category = "Development Runtimes"
    CategorySecurity Category = "Security & Encryption"
    CategorySystem   Category = "System Tools"
    CategoryInfra    Category = "Infrastructure"
)

type OS string

const (
    OSLinux OS = "linux"
    OSMacOS OS = "macos"
    OSAny   OS = "any"
)

type Component struct {
    Name         string
    Description  string
    Category     Category
    SupportedOS  []OS
    Dependencies []string   // other component names
    Tags         []string   // for fuzzy search

    // Executor: if GoInstall is non-nil, use it. Otherwise shell out.
    GoInstall    func(ctx context.Context, opts ExecOpts) error
    GoUninstall  func(ctx context.Context, opts ExecOpts) error

    // Paths to bash scripts (relative to dotfiles root)
    BashInstall   string  // "components/docker/install.sh"
    BashUninstall string  // "components/docker/uninstall.sh"
}

type ExecOpts struct {
    DryRun  bool
    Force   bool
    Verbose bool
    Stdout  io.Writer  // TUI captures this for progress display
    Stderr  io.Writer
}
```

### Executor

```go
type Executor struct {
    DotfilesRoot string
    Platform     platform.Info
}

func (inst *Executor) Install(ctx context.Context, c *Component, opts ExecOpts) error {
    if c.GoInstall != nil {
        return c.GoInstall(ctx, opts)
    }
    return inst.runBash(ctx, c.BashInstall, opts)
}

func (inst *Executor) runBash(ctx context.Context, script string, opts ExecOpts) error {
    cmd := exec.CommandContext(ctx, "bash", filepath.Join(inst.DotfilesRoot, script))
    cmd.Env = inst.buildEnv(opts) // FORCE, DRY_RUN, VERBOSE
    cmd.Stdout = opts.Stdout
    cmd.Stderr = opts.Stderr
    return cmd.Run()
}
```

Key points:
- `inst` receiver name per project convention
- Function fields instead of interface — nil means shell out, non-nil means run Go
- Stdout/Stderr as io.Writer so TUI can capture output for progress display

### Registry

All 36 components defined in `component/registry.go` as a slice of Component structs. Each component specifies:
- Name, description, category, supported OSes
- Dependencies (e.g., helm depends on kubectl)
- Tags for fuzzy search
- Bash script paths (all components start here)
- GoInstall/GoUninstall (nil initially, populated as components are ported)

### Component Categories

| Category | Components |
|----------|-----------|
| CLI Tools | btop, bun, claude-code, codex, docker, doctl, gh, git-spice, helm, jq, kubectl, shellcheck, tmux |
| Desktop Applications | 1password, chatgpt, chrome, cleanmymac, claude-desktop, dbeaver, ghostty, linear, logi-options, slack, vscode |
| Development Runtimes | fonts, git, node, ruby, zsh |
| Security & Encryption | age, sops, tailscale |
| Infrastructure | terraform |
| System Tools | bleachbit, earlyoom, solaar |

## State Management (XDG)

### Paths

- **Config:** `~/.config/ctdev/config.yaml` — User preferences via Viper
- **State:** `~/.local/state/ctdev/components/<name>.json` — Install markers
- **Cache:** `~/.cache/ctdev/` — Temporary downloads, update check cache

### Config (Viper)

```yaml
# ~/.config/ctdev/config.yaml
default_components:
  - zsh
  - git
  - docker
  - ghostty
update_interval: weekly  # daily, weekly, manual
```

### Install Markers

```json
// ~/.local/state/ctdev/components/docker.json
{
  "installed_at": "2026-03-13T10:00:00Z",
  "version": "27.0.3",
  "updated_at": "2026-03-13T10:00:00Z"
}
```

Upgraded from flat timestamp files to JSON for richer state tracking.

### Migration

On first run, the Go CLI detects existing `~/.config/ctdev/<name>.installed` marker files and migrates them to the new JSON format in `~/.local/state/ctdev/`. The `installed_at` field is populated from the old file's timestamp content (ISO 8601). The `version` field is set to `"unknown"` and `updated_at` is set to `installed_at` since the old format didn't track these. Subsequent updates will populate them correctly.

## TUI Designs

### 1. Component Installer (`ctdev install`)

**No args:** Full-screen multi-select picker.

- Components grouped by category (CLI Tools, Desktop Apps, etc.)
- Collapsible groups via Tab
- Fuzzy filter via `/` key
- Space to toggle, Enter to confirm, `q` to quit
- Already-installed components marked differently
- Unsupported components for current OS dimmed/hidden
- Dependency hints — selecting helm auto-suggests kubectl
- Status bar: selection count, installed count, platform info

**With args:** `ctdev install docker tmux` bypasses TUI, installs directly with progress output. Dependencies are auto-installed (with a note shown) — e.g., `ctdev install helm` also installs kubectl if not present.

### 2. Installation Progress

After confirming selection, TUI switches to progress view:

- Overall progress bar with count (2 of 4)
- Per-component status: completed (green check + time), active (spinner + streaming stdout tail), waiting (dimmed)
- Last 2-3 lines of active bash script's stdout shown in real time
- Sequential execution (one at a time, avoids package manager conflicts)
- Failed components marked red, remaining continue
- Completion summary: succeeded/failed counts, timing, retry command for failures
- `--verbose` shows full output instead of tail

### 3. Update Manager (`ctdev update`)

**Phase 1 — Scan:** Checks all update sources in parallel (apt, flatpak, git repos, bun, NVIDIA). Shows checklist of sources as each completes.

**Phase 2 — Checklist:** All available updates grouped by source:
- System Packages (apt/brew) — shows current -> new version
- Component Updates (git repos) — shows "N commits behind"
- Flatpak — shows version diff
- Runtime Updates (bun) — shows version diff
- Warning badges for kernel updates, major version bumps

All selected by default. `a` for all, `n` for none, Space to toggle, Enter to install.

**Batch modes:** `ctdev update -y` installs all, `ctdev update --check` lists and exits.

### 4. Setup Wizard (`ctdev setup`)

Multi-step wizard with sidebar navigation:

- Left sidebar: all steps with completion status (check, active dot, pending circle)
- Right panel: current step's options as toggleable checkboxes
- Info boxes explain what each option does
- Enter = next step, Esc = back, `s` = skip

**Linux Mint steps:**
1. GPU Drivers — NVIDIA signing, Secure Boot
2. GRUB Config — Menu style, timeout, OS prober
3. Audio & Bluetooth — PipeWire, LDAC, camera, firmware
4. Desktop Services — gsettings, system services
5. Input Devices — Key repeat, mouse bindings
6. Install Components — Opens the component picker

**Smart features:**
- Hardware detection — options only appear if relevant hardware exists
- Already-configured detection — configured options shown as "already active"
- OS-dependent steps — macOS shows its own set

**`ctdev setup --show`:** Dashboard with 4 panels (GPU & Boot, Audio & Bluetooth, Input Devices, Components) showing current configuration at a glance.

**`ctdev setup --batch`:** Non-interactive, applies all defaults.

### 5. System Info (`ctdev info`)

Rich dashboard display:
- System info (OS, arch, package manager, shell, ctdev version)
- Hardware (CPU with thread count, memory, GPU with VRAM, Secure Boot status)
- Disk usage with visual progress bars (color-coded by usage percentage)
- Installed components as pill badges
- Update availability count

### 6. Other Commands

- **`ctdev uninstall`:** Same picker as install, filtered to installed components only. Red selection indicators.
- **`ctdev cleanup`:** Checklist of cleanup tasks with size/count info per task (old kernels, APT repos, package cache).
- **`ctdev configure git`:** Bubble Tea text inputs for name/email, tab navigation, global/local scope toggle.
- **`ctdev gpu info`:** Static display (no interactivity needed).
- **`ctdev gpu setup`:** Step-by-step flow similar to setup wizard, GPU-specific.

## CLI Flags

### Global Flags (same as today)

- `-h, --help` — Show help
- `-v, --verbose` — Verbose output
- `-n, --dry-run` — Preview without applying
- `-f, --force` — Force re-run
- `--version` — Show version
- `--batch` — Non-interactive mode (new)

### Command-Specific Flags

All existing flags from the bash CLI are preserved:

**`ctdev setup`:**
- `--show` — Display current configuration
- `--reset` — Reset configuration to defaults
- `--skip-gpu` — Skip GPU driver setup
- `--skip-configure` — Skip interactive configuration prompts

**`ctdev update`:**
- `-y, --yes` — Skip confirmation (alias for global `--batch`)
- `--check` — List available updates without installing
- `--refresh-keys [COMPONENT...]` — Refresh APT GPG keys before updating

**`ctdev configure git`:**
- `--name` — Set git user name
- `--email` — Set git user email
- `--local` — Apply to current repo only
- `--show` — Show current git config

**`ctdev gpu setup`:**
- `--recover` — Re-enroll MOK after CMOS reset

## Dependencies

- **cobra** — CLI framework
- **viper** — Configuration management
- **bubbletea** — TUI framework
- **bubbles** — TUI components (list, spinner, progress, textinput, viewport)
- **lipgloss** — TUI styling

## Error Handling

- Bash script exit code 0 = success, 2 = unsupported OS (skip), other = failure
- Failed components don't block remaining installations
- Errors shown inline in TUI with red indicators
- Completion summary always shown with retry commands
- `--verbose` expands error output

## Testing Strategy

- Unit tests for component registry, platform detection, state management, marker migration
- Integration tests for executor (bash bridge) — test with simple test scripts, not real package managers
- TUI tests using Bubble Tea's `teatest` package — send key sequences, assert model state and view output. These run headless (no TTY required), safe for CI.
- No mocking of package managers — test the orchestration layer, not apt/brew

## Build & Version

The binary version is injected at build time via Go linker flags (`-ldflags`), reading from the `VERSION` file:

```bash
go build -ldflags "-X main.version=$(cat ../VERSION)" -o ctdev ./
```

## Migration Path

1. Go module lives at `ctdev/` in the repo
2. `go build` produces the `ctdev` binary
3. Pre-built binaries published as GitHub release assets (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64). Install script downloads the right binary for the platform — no Go toolchain required on the target machine. Building from source is optional for development.
4. Existing bash scripts remain in `components/` — Go shells out to them
5. Components ported to pure Go incrementally (no rush)
6. Old `cmds/` and `lib/` directories remain until all components are ported
