# CLAUDE.md

Instructions for Claude Code when working with this repository.

## ctdev CLI

```bash
ctdev install <component...>    # Install specific components
ctdev uninstall <component...>  # Remove specific components
ctdev update [-y]               # Update system packages and components
ctdev update --check                   # List available updates without installing
ctdev update --refresh-keys            # Refresh APT GPG keys before updating
ctdev info                             # Show system information
ctdev configure git             # Configure git user
ctdev gpu info                  # Show GPU hardware info and signing status
ctdev gpu setup                 # Configure MOK signing for NVIDIA drivers
ctdev setup                     # Run full fresh-install setup
ctdev setup --show              # Show current system configuration
ctdev setup --reset             # Reset system configuration to defaults
ctdev cleanup                   # Run all cleanup tasks (with prompts)
```

**Flags:** `--help`, `--verbose`, `--dry-run`, `--force`, `--version`, `--refresh-keys`

## Components

35 components:

1password, age, btop, bun, chatgpt, chrome, cleanmymac, claude-code, claude-desktop, codex, dbeaver, docker, doctl, earlyoom, fonts, gh, ghostty, git, git-spice, helm, jq, kubectl, linear, logi-options, node, ruby, shellcheck, slack, solaar, sops, tailscale, terraform, tmux, vscode, zsh

## Directory structure

```
lib/           Shared utilities (logging, platform detection, packages)
cmds/          CLI command implementations
components/    Installable components (one dir per component with install.sh and uninstall.sh)
```

## Key utilities

- `log_info`, `log_success`, `log_warning`, `log_error`, `log_step`
- `detect_os`, `detect_arch`, `get_package_manager`, `maybe_sudo`
- `safe_symlink`, `run_cmd` (respects DRY_RUN)
- `install_package`, `ensure_git_repo`

## Conventions

- All scripts use `set -euo pipefail`
- Scripts are idempotent
- Use `maybe_sudo` instead of `sudo` for Docker compatibility
- Use `run_cmd` to respect DRY_RUN
- Symlink configs instead of copying

## Adding a new component

Create `components/<name>/install.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOTFILES_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$DOTFILES_ROOT/lib/utils.sh"

log_info "Installing <name>"

if [[ "${FORCE:-false}" != "true" ]] && command -v <name> >/dev/null 2>&1; then
    log_info "<name> already installed"
    exit 0
fi

OS=$(detect_os)
if [[ "$OS" == "macos" ]]; then
    brew install <name>
else
    install_package <name>
fi
```

Create `components/<name>/uninstall.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOTFILES_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$DOTFILES_ROOT/lib/utils.sh"

OS=$(detect_os)
PM=$(get_package_manager)

log_info "Uninstalling <name>..."

if [[ "$OS" == "macos" ]]; then
    run_cmd brew uninstall <name> || true
elif [[ "$PM" == "apt" ]]; then
    run_cmd maybe_sudo apt remove -y <name> || true
fi
```

Exit codes for uninstall scripts:
- `0`: success
- `2`: unsupported on this OS (component will be skipped)

Then add to `lib/components.sh` COMPONENTS array.

## Git commits

Single-line messages only. No footers.

## Releases

1. Commit changes
2. Update CHANGELOG.md
3. Bump VERSION
4. Commit: `docs: update for vX.Y.Z`
5. Tag: `git tag vX.Y.Z`
6. Push: `git push && git push --tags`
7. Create release: `gh release create vX.Y.Z`
