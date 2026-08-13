# CLAUDE.md

Instructions for Claude Code when working with this repository.

## ctdev CLI

`ctdev install <component>` installs the component (pulling in its `Dependencies`
first) and then runs its configuration step if it has one — a `configure <name>`
category (e.g. `pihole`) or the `caddy` wizard. Re-running `install` on something
already installed says so and jumps straight to configuration. `ctdev configure
<name>` configures without installing. (Both skipped in `--batch`/`--dry-run`.)

```bash
ctdev apply [profile]           # Apply a machine profile; no args lists profiles
ctdev diff <profile>            # Show drift from a profile (non-zero exit on drift)
ctdev install <component...>    # Install components (then configure them)
ctdev uninstall <component...>  # Remove specific components
ctdev update [-y]               # Update system packages, components, and Docker stacks
ctdev update --check            # List available updates without installing
ctdev update --refresh-keys     # Refresh APT GPG keys before updating
ctdev info                      # Inventory: specs, kernel, uptime, profile, drives, usage, installed components
ctdev status                    # Needs attention: reboot/failed units, disk pressure/SMART, wedged apt, containers, backups, updates
ctdev configure                 # Full-screen settings browser (all categories)
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
ctdev configure autoupdate      # Automatic security updates + apt-daily job timeout
ctdev configure macos           # macOS defaults (Dock/Finder/keyboard) — macOS only
ctdev configure pihole          # Pi-hole DNS (upstreams, listening mode, blocking)
ctdev configure caddy           # Caddy reverse proxy (domain, ACME email, CF token)
ctdev configure restic          # restic backups (repo, credentials, paths) — --show
ctdev configure gpu             # NVIDIA driver/MOK signing + GPU settings (--show, --recover)
ctdev configure <category> --batch  # Apply a category's defaults non-interactively
ctdev backup now                # Run a restic snapshot of this machine now
ctdev backup test               # Check backups are set up correctly (config, connection, paths)
ctdev backup disable            # Pause scheduled backups (config + snapshots kept)
ctdev backup enable             # Resume scheduled backups
ctdev backup snapshots [primary|local]  # List this machine's restic snapshots
ctdev backup paths              # Pick what to back up in a local web UI
ctdev restore ls|files|in-place|check   # Inspect/restore from restic (see RECOVERY.md)
ctdev cleanup                   # Reclaim disk space (scan, pick tasks, clean; --dry-run to preview)
ctdev verify                    # Verify the bootstrap installation
ctdev doctor                    # Diagnose any machine: network, hardware, OS, security
ctdev doctor --deep             # + vendor APIs (UniFi/Synology/Proxmox), gear fingerprint
ctdev doctor --network          # network and internet checks only
ctdev doctor --report [path]    # also write a shareable Markdown report
ctdev doctor --redact           # mask SSID/MAC/public IP so the report can be shared
ctdev doctor --strict           # exit non-zero on failure (for cron)
ctdev doctor --no-integrations  # never call a vendor API, even with credentials
```

## Install

`install.sh` (repo root) installs just the `ctdev` binary (downloads the latest
release and verifies it against `SHA256SUMS`; there is no source-build path).
`install.sh --doctor` instead runs `ctdev doctor` from a temp directory and
deletes it — nothing installed, no PATH change, no sudo. `install.ps1` is the
Windows equivalent. There is no all-in-one machine bootstrap — compose a machine from
individual `ctdev install <component>` and `ctdev configure <category>` calls.
See README "Fresh Machine Setup".

**Flags:** `--help`, `--verbose`, `--dry-run`, `--force`, `--version`, `--refresh-keys`

## Components

50 components:

1password, age, bat, beszel, btop, bun, caddy, chrome, cleanmymac, claude-code, claude-desktop, dbeaver, devcontainer, direnv, docker, doctl, earlyoom, fd, fonts, fzf, gh, git, git-spice, go, helm, jq, kubectl, lazygit, linear, logi-options, mosh, node, nomachine, pihole, portainer, restic, ripgrep, ruby, shellcheck, slack, smartmontools, solaar, sops, syncthing, tailscale, terraform, tmux, vscode, zoxide, zsh

## Profiles

Machine profiles are declarative TOML files (components + configure categories
applied at recommended values). Built-ins — `pihole-node`, `dev-workstation`,
`family-desktop` — are **embedded in the binary** (`ctdev/profile/profiles/`),
so a fresh machine can `ctdev apply pihole-node` with nothing but the installed
binary. Local files in `~/.config/ctdev/profiles/<name>.toml` add profiles or
override built-ins by name. `apply` shows the plan and confirms, installs
(deps resolve), batch-configures, then prints the profile's `notes` (its
next-steps runbook — interactive wizards like restic/caddy are never run by
apply, and the `gpu` category is rejected in profiles because MOK signing is
interactive). `diff` exits non-zero on drift, so it works as a cron check.

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
  Pi-hole's lists, settings, and gravity.db persist in `~/pihole/etc-pihole`, which
  restic backs up — there is no separate per-service config export. Set the admin
  password with `docker exec -it pihole pihole setpassword`.
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
  restic and deploys `/usr/local/bin/restic-backup.sh` (snapshots the paths listed
  in `/etc/restic/backup-paths` to the configured repo, optionally a second repo,
  then prunes 7d/4w/6m), `/usr/local/bin/restic-restore.sh` (a restore helper), and
  `restic-backup.{service,timer}`. Then run **`ctdev configure restic`** — it prompts
  for the repository (any backend: B2/S3/SFTP/local), credentials, and password,
  writes `/etc/restic/restic.env` (root-only, **never committed**), seeds default
  exclude patterns, runs `restic init`, and enables the timer. Backups are **opt-in**:
  nothing is snapshotted until you choose what to include with **`ctdev backup paths`** —
  a local web UI (localhost only) that browses the filesystem with folder sizes and
  include/exclude buttons, writing `/etc/restic/backup-paths` and `/etc/restic/backup-excludes`.
  (An `include` is a tree to back up; `exclude` globs/paths carve junk out of an included
  tree — e.g. include `~/Repos`, exclude `**/node_modules`.) Outside the timer,
  `ctdev backup now` snapshots immediately and
  `ctdev backup snapshots [primary|local]` lists this machine's snapshots (tagged by
  hostname); `ctdev restore …` inspects/restores. **Full restore runbook: `RECOVERY.md`.**

