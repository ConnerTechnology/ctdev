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
ctdev configure git             # Configure git user and SSH signing key
ctdev configure aws             # Configure AWS profile
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
ctdev/                 Go module root
  cmd/                 Cobra command handlers
  component/           Component registry, installers, and embedded config files
    configs/           Config files deployed by installers (go:embed)
  gpu/                 GPU/NVIDIA signing management
  platform/            OS/arch detection
  setup/               System settings (Linux dconf/GRUB, macOS defaults)
    configs/           Setup config files (go:embed)
  state/               Install markers and XDG state
  sysutil/             System utilities (packages, downloads, deploy, exec)
  tui/                 Bubble Tea UI models
  internal/shell/      Shell execution wrapper
```

## Conventions

- Use `inst` as the Go receiver name (not single letters)
- Use `sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}` for all sysutil calls
- Config files are embedded via `all:configs` in `go:embed` (the `all:` prefix is required to include dot-files like `.zshrc`)
- Deploy configs with `sysutil.DeployFileFromFS` — it handles backup-on-diff and replaces dangling symlinks
- Components with configs use Phase 1/Phase 2: Phase 1 installs the binary (skip if exists), Phase 2 always deploys configs
- Return `ErrUnsupportedOS` for unsupported platforms
- All installers accept `ctx context.Context` as first parameter

## Adding a new component

Create `ctdev/component/<name>.go`:

```go
package component

import (
    "context"
    "fmt"

    "github.com/ConnerTechnology/dotfiles/ctdev/platform"
    "github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func nameInstall(ctx context.Context, opts ExecOpts) error {
    p := platform.Detect()
    o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

    if !opts.Force && sysutil.CommandExists("name") {
        fmt.Fprintln(opts.Stdout, "name already installed")
        return nil
    }

    fmt.Fprintln(opts.Stdout, "Installing name...")

    switch p.PackageManager {
    case "brew":
        return sysutil.InstallPackage(o, "name")
    case "apt":
        return sysutil.InstallPackage(o, "name")
    default:
        return fmt.Errorf("name not supported for: %s", p.PackageManager)
    }
}

func nameUninstall(ctx context.Context, opts ExecOpts) error {
    o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
    fmt.Fprintln(opts.Stdout, "Removing name...")
    return sysutil.RemovePackage(o, "name")
}
```

Then add to `ctdev/component/registry.go`:

```go
{Name: "name", Description: "Description", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: nameInstall, GoUninstall: nameUninstall, Tags: []string{"tag1"}},
```

For simple package-manager installs, use the helper:
```go
GoInstall: SimplePackageInstaller("name"), GoUninstall: SimplePackageUninstaller("name")
```

If the component has config files, place them in `ctdev/component/configs/<name>/` and use Phase 1/Phase 2:
```go
// Phase 1: Install binary (skip if exists unless --force)
if opts.Force || !sysutil.CommandExists("name") {
    // ... install package ...
}

// Phase 2: Always deploy configs (keeps dotfiles in sync)
sysutil.DeployFileFromFS(Configs, "configs/<name>/file", dest)
```

## Git commits

Single-line messages only. No footers.

## Releases

1. Commit changes
2. Update CHANGELOG.md
3. Bump VERSION
4. Commit: `docs: update for vX.Y.Z`
5. Tag: `git tag vX.Y.Z`
6. Push: `git push && git push --tags`
7. CI builds and creates the GitHub Release automatically via `.github/workflows/release.yml`
