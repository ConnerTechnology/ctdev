# CLAUDE.md

Instructions for Claude Code when working with this repository.

## ctdev CLI

`ctdev install <component>` installs the component (pulling in its `Dependencies`
first) and then runs its configuration step if it has one — a `configure <name>`
category (e.g. `pihole`) or a dedicated wizard (`caddy`, `mcp-email-server`; see
`componentWizards` in `cmd/install.go`). Re-running `install` on something
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
ctdev configure mcp-email-server # mailboxes for the MCP email server (+ tailscale serve)
ctdev configure brain           # brain checkout, schedule, Claude credential — --show
ctdev configure gpu             # NVIDIA driver/MOK signing + GPU settings (--show, --recover)
ctdev configure <category> --batch  # Apply a category's defaults non-interactively
ctdev backup now                # Run a restic snapshot of this machine now
ctdev backup test               # Check backups are set up correctly (config, connection, paths)
ctdev backup disable            # Pause scheduled backups (config + snapshots kept)
ctdev backup enable             # Resume scheduled backups
ctdev backup snapshots [primary|local]  # List this machine's restic snapshots
ctdev backup paths              # Pick what to back up in a local web UI
ctdev backup paths --listen tailnet  # serve the picker on this node's Tailscale address
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

# Vendor deep-dive: the key alone is enough — the controller defaults to the gateway.
CTDEV_UNIFI_API_KEY=<key> ctdev doctor --deep
ctdev doctor --deep --unifi https://10.2.2.1   # when it isn't the gateway
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

52 components:

1password, age, bat, beszel, brain, btop, bun, caddy, chrome, cleanmymac, claude-code, claude-desktop, dbeaver, devcontainer, direnv, docker, doctl, earlyoom, fd, fonts, fzf, gh, git, git-spice, go, helm, jq, kubectl, lazygit, linear, logi-options, mcp-email-server, mosh, node, nomachine, pihole, portainer, restic, ripgrep, ruby, shellcheck, slack, smartmontools, solaar, sops, syncthing, tailscale, terraform, tmux, vscode, zoxide, zsh

## Profiles

Machine profiles are declarative TOML files (components + configure categories
applied at recommended values). Built-ins — `pihole-node`, `ai-node`, `dev-workstation`,
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
system packages. It checks each managed stack (pihole, caddy, beszel, portainer,
mcp-email-server) for a newer image by digest — without pulling — and updates the ones you select
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
- **mcp-email-server** (one app-specific password per mailbox) →
  `~/mcp-email-server/config/config.toml` (0600, cleartext — a headless node has no
  keyring), written by the container during `ctdev configure mcp-email-server`.

