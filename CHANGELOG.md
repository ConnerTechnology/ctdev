# Changelog

All notable changes to this project will be documented in this file.

## [9.0.0] - 2026-04-02

### Changed
- Complete rewrite from bash to Go — ctdev is now a single compiled binary
- All 35 component installers ported to native Go with proper error handling
- Bubble Tea TUI for interactive component picker, setup wizard, and update checklist
- Embedded config files via go:embed (no more symlinks)
- Install via GitHub Releases binary download instead of cloning the repo
- Self-update via `ctdev update` checks GitHub Releases for new versions

### Added
- `ctdev configure git` — interactive SSH key picker, signing key setup, GitHub upload
- `ctdev configure aws` — AWS profile picker from ~/.aws/config
- `ctdev gpu info` — GPU hardware info with NVIDIA signing status
- `ctdev gpu setup` — MOK enrollment and DKMS signing for Secure Boot
- Batch/CI mode — all commands work non-interactively when stdin is not a TTY
- Zsh completions for all commands including configure and gpu subcommands
- Progress TUI shows elapsed time and per-component output during install/uninstall

### Fixed
- APT keyring and source operations now use sudo correctly
- Component detection uses app bundles on macOS instead of CLI commands
- Setup key repeat rate correctly converts cps to milliseconds for gsettings
- Setup reset now undoes the same dconf paths that apply uses
- Kernel cleanup scoped to versioned packages only (won't remove meta-packages)
- GRUB backed up before any sed modifications
- Update functions use safe temp directories instead of predictable /tmp paths
- Pre-release versions filtered from GitHub release tag scanning

## [8.4.0] - 2026-03-08

### Changed
- Keyboard repeat rate set to 200ms delay / 50 cps via `xset r rate 200 50` in Linux Mint setup
- Updated gsettings keyboard delay (200ms) and repeat interval (20ms) to match xset values
- `ctdev setup --show` displays current xset repeat rate
- `ctdev setup --reset` resets xset to X11 defaults

## [8.3.1] - 2026-03-07

### Changed
- GRUB setup defaults to `menu` style with 10s timeout and OS prober enabled

## [8.3.0] - 2026-03-07

### Added
- xbindkeys mouse button bindings in `ctdev setup` (middle click → Expo via xdotool)
- Installs `xbindkeys` and `xdotool` packages, symlinks config, creates autostart entry
- xbindkeys status in `ctdev setup --show`
- xbindkeys cleanup in `ctdev setup --reset`

## [8.2.0] - 2026-03-05

### Removed
- Swapfile creation and `vm.swappiness` tuning from `ctdev setup` (not needed for Linux Mint)

## [8.1.0] - 2026-03-05

### Added
- Bluetooth/audio/camera support in `ctdev setup` (LDAC, PipeWire Bluetooth, v4l-utils, linux-firmware)
- WirePlumber LDAC Bluetooth audio config deployed from repo template
- Bluetooth service enablement in `ctdev setup`
- `vm.swappiness=10` tuning for high-memory desktops
- `ctdev install list` to show available components with install status (filtered by OS)
- New component: `solaar` (Logitech Unifying/Bolt receiver manager, Linux-only)
- New component: `tailscale` (Tailscale VPN, macOS + Linux)
- Bluetooth & audio status in `ctdev setup --show`

### Changed
- Simplified README to focus on getting started and commands
- `ctdev setup --reset` now removes WirePlumber config and swappiness tuning

## [8.0.0] - 2026-03-04

### Breaking Changes
- `ctdev list` removed (use `ctdev install --help` to see available components)
- `ctdev configure macos` and `ctdev configure linux-mint` removed (use `ctdev setup` instead)
- `ctdev cleanup kernels` and `ctdev cleanup apt` subcommands removed (use `ctdev cleanup` which runs all tasks sequentially)

### Added
- `ctdev setup --show` to display current system configuration
- `ctdev setup --reset` to reset system configuration to defaults

### Changed
- `ctdev setup` simplified to GPU setup + system configuration (update/cleanup removed from setup flow)
- `ctdev cleanup` now runs all tasks sequentially with prompts instead of requiring a subcommand

### Fixed
- `ctdev update` silently failing to update zsh, node, and ruby components (git pull used wrong branch name when `origin/HEAD` was unset)
- `ctdev update` showing bun as updatable even when already on latest version (now checks GitHub releases)
- `ctdev update` suppressing bun upgrade output with `2>/dev/null`
- `ctdev update` redundantly re-checking for component updates during the update phase

## [7.12.0] - 2026-03-03

### Added
- Comprehensive GPU driver signing for Secure Boot (`ctdev gpu status` and `ctdev gpu setup`)
  - Detect open vs closed NVIDIA driver variant
  - MOK clutter detection and cleanup (flags unexpected files in `/var/lib/shim-signed/mok/`)
  - DKMS `framework.conf` configuration for automatic module signing
  - Module signature verification against enrolled MOK certificate
  - `--recover` flag for re-enrolling MOK key after CMOS/firmware reset
  - DKMS rebuild with post-rebuild signature verification

### Changed
- `ctdev gpu` reduced to 2 subcommands: `status` and `setup` (removed `sign` and `info`)
- Zsh completion updated for new GPU subcommand structure

### Fixed
- CI test failures: removed references to nonexistent `cli` component, `--skip-system` flag, `~/.gitignore`, and `~/.gitconfig.local`
- CI `shellcheck components/cli/*.sh` step removed (directory doesn't exist)
- `ctdev info` crash on CI due to `pipefail` + `grep` in GPU and disk detection pipelines
- ShellCheck SC2034 warning for unused variable in `cmds/list.sh`
- `--dry-run install` now requires a component name (matches CLI behavior)
- Git configure `--skip` no longer ignores explicit `--name`/`--email` arguments

## [7.11.0] - 2026-03-03

### Added
- `earlyoom` component: Early OOM killer for Linux (apt, dnf, pacman)
- Swap file management in `ctdev configure linux-mint` (creates 8GB swap file, adds to fstab)
- Swap info display in `ctdev configure linux-mint --show`
- Swap reset in `ctdev configure linux-mint --reset`

### Changed
- Repository URLs updated to `ConnerTechnology/dotfiles`
- Git config: added SSH signing key, removed `compactionHeuristic`
- Claude Code settings: added `Bash(journalctl:*)` permission
- Component count updated to 35

## [7.10.1] - 2026-03-02

### Removed
- Dock auto-hide delay and animation speed overrides from macOS defaults (restores default macOS hide/show animation)

## [7.10.0] - 2026-03-01

### Added
- Dock auto-hide to macOS defaults
- Volume change feedback to macOS defaults

### Removed
- Design and implementation plan documents from `docs/plans`

## [7.9.1] - 2026-02-28

### Removed
- `AppleShowAllExtensions` from macOS defaults (prevents .app extensions showing on applications)
- `minimize-to-application` from macOS defaults (windows now minimize as separate dock tiles)

## [7.9.0] - 2026-02-25

### Changed
- Renamed `ctdev upgrade` to `ctdev update` — all flags and behavior preserved

### Removed
- `ctdev upgrade` command (replaced by `ctdev update`)

## [7.8.0] - 2026-02-19

### Added
- `ctdev upgrade --check` flag to list available updates without installing
- `ctdev upgrade --refresh-keys` flag (absorbed from `ctdev update`)
- macOS `softwareupdate` as an upgrade source
- Flatpak packages as an upgrade source
- Bun self-upgrade as an upgrade source
- NVIDIA kernel module re-signing after system upgrades (Linux, Secure Boot)

### Changed
- Merged `ctdev update` into `ctdev upgrade` — single command for all upgrade operations
- `ctdev update` now shows deprecation warning and forwards to `ctdev upgrade --check`

## [7.7.0] - 2026-02-18

### Added
- `check_installed_cmd` and `check_installed_app` utility functions in `lib/utils.sh`
- FORCE flag support for 9 macOS GUI app installs (1password, chrome, claude-desktop, cleanmymac, dbeaver, linear, logi-options, slack, vscode)
- Documented `ctdev gpu`, `ctdev configure linux-mint`, and `--refresh-keys` flag in CLAUDE.md

### Changed
- Refactored 23 component install scripts to use shared `check_installed_cmd`/`check_installed_app` utilities
- Standardized initial logging to `log_info` across all components (was mixed `log_step`/`log_info`)

### Fixed
- TradingView macOS install now uses Homebrew cask instead of manual DMG download
- Ghostty and tmux install scripts had wrong `DOTFILES_ROOT` path (`../../..` instead of `../..`)
- Ghostty macOS install now uses `install_brew_cask` instead of raw `brew install --cask`
- TradingView quarantine flag removed after install to prevent macOS Gatekeeper hang

## [7.6.1] - 2026-02-17

### Fixed
- Removed SSH URL rewrite from gitconfig that forced HTTPS clones to use SSH

## [7.6.0] - 2026-02-14

### Added
- Zsh tab completion for `ctdev` CLI
  - Completes all commands, global flags, and command-specific options
  - Component names with descriptions for `install`, `uninstall`, and `upgrade`
  - Configure targets (`git`, `macos`, `linux-mint`) with per-target flags
  - GPU subcommands (`status`, `setup`, `sign`, `info`)
  - Dynamically reads components from `lib/components.sh`
  - Installed via `ctdev install zsh`, symlinked to `~/.zfunc/_ctdev`

## [7.5.1] - 2026-02-14

### Fixed
- `ctdev configure --show` output now human-readable for both macOS and Linux Mint
  - Booleans display as `yes`/`no` instead of `true`/`false`/`0`/`1`
  - Time values display as `30 min`, `1 hr` instead of raw seconds with `uint32` prefixes
  - Mouse speed displays as `65%` instead of raw float
  - macOS Finder codes translated to names (`current folder`, `list`)
  - Quotes and type annotations stripped from all values

## [7.5.0] - 2026-02-14

### Added
- NVIDIA suspend stability in `ctdev configure linux-mint` (auto-detected when NVIDIA driver is loaded)
  - Adds `nvidia.NVreg_PreserveVideoMemoryAllocations=1` to GRUB kernel parameters
  - Enables `nvidia-suspend`, `nvidia-resume`, `nvidia-hibernate` systemd services
  - Warns if `amdgpu` module is loaded on dual-GPU systems
  - Supported in `--show`, `--reset`, and `--dry-run` modes
- Desktop freeze troubleshooting guide for NVIDIA + dual-GPU systems in TROUBLESHOOTING.md

## [7.4.0] - 2026-02-13

### Added
- `ctdev update --refresh-keys` to re-download expired APT repository GPG keys
  - Supports optional component filter: `ctdev update --refresh-keys docker gh`
  - Central key registry in `lib/keys.sh` for 6 components (docker, gh, 1password, terraform, vscode, dbeaver)
  - Respects `--dry-run` flag

## [7.3.0] - 2026-02-05

### Added
- `ctdev configure linux-mint` command for configuring Linux Mint (Cinnamon) system defaults
  - Power: performance profile, display/inactive sleep timers, lock on suspend
  - Screensaver: idle delay, lock settings
  - Keyboard: repeat rate, repeat delay, numlock state
  - Mouse: flat acceleration, speed, natural scroll
  - Sound: disable event sounds
  - Nemo: list view default
- `ctdev configure linux-mint --show` to display current settings
- `ctdev configure linux-mint --reset` to reset to Cinnamon defaults

## [7.2.0] - 2026-02-04

### Added
- Per-component `uninstall.sh` scripts for all 34 components
- `ctdev list` now shows unsupported components with "not supported" status
- `ctdev uninstall` summary now shows skipped (unsupported) components separately

### Changed
- `ctdev uninstall` refactored to dispatch to component scripts (464 → 140 lines)
- `ctdev upgrade` no longer auto-upgrades bun (no reliable update detection)
- `ctdev upgrade` now shows reminder: "To upgrade bun: bun upgrade"
- Uninstall scripts now use `apt remove` on Linux instead of just logging
- Updated README and CLAUDE.md with correct component count (34)

### Removed
- Monolithic uninstall functions from `cmds/uninstall.sh`

## [7.1.1] - 2026-02-04

### Fixed
- `ctdev configure git` no longer fails with "unbound variable" when called without arguments
- `ctdev info` now shows accurate disk usage on macOS APFS volumes (uses diskutil instead of df)

## [7.1.0] - 2026-02-03

### Added
- `ctdev configure git --show` to display current git user configuration (global and local)
- `ctdev configure macos --show` to display current macOS defaults (Dock, Finder, Keyboard, Dialogs, Security)

## [7.0.1] - 2026-02-03

### Fixed
- Git component now copies `.gitconfig` instead of symlinking (prevents global config changes from modifying repo)

## [7.0.0] - 2026-02-02

### Added
- `ctdev configure` command for post-install configuration
  - `ctdev configure git` - Configure git user name/email (global by default)
  - `ctdev configure git --local` - Configure git per-repository
  - `ctdev configure macos` - Apply macOS system defaults
- `bleachbit` component for Linux system cleaning (counterpart to `cleanmymac` on macOS)
- Hardware info section in `ctdev info`:
  - CPU model with core/thread count
  - Memory with snap-to-standard sizes (8/16/32/64/128 GB)
  - GPU with VRAM via nvidia-smi
  - Disk usage with GB/TB formatting and percentages
- Source guards to prevent multiple sourcing of bash library files
- Linux Mint support for TradingView installer

### Changed
- `ctdev list` now hides components not supported on current OS
- `ctdev upgrade` output cleaned up to only show packages with actual updates
- `ctdev upgrade` now shows version info: `package: 1.0.0 → 1.1.0`
- Exit code 2 now indicates skipped/unsupported installs (0=success, 1=error, 2=skipped)
- Consistent warning messages across all components for unsupported platforms
- Simplified claude-code detection (just checks for `claude` command)

### Removed
- `ctdev macos` command (replaced by `ctdev configure macos`)
- `.gitconfig.local.template` from git component (use `ctdev configure git --local` instead)
- `.gitignore` symlink from git component

## [6.1.0] - 2026-02-02

### Added
- Bun JavaScript runtime installer (macOS via Homebrew, Linux via official installer)

## [6.0.1] - 2026-01-31

### Fixed
- `devcontainer.sh` now sets `FORCE=true` to ensure full installation in containers with pre-installed zsh

## [6.0.0] - 2026-01-30

### Added
- `ctdev uninstall` with no args uninstalls all installed components (with confirmation)
- `--force` flag for `ctdev install` to reinstall components even if already installed
- Root `uninstall.sh` script for removing ctdev itself
- `uninstall_claude` function for claude component
- `list_installed_components` helper in lib/components.sh

### Changed
- All component install scripts now support FORCE environment variable
- All CLI tool scripts now support FORCE environment variable
- Simplified CLAUDE.md (205 → 89 lines)
- Simplified TROUBLESHOOTING.md (343 → 83 lines)
- Removed redundant comments across component scripts

## [5.11.1] - 2026-01-27

### Fixed
- `ctdev info` now checks `claude` CLI in CLI Tools section (moved from Node.js npm check)
- `ctdev info` now checks `path.zsh` in correct location (`~/.zsh/` instead of `~/.oh-my-zsh/custom/`)
- Added Claude component health check for config symlinks

### Changed
- Updated project CLAUDE.md with `claude` and `macos` components
- Added Claude Code troubleshooting section to TROUBLESHOOTING.md

## [5.11.0] - 2026-01-27

### Added
- New `claude` component for syncing Claude Code configuration across machines
- Symlinks `~/.claude/CLAUDE.md`, `settings.json`, and `settings.local.json` from dotfiles repo

## [5.10.2] - 2026-01-27

### Fixed
- Consolidated all PATH setup in `path.zsh` to fix "nodenv: command not found" error
- nodenv/rbenv bin directories are now added to PATH before running their init scripts

## [5.10.1] - 2026-01-26

### Fixed
- PATH ordering: source path.zsh last so `~/.local/bin` takes precedence over nodenv/rbenv shims

## [5.10.0] - 2026-01-26

### Added
- Claude Code CLI installation via native installer (`curl -fsSL https://claude.ai/install.sh | bash`)

### Removed
- Deprecated npm-based Claude Code installation from node component

## [5.9.1] - 2026-01-18

### Removed
- Linux Mint version upgrade detection from `ctdev update`

## [5.9.0] - 2026-01-16

### Added
- Linux Mint version upgrade detection in `ctdev update` (checks if mintupgrade is available)

### Changed
- Ghostty config: set default window size to 180x80, removed copy/paste keybinds (use defaults)

## [5.8.0] - 2026-01-12

### Added
- One-liner install script: `curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/install.sh | bash`

## [5.7.1] - 2026-01-12

### Fixed
- `ctdev gpu status` now correctly detects loaded NVIDIA driver (fixed SIGPIPE issue with pipefail)
- `ctdev gpu status` now correctly detects enrolled MOK keys (fixed fingerprint matching)

## [5.7.0] - 2026-01-12

### Added
- Ghostty terminal emulator installer (cross-platform)
- Ghostty configuration with `ctrl+c`/`ctrl+v` copy/paste keybindings
- `--force` flag to bypass already-installed checks
- Ghostty health check in `ctdev info`

### Changed
- Replaced iTerm2 with Ghostty as the default terminal app
- Updated Nerd Fonts instructions for Ghostty configuration

### Fixed
- Use official ghostty-ubuntu install script for Ubuntu/Debian
- Detect Ubuntu base codename for Linux Mint in Ghostty install

## [5.6.0] - 2026-01-03

### Added
- `ctdev update` command for updating system packages and installed components
- Update detection in `ctdev install` - checks git repos for available updates
- Interactive prompt when updates are available during install

### Changed
- `ctdev install` now only installs new components (no longer auto-updates)
- When already-installed components have updates, user is prompted to update now or defer
- `--skip-system` flag moved to `ctdev update` (deprecated in install)

## [5.5.5] - 2025-12-28

### Fixed
- Silence ShellCheck SC2140 false positive for git URL rewrite syntax in `lib/github.sh`
- Updated CLAUDE.md to reflect current `lib/` directory structure (7 modular files)
- Removed hardcoded version from CLAUDE.md overview

### Changed
- Simplified README.md DevContainers section

## [5.5.4] - 2025-12-28

### Fixed
- `ctdev info` now filters all `/boot*` mounts on Linux (was only filtering `/boot/efi`)
- Simplified disk mount labels (changed "Root (/)" to just "/")

## [5.5.3] - 2025-12-27

### Fixed
- `devcontainer.sh` now runs install script directly (bypasses partial install detection)
- Git clone/pull in devcontainers now forces HTTPS to avoid SSH key issues with URL rewrites

## [5.5.2] - 2025-12-27

### Fixed
- `devcontainer.sh` now skips system package updates that require sudo
- `maybe_sudo` gracefully handles containers with "no new privileges" flag
- zsh install skips `chsh` in devcontainers (shell managed by container)
- Clear error message when zsh not installed in devcontainer

### Added
- `is_devcontainer()` helper detects VS Code devcontainers, Codespaces, and custom containers

## [5.5.1] - 2025-12-27

### Fixed
- `ctdev info` now filters out system volumes on macOS (was showing 10+ unnecessary mounts)
- `ctdev info` now filters out snap/loop/docker mounts on Linux
- `ctdev info` now shows memory used/available on macOS (was only showing total)
- `ctdev info` network section now correctly displays active interfaces on macOS

### Added
- `.zshrc` now sources `~/.secrets` for sensitive credentials (keeps secrets out of git)

## [5.5.0] - 2025-12-26

### Changed
- Merged `ctdev update` into `ctdev install` - single command now handles both installation and updates
- Components not installed will be installed; components already installed will be updated
- System package updates (apt/brew/dnf/pacman) run by default with `--skip-system` flag to skip
- All dry-run messages now visible without `--verbose` flag (use `log_info` instead of `log_debug`)
- Standardized `[DRY-RUN]` prefix (with hyphen) across all scripts

### Added
- `--skip-system` flag for `ctdev install` to skip system package updates
- ShellCheck linting job in GitHub Actions workflow

### Removed
- `ctdev update` command (functionality merged into `ctdev install`)
- `cmds/update.sh` file

## [5.4.0] - 2025-12-26

### Added
- Installation marker files (`~/.config/ctdev/<component>.installed`) for reliable component detection
- TROUBLESHOOTING.md documentation for common issues and solutions
- Enhanced hardware info in `ctdev info`:
  - GPU details via nvidia-smi (model, memory, power, temperature, driver, CUDA version)
  - All mounted disks with usage statistics
  - Network interfaces with IP, MAC address, and state
- Checksum verification for CLI tool downloads (git-spice, doctl, helm, sops)
- Input validation for git user configuration
- ShellCheck static analysis tool to CLI component

### Changed
- Refactored `lib/utils.sh` into modular files:
  - `lib/logging.sh` - Color configuration and log functions
  - `lib/platform.sh` - OS/architecture detection, package management
  - `lib/packages.sh` - Dependency management helpers
  - `lib/github.sh` - GitHub API, checksums, git repository functions
- Memory display now uses binary units (GB = 1024³) for accurate reporting
- Hardware info sections now use consistent indented formatting

### Fixed
- Memory calculation now correctly shows binary units (was showing ~64.8 GB for 64 GB RAM)
- Added 30-second timeout on interactive prompts (prevents CI/CD hangs)
- Non-interactive environments skip update prompts gracefully
- All scripts now pass ShellCheck static analysis

## [5.3.2] - 2025-12-23

### Fixed
- git-spice installer now correctly detects git-spice vs Ghostscript (both use `gs` command)

## [5.3.1] - 2025-12-23

### Added
- git-spice CLI tool for stacked branches workflow (macOS via Homebrew, Linux via GitHub releases)

## [5.3.0] - 2025-12-23

### Changed
- Merged `ctdev doctor` into `ctdev info` - single command now shows system info and health checks
- Moved tmux from `apps` to `cli` component (where it belongs as a CLI tool)

### Removed
- `ctdev doctor` command (functionality merged into `ctdev info`)
- Cursor AI editor app (removed from apps component)

### Fixed
- tmux no longer shows redundant "installation complete" message when already installed

## [5.2.0] - 2025-12-23

### Added
- Cursor AI editor installation for macOS (Homebrew) and Linux (AppImage)
- Comprehensive app checks in `ctdev doctor` for all installed apps (Cursor, Claude, 1Password, DBeaver, TradingView, Linear, CleanMyMac, Logi Options+, tmux)
- CLI tool checks for age, sops, terraform, docker in `ctdev doctor`
- Editor version display (code, cursor) in `ctdev info`
- CLI tool version display for age, sops, terraform in `ctdev info`

## [5.1.3] - 2025-12-19

### Fixed
- Logi Options+ app detection path (bundle name is `logioptionsplus.app`)
- TradingView installer now downloads directly from official URLs instead of using Homebrew cask

### Changed
- TradingView installer supports macOS (DMG) and Debian/Ubuntu (deb) with proper installation flows

## [5.1.2] - 2025-12-14

### Added
- `ctdev doctor` now checks apps, fonts, and macOS defaults components
- `log_check_pass` and `log_check_fail` helpers in `lib/utils.sh` for consistent status output

### Changed
- Unified logging across `ctdev info` and `ctdev doctor` using shared check helpers
- Consistent colored checkmark output (green for pass, yellow for fail)

## [5.1.1] - 2025-12-14

### Changed
- `ctdev update` now checks if components are installed before updating
- Shows helpful skip message with install instructions for non-installed components
- Components without update support (git, fonts, apps, macos) show "No update needed" message

### Fixed
- Variable scope bug in `lib/components.sh` that caused incorrect component names in output

## [5.1.0] - 2025-12-14

### Added
- New `macos` component for configuring macOS system defaults (Dock, Finder, keyboard, dialogs, security)
- GitHub CLI extensions update in `ctdev update cli`

## [5.0.6] - 2025-12-12

### Added
- `devcontainer.sh` for VS Code dotfiles integration (supports `dotfiles.installCommand` setting)

## [5.0.5] - 2025-12-11

### Fixed
- Nerd Fonts installation on macOS with Bash 3.2 (replaced `${VAR,,}` with `tr` for lowercase conversion)
- Added detection for manually installed fonts to avoid Homebrew cask conflicts

### Added
- Terminal configuration instructions printed after fonts installation (iTerm2, Terminal.app, VS Code)

## [5.0.4] - 2025-12-11

### Added
- Auto-update check: ctdev now checks if the repo is behind origin before running any command and prompts to pull the latest changes

## [5.0.3] - 2025-12-10

### Added
- DBeaver Community Edition installer with macOS (Homebrew), Debian/Ubuntu (apt), Fedora (dnf), and Arch (pacman) support

## [5.0.2] - 2025-12-10

### Fixed
- Fonts installation failing with SIGPIPE (exit 141) due to find | head pipeline

## [5.0.1] - 2025-12-10

### Fixed
- gh.sh and terraform.sh no longer require lsb_release
- Git component verification in CI uses correct file paths
- DevContainers documentation expanded with examples

### Changed
- GitHub Actions workflow updated for ctdev CLI
- Removed ShellCheck (false positives on zsh config files)

## [5.0.0] - 2025-12-10

### Added
- `ctdev` unified CLI with subcommands (install, update, doctor, list, info, uninstall, setup)
- `ctdev setup` command to symlink ctdev to ~/.local/bin
- `ctdev doctor` command to check installation health
- `ctdev list` command to show available components
- Modular CLI tools installation (individual scripts per tool)
- Git user config with interactive prompts and CLI arguments
- Support for Homebrew-installed nodenv/rbenv (skips git updates)
- Nerd Fonts installation via Homebrew casks on macOS

### Changed
- Reorganized directory structure: components/, cmds/, lib/
- Moved all component installers to components/ directory
- CLI tools now installed individually (age, btop, docker, doctl, gh, helm, jq, kubectl, sops, terraform)
- Node.js uses `brew install nodenv` on macOS, git clone on Linux
- Ruby uses `brew install rbenv` on macOS, git clone on Linux
- Fonts use Homebrew casks on macOS, GitHub releases on Linux
- PATH configuration prioritizes ~/.local/bin
- Symlink resolution in ctdev supports being called via symlink

### Removed
- Old top-level install.sh, update.sh, report.sh, devcontainer.sh
- Old directory structure (apps/, cli/, fonts/, git/, node/, ruby/, zsh/ at root)
- scripts/utils.sh (moved to lib/utils.sh)

## [4.0.0] - 2025-12-10

### Added
- Initial ctdev CLI structure
- Component-based architecture

## [3.0.0] - 2024

### Added
- Cross-platform support (Ubuntu, Debian, Fedora, Arch, macOS, FreeBSD)
- Structured logging functions
- Dry-run mode
- maybe_sudo for Docker compatibility

## [2.0.0] - 2024

### Added
- Modular component installers
- Shared utilities (scripts/utils.sh)

## [1.0.0] - 2024

### Added
- Initial dotfiles implementation