Keeping a node current: `ctdev update` refreshes these compose stacks along with
system packages. It checks each managed stack (pihole, caddy, beszel, portainer)
for a newer image by digest — without pulling — and updates the ones you select
(`docker compose pull && up -d`, or a `build --pull` rebuild for the locally-built
caddy image). `ctdev update --check` lists what's available read-only.

Note: a Pi-hole/DNS host should usually **not** run `ctdev configure ufw` — UFW's
default-deny blocks DNS (53) and the proxy (80/443) unless you open them first.

Secrets are **never stored in the repo**. Each is entered at `configure` time and
stored only on the host that needs it; if lost, just reconfigure:
- **restic** (repo + credentials + password) → `/etc/restic/restic.env`, written by
  `ctdev configure restic`. restic itself can't restore its own credentials, so they
  live only here — keep a copy in your password manager.
- **caddy** (domain/ACME email/CF token) → `~/caddy/.env`, written by `ctdev configure caddy`.
- **pihole** (admin password) → set with `docker exec -it pihole pihole setpassword`.
- **beszel** (agent KEY/TOKEN) → `~/beszel/.env`, pasted from the hub's "Add System" dialog.

restic snapshots the rendered `~/<svc>/.env` files (they're under the backed-up paths),
so a restore brings them back; a brand-new node re-enters them from your password manager.
**Never commit a secret.** See `RECOVERY.md` (disaster recovery).

## ctdev doctor

`ctdev doctor` is the one command that assumes nothing about the machine — it is
built for diagnosing hardware you did not set up. Every check is read-only, root
is never required (checks needing it report Skipped and say so), and no data
leaves the machine beyond the diagnostic probes themselves.

- **Checks** live in `ctdev/diagnose/` as a catalog of struct literals with
  closures, built as a function of `platform.Info` + `Facts` — the same shape as
  `cleanup.Task`. Gate at construction time so a wired machine has no Wi-Fi rows
  rather than a column of "n/a".
- **`Check.Network`** marks a check that does network I/O; `ctdev status` reuses
  the same catalog filtered to `!Network && !Deep`, which is what keeps its
  "no network calls" contract honest. **`Check.Deep`** marks slow or third-party
  probes that only run under `--deep`.
- **`Diagnose(results, facts) []Finding`** is the correlation engine: a pure
  function that turns combinations into verdicts. Network rules are layered and
  first-match-wins, because a network fault has one root cause and everything
  downstream is noise. Hardware verdicts accumulate.
- **Vendor integrations** (`integration_*.go`) are read-only *by construction* —
  the `Provider` interface has no action method. They live in package `diagnose`
  rather than a subpackage to avoid an import cycle.
- **Credentials** come from flags, environment (`CTDEV_UNIFI_API_KEY`,
  `CTDEV_SYNOLOGY_PASSWORD`, `CTDEV_PROXMOX_SECRET`, …), or a one-shot prompt
  held in memory. **Nothing is ever written to the machine being diagnosed.**
  They are only ever sent to private addresses, and never appear in a report —
  `visibleData` drops secret-looking keys at the render boundary.

## Directory structure

```
ctdev/                 Go module root
  cmd/                 Cobra command handlers
  cleanup/             Disk-reclaim task catalog (Linux + macOS), scan/run
  diagnose/            ctdev doctor: check catalog, correlation engine, vendor integrations
  component/           Component registry, installers, and embedded config files
    configs/           Config files deployed by installers (go:embed)
  gpu/                 GPU/NVIDIA signing management
  platform/            OS/arch detection
  profile/             Machine profiles (embedded TOML + ~/.config/ctdev/profiles)
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
- Never spell `sudo` in an argv — use `sysutil.SudoRun` (or `sysutil.SudoNoPrompt` for silent probes). They drop the wrapper when we already are root, which is how a container with no sudo installed still works

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

Set `Root` when the component isn't a plain package install. The zero value,
`RootWhenMissing`, means root is needed to put the software in place while a
re-run over an installed component only re-syncs `$HOME` — so `ctdev install`
asks for a sudo password only when a selected component actually needs it, and a
dotfiles-only install works in a container with no sudo. Declare `RootAlways`
when every run does privileged work (redeploying a systemd unit, `sudo docker
compose`, a ufw rule) and `RootNever` when install and uninstall stay inside
`$HOME` (Homebrew, an upstream user-scope installer, the docker socket).
Forgetting `RootAlways` costs a password prompt inside the progress TUI, which
hangs — when in doubt, leave the default.

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
7. CI builds and creates the GitHub Release automatically via `.github/workflows/ci.yml`