restic snapshots the rendered `~/<svc>/.env` files (they're under the backed-up paths),
so a restore brings them back; a brand-new node re-enters them from your password manager.
**Never commit a secret.** See `RECOVERY.md` (disaster recovery).

## AI / MCP nodes

An always-on node that holds credentials so laptops don't have to. Same shape as
the homelab node — compose it from components; `ctdev apply ai-node` is the
built-in profile.

- `ctdev install mcp-email-server` — [mcp-email-server](https://github.com/ai-zerolab/mcp-email-server)
  as a Docker compose stack in `~/mcp-email-server/`, exposing IMAP mailboxes to
  MCP clients over streamable-HTTP.
- `ctdev configure mcp-email-server` — adds/removes mailboxes and publishes the
  service to the tailnet. Also `--show`.

**Upstream ships no authentication.** `mcp-email-server streamable-http` takes
only `--host` and `--port`; whatever reaches the port reads every mailbox. Three
invariants carry the whole security model, and all three are load-bearing:

1. **The published port is `127.0.0.1:9557` only.** Never `0.0.0.0`, never a bare
   mapping — Docker's iptables rules are inserted ahead of UFW, so a bare mapping
   is LAN-wide even on a firewalled host. `TestMCPEmailServerPublishesOnLoopbackOnly`
   asserts this against the embedded compose file.
2. **`tailscale serve` on the host is the security boundary** — TLS from the
   tailnet's own cert, reachable only by authenticated peers. On the host rather
   than in a sidecar: the node is already on the tailnet, so a sidecar would add
   a second node identity plus an auth key to store, and its serve config would
   die with the container. `serve`, never `funnel`.
3. **The version is pinned.** `latest` on the process holding mail credentials
   is a silent-upgrade risk. Bump `mcp-email-server==<x.y.z>` in
   `component/configs/mcp-email-server/Dockerfile` deliberately.

**Do not serve on 443 next to Caddy.** `ctdev configure caddy` points
`*.<domain>` at the node's *Tailscale* IP, and Caddy answers there on 443. A
`tailscale serve --https=443` rule intercepts that port for the node's own
tailnet addresses, so every homelab site would silently start hitting the email
server. `MCPEmailServerServePort` therefore returns 8443 when the caddy stack is
present, 443 otherwise, and an explicit `MCP_SERVE_PORT` in the stack's `.env`
(written by `--serve-port`) wins over both. It probes for the caddy compose file
by path rather than through `FindByName`, because the registry references this
component's uninstaller — reading `Registry` from there is an initialization
cycle. A bare hostname in `MCP_ALLOWED_HOSTS` covers every port (upstream
expands it to `<name>` and `<name>:*`), so a non-default port needs no extra
allowlist entry.

Two non-obvious facts, both verified against the running container rather than
inferred — change either and tailnet requests break in a way that looks like a
network fault:

- `tailscale serve` forwards the client's **original Host header**
  (`r.Out.Host = r.In.Host` in `ipn/ipnlocal/serve.go`), and MCP's DNS-rebinding
  protection answers **421** for any Host it wasn't told about. So the node's
  MagicDNS name must be in `MCP_ALLOWED_HOSTS`, which
  `ctdev configure mcp-email-server` writes into `~/mcp-email-server/.env`.
  Unconfigured, the compose defaults are loopback-only — it fails closed.
- **The stack is built, not pulled.** `ghcr.io/ai-zerolab/mcp-email-server` stops
  at 0.16.0 (44 tags, single page, no 1.x; `manifests/1.4.1` → 404) while PyPI is
  on 1.x. So the Dockerfile installs the pinned package on `python:3.12-slim`,
  the same locally-built shape caddy uses (`build: .` + `image: <name>:local`).
  1.x is also what makes headless setup first-class: `account add
  --password-stdin` (upstream's own "never place credentials in argv"), `--json`
  on every command, and `account test` for a real IMAP login check.
- **The container runs as the invoking user, not root.** 1.x refuses to open its
  catalog unless the parent directory is owner-only *from the running user's*
  point of view — a root-run container against a user-owned `./config` fails with
  "Managed catalog parent must be owner-only". `ctdev install` writes `MCP_UID`/
  `MCP_GID` into the stack's `.env` before the first `compose up`, and the
  compose file uses the `${MCP_UID:?}` form so a missing value fails loudly
  instead of silently reverting to root. `.env` is therefore written by *both*
  install (uid/gid) and configure (tailnet settings) — `MCPEmailServerSetEnv`
  merges rather than truncates.
- **Install must not silently replace another email server.** It did once:
  `list_available_accounts` came back `[]` with `isError:false`, which reads as
  "no mailboxes", not "your accounts were orphaned". `mcpEmailServerConflicts`
  now gates the install on three probes — a container whose `.Config.Image`
  isn't `mcp-email-server:local` (use `.Config.Image`, not `docker ps`, which
  reports a bare hash once a local rebuild moves the tag), a non-empty
  `./config` without `managed.sqlite3`, and a `tailscale serve` handler on the
  serve port pointing somewhere other than our loopback port. The rules are a
  pure function over a `mcpEmailServerState` struct so they are testable without
  docker or a tailnet. `--force` replaces the container (never its volumes) but
  still keeps `./config`. Install also prints the account count every run, so an
  empty catalog is stated rather than inferred.
- **`account remove` needs `--expected-revision` and `--confirm`.** Upstream
  guards removal with optimistic concurrency; `account remove <name>` alone
  fails, and failed silently here until it was run against a real server.
  `MCPEmailServerRemoveAccount` reads the revision back from `account list`
  first. `account add` rejects a duplicate name outright, so replacing an
  account is remove-then-add.
- **Secrets are not encrypted at rest.** The managed catalog stores passwords in
  cleartext inside `managed.sqlite3` (verified with `strings`); 0600 and the
  node's isolation are the whole protection. Don't describe it as encrypted.

Credentials live in a **bind mount** (`./config`), not a named volume, so they
survive `docker compose down` *and* the host can verify the mode without
entering the container. `ctdev uninstall` stops the stack and keeps the catalog —
losing it means re-issuing an app password per mailbox.

## The brain (`brain` component)

`ctdev install brain` provisions **ConnerTechnology/AI** — the agent org — onto an
always-on node and runs its scheduled work there. A git checkout and two systemd
timers; **not** a compose stack. `ctdev configure brain` sets the checkout, the
schedule, and the Claude credential; `--show` reports state.

The reason it exists: a schedule living in a Claude Code session dies with the
window, and two laptops running scheduled agents write the same `memory/` files
and disagree. One always-on writer removes both.

**Paths are an API surface**, chosen so a later service (the tailnet app in the AI
repo's `docs/vision.md`) can find the brain without reading a systemd unit:

```
/srv/brain                          checkout, brain:brain, 2770 (setgid, no world bit)
/var/lib/brain                      state: runs/, brain.lock, .ssh/, the account's $HOME
/etc/ctdev/brain.conf               pointer file — 0644, shell-quoted, NO SECRETS
/etc/ctdev/brain-claude-token.cred  Claude token, host-encrypted
/etc/ctdev/brain-triage.prompt      default prompt; <repo>/scheduled/triage.md wins
/usr/local/bin/brain-run            the one entry point both timers call
```

**The service account is `brain`**, a system user — deliberately neither Thomas
nor Le'Anna, who are principals of equal standing. Its commits are attributable to
the node. A second service account joins group `brain` to read the checkout
without being the timer's user.

Facts that are load-bearing; change any of them and something breaks quietly:

- **`systemd-creds`, not `op run`, holds the Claude token.** Unattended `op` needs
  `OP_SERVICE_ACCOUNT_TOKEN`, itself a long-lived secret that would have to sit on
  the node in plaintext to bootstrap the thing meant to keep plaintext off it —
  plus a network round-trip at 07:03 on the node that serves the household's DNS.
  1Password stays the system of record (`BRAIN_TOKEN_REF` records the `op://` URI,
  which is not secret); what lands here is encrypted to
  `/var/lib/systemd/credential.secret`. That file is **not** in the restic backup
  set, which is what makes the `.cred` inert inside a snapshot of `/etc`.
- **The token comes from `claude setup-token`** (one year, needs Pro/Max/Team/
  Enterprise). It **cannot fetch claude.ai connectors** — Gmail, Calendar, Drive,
  Notion are unavailable to scheduled runs. Locally-configured MCP servers work,
  which is what the tailnet mail server is. The `inbox` agent only uses
  `mcp__email__*`, so triage is unaffected.
- **Git auth is a repo deploy key generated on the node.** The only credential in
  the design with no transport problem. It needs *write* access; without it the
  node commits and strands.
- **Nothing is ever force-pushed and no conflict is auto-resolved.** A rejected
  push rebases and retries once; a real conflict aborts and **fails the unit**, so
  it surfaces in `systemctl --failed` / `ctdev status`. `brain-run` holds a
  `flock` so the two timers and a hand run cannot interleave. `brain_test.go`
  asserts the absence of `push --force`, `reset --hard`, `checkout --theirs`.
- **The prompt points, it never restates.** A prompt with the rules copied in is a
  snapshot that goes stale silently — it already happened on 2026-08-20. The test
  caps the shipped prompt's length for exactly this reason.
- **`brain-sync.service` deliberately loads no credential.** Git only, so the
  checkout keeps tracking origin after the token expires and a laptop's `git pull`
  still receives what the last triage committed. `BrainEnableTimers` gates the two
  separately for the same reason: sync starts as soon as a checkout exists, triage
  waits for the credential. The half that works is never withheld by the half that
  does not, and it is the half a phone depends on.
- **MCP is an allow-list, not a deny-list.** `brain-run` filters the servers the
  repo's setup registered down to `BRAIN_CLAUDE_MCP` (default `email`) and passes
  `--strict-mcp-config`. A deny-list would fail open when a server is added later.
  `--tools` separately removes Bash, WebFetch and WebSearch from the session —
  verified against a live session, not assumed.
- **The workspace is marked trusted in the service account's `~/.claude.json`.**
  Claude Code ignores a project's `.claude/settings.json` until a trust dialog is
  accepted, and a timer cannot answer a dialog. Without it the node silently runs
  with different settings from every laptop.
- **`Root: RootAlways`.** Every run redeploys units, writes `/etc`, and works as
  the service account. `DetectPath` is the runner, not the checkout, so a repo
  cloned by hand does not read as an installed component.
- **Existence checks go through `brainPathExists`.** `/srv/brain` (2770) and
  `/var/lib/brain` (0750) are not traversable by the operator's own account, so a
  bare `os.Stat` reports "missing" for files that are plainly there.

Uninstall stops the timers and removes the units and runner. It **keeps** the
checkout, `memory/`, the state directory, `brain.conf`, the credential, the
schedule drop-ins and the account.

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
