# CLAUDE.md

Instructions for Claude Code when working with this repository.

## ctdev CLI

`ctdev install <component>` installs the component (pulling in its `Dependencies`
first) and then runs its configuration step if it has one — a `configure <name>`
category (e.g. `pihole`) or the `caddy` wizard. Re-running `install` on something
already installed says so and jumps straight to configuration. `ctdev configure
<name>` configures without installing. (Both skipped in `--batch`/`--dry-run`.)

```bash
ctdev install <component...>    # Install components (then configure them)
ctdev uninstall <component...>  # Remove specific components
ctdev update [-y]               # Update system packages, components, and Docker stacks
ctdev update --check            # List available updates without installing
ctdev update --refresh-keys     # Refresh APT GPG keys before updating
ctdev info                      # Show system information
ctdev configure                 # Walk through all system configuration
ctdev configure <category>      # Configure a specific category
ctdev configure --show          # Show current system configuration
ctdev configure git             # Configure git user and SSH signing key
ctdev configure aws             # Configure AWS profile
ctdev configure ssh             # SSH server + key-based auth hardening
ctdev configure ufw             # UFW firewall (SSH/Mosh from private ranges)
ctdev configure sleep           # Never-suspend (mask sleep targets)
ctdev configure locale          # UTF-8 locale (for Mosh)
ctdev configure linger          # User-service lingering
ctdev configure tunnel          # VS Code tunnel service
ctdev configure pihole          # Pi-hole DNS (upstreams, listening mode, blocking)
ctdev configure caddy           # Caddy reverse proxy (domain, ACME email, CF token)
ctdev configure gpu             # NVIDIA driver/MOK signing + GPU settings (--show, --recover)
ctdev configure <category> --batch  # Apply a category's defaults non-interactively
ctdev backup [service...]       # Export service config to version control (default: all)
ctdev restore [service...]      # Re-apply version-controlled service config (inverse of backup)
ctdev backup now                # Run a restic data snapshot now (restic-backup.sh)
ctdev backup snapshots [b2|local]  # List restic snapshots
ctdev cleanup                   # Run all cleanup tasks (with prompts)
ctdev verify                    # Verify the bootstrap installation
```

## Install

`install.sh` (repo root) installs just the `ctdev` binary (downloads the latest
release, or builds from source in a clone when Go is present, verifying against
`SHA256SUMS`). There is no all-in-one machine bootstrap — compose a machine from
individual `ctdev install <component>` and `ctdev configure <category>` calls.
See README "Fresh Machine Setup".

**Flags:** `--help`, `--verbose`, `--dry-run`, `--force`, `--version`, `--refresh-keys`

## Components

43 components:

1password, age, beszel, btop, bun, caddy, chatgpt, chrome, cleanmymac, claude-code, claude-desktop, codex, dbeaver, devcontainer, docker, doctl, earlyoom, fonts, gh, ghostty, git, git-spice, go, helm, jq, kubectl, linear, logi-options, node, nomachine, pihole, portainer, restic, ruby, shellcheck, slack, solaar, sops, tailscale, terraform, tmux, vscode, zsh

## Homelab / Pi-hole nodes

ctdev has no "homelab mode" — you compose a node from individual components and
`configure` categories, the same way you would a desktop. For a Raspberry Pi
running Pi-hole behind a Caddy reverse proxy:

- `ctdev install pihole` — Pi-hole as a Docker container (official image, host
  networking) deployed to `~/pihole/`; config/lists persist in `./etc-pihole`.
- `ctdev configure pihole` — upstream resolvers, listening mode, blocking on/off
  (runs against the container via `docker exec`, or a native install if present).
  The upstream choices include "Local recursive (Unbound)" → `127.0.0.1#5335`,
  served by the `unbound` sidecar in the Pi-hole stack (recursive + DNSSEC).
