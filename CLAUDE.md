# CLAUDE.md

Instructions for Claude Code when working with this repository.

## ctdev CLI

```bash
ctdev install <component...>    # Install specific components
ctdev uninstall <component...>  # Remove specific components
ctdev update [-y]               # Update system packages and components
ctdev update --check            # List available updates without installing
ctdev update --refresh-keys     # Refresh APT GPG keys before updating
ctdev info                      # Show system information
ctdev configure                 # Walk through all system configuration
ctdev configure <category>      # Configure a specific category
ctdev configure --show          # Show current system configuration
ctdev configure git             # Configure git user and SSH signing key
ctdev configure aws             # Configure AWS profile
ctdev configure remote          # Configure remote access (SSH/Mosh/UFW/tunnel)
ctdev configure remote --batch  # Apply remote-access defaults non-interactively
ctdev gpu info                  # Show GPU hardware info and signing status
ctdev gpu setup                 # Configure MOK signing for NVIDIA drivers
ctdev cleanup                   # Run all cleanup tasks (with prompts)
ctdev verify                    # Verify the bootstrap installation
```

## Bootstrap

`bootstrap.sh` (repo root) sets up a fresh Linux Mint machine in one command. It's a
thin orchestrator: base apt packages → install/build the `ctdev` binary →
`ctdev install <components>` → `ctdev configure remote --batch`. When run from a
clone with Go present it builds `ctdev` from source; otherwise it downloads the
released binary. Idempotent. See README "Fresh Machine Setup".

**Flags:** `--help`, `--verbose`, `--dry-run`, `--force`, `--version`, `--refresh-keys`

## Components

37 components:

1password, age, btop, bun, chatgpt, chrome, cleanmymac, claude-code, claude-desktop, codex, dbeaver, devcontainer, docker, doctl, earlyoom, fonts, gh, ghostty, git, git-spice, go, helm, jq, kubectl, linear, logi-options, node, ruby, shellcheck, slack, solaar, sops, tailscale, terraform, tmux, vscode, zsh

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
```

## Conventions

- Use `inst` as the Go receiver name (not single letters)
- Every `sysutil` shell-out takes `(ctx context.Context, o sysutil.Opts, ...)` — thread `ctx` from the caller so Ctrl-C cancels in-flight work
- Build `sysutil.Opts` via `execOpts(opts)` — one-liner that copies Stdout and DryRun
- Use `alreadyInstalled("<name>")` for install-time "already present?" checks (delegates to the registry's `IsInstalled()`) instead of re-checking paths/commands inline
- For unsupported package-manager branches, return `unsupportedPMError("<component>", p.PackageManager)` so the executor's `errors.Is(err, ErrUnsupportedOS)` maps it to Skipped
- Config files are embedded via `all:configs` in `go:embed` (the `all:` prefix is required to include dot-files like `.zshrc`)
- Deploy configs with `sysutil.DeployFileFromFS` — it handles backup-on-diff and replaces dangling symlinks
- Components with configs use Phase 1/Phase 2: Phase 1 installs the binary (skip if exists), Phase 2 always deploys configs
- For `.deb` installs on apt, use `installDebWithDepFix(ctx, o, debPath, pkgName)` — runs dpkg, recovers with `apt-get -f`, and verifies via `IsPackageInstalled` so corrupt/wrong-arch `.deb`s don't silently report success
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
    o := execOpts(opts)

    if !opts.Force && alreadyInstalled("name") {
        fmt.Fprintln(opts.Stdout, "name already installed")
        return nil
    }

    fmt.Fprintln(opts.Stdout, "Installing name...")

    switch p.PackageManager {
    case "brew", "apt":
        return sysutil.InstallPackage(ctx, o, "name")
    default:
        return unsupportedPMError("name", p.PackageManager)
    }
}

func nameUninstall(ctx context.Context, opts ExecOpts) error {
    o := execOpts(opts)
    fmt.Fprintln(opts.Stdout, "Removing name...")
    return sysutil.RemovePackage(ctx, o, "name")
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

If the package is installed via a downloaded `.deb`:
```go
tmp, err := os.CreateTemp("", "name-*.deb")
if err != nil { return err }
defer os.Remove(tmp.Name())
tmp.Close()
if err := sysutil.DownloadFile(ctx, debURL, tmp.Name()); err != nil {
    return fmt.Errorf("download name: %w", err)
}
return installDebWithDepFix(ctx, o, tmp.Name(), "name-pkg")
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