- `ctdev backup pihole` / `ctdev restore pihole` — version-control the lists. Backup
  snapshots adlists, allow/deny lists, and regex filters to plain-text files under
  `component/configs/pihole/` (embedded; diffable in git); restore applies them and
  rebuilds gravity (additive). Custom DNS records are SOPS-encrypted to
  `component/configs/pihole/hosts/<node>.sops.json` (rule in `.sops.yaml`). `ctdev
  backup` with no service backs up every backup-capable service (just Pi-hole today).
- `ctdev install caddy` — deploys the Caddy stack from `component/configs/caddy/`
  to `~/caddy/` and runs `docker compose up`. The stack is generic; the domain,
  ACME email, and Cloudflare token come from `~/caddy/.env`.
- `ctdev configure caddy` — writes `~/caddy/.env` (domain / ACME email / CF token)
  and, when Pi-hole is present, wires it (frees port 443, points `*.<domain>` at
  this node's Tailscale IP). Run this before `ctdev install caddy`.
- `ctdev install portainer` — Portainer CE Docker management web UI as a
  container deployed to `~/portainer/` (users/settings persist in the
  portainer_data volume). The Caddy stack reverse-proxies it at
  `https://portainer.<domain>` (port 9000); it is also reachable directly at
  `https://<node>:9443`. No `configure` step — create the admin user on first
  web login. The Docker socket is mounted, so keep it off any public network.
- `ctdev install beszel` — Beszel server/container monitoring (hub + agent) as
  containers deployed to `~/beszel/`. The hub (web UI + data, port 8090) is
  reverse-proxied by Caddy at `https://beszel.<domain>` via a `@beszel` route;
  the agent monitors this host over a shared unix socket. The install brings the
  hub up first; create the admin user, click "Add System", put the issued
  KEY/TOKEN in `~/beszel/.env`, then re-run `ctdev install beszel` to start the
  agent. Keep it off any public network (Tailscale only). Note: per-container
  memory stats need the kernel memory cgroup, which is off on some customized
  Pi boot images (`cgroup_disable=memory` baked into the device-tree bootargs).
- `ctdev install restic` — restic backups with a daily systemd timer. Installs
  restic and deploys `/usr/local/bin/restic-backup.sh` (snapshots the stack dirs
  under `$HOME` + Docker named-volume data dirs to an offsite B2 repo and a local
  USB repo at `/mnt/backup` when mounted, then prunes 7d/4w/6m),
  `/usr/local/bin/restic-restore.sh` (a restore helper), and
  `restic-backup.{service,timer}`. Repo locations, B2 creds, and the repo
  password live in `/etc/restic/` (root-only, **never committed**); the timer is
  enabled only once that config exists. Outside the timer, `ctdev backup now`
  runs a snapshot immediately and `ctdev backup snapshots [b2|local]` lists them
  (both shell out to these scripts via sudo). **Full restore runbook: `RECOVERY.md`.**

Keeping a node current: `ctdev update` refreshes these compose stacks along with
system packages. It checks each managed stack (pihole, caddy, beszel, portainer)
for a newer image by digest — without pulling — and updates the ones you select
(`docker compose pull && up -d`, or a `build --pull` rebuild for the locally-built
caddy image). `ctdev update --check` lists what's available read-only.

Note: a Pi-hole/DNS host should usually **not** run `ctdev configure ufw` — UFW's
default-deny blocks DNS (53) and the proxy (80/443) unless you open them first.

Per-node secrets are SOPS-encrypted under `component/configs/<svc>/hosts/<node>.sops.env`
(age recipient in `.sops.yaml`): **caddy** (CF token), **restic** (repo password +
B2 keys → `/etc/restic/restic.env`), **beszel** (agent KEY/TOKEN), **pihole**
(admin password). Decrypt with `sops -d <file>` into the target path. The age
private key (`~/.config/sops/age/keys.txt`) is the only thing that can decrypt
them — keep it in 1Password; it is never committed or backed up.
**Never commit a plaintext host config or an age private key.**
See `SECRETS.md` (encryption workflow) and `RECOVERY.md` (disaster recovery).

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
