# dotfiles

Modular dotfiles for macOS and Linux. Managed via the `ctdev` CLI.

## Fresh Machine Setup

Install the `ctdev` binary, then either apply a **machine profile** (the fast
path) or compose the machine by hand from components and `configure` categories.

```bash
# 1. install ctdev
curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/install.sh | bash

# 2a. the fast path: apply a profile (built in — no repo clone needed)
ctdev apply                      # list profiles: dev-workstation, pihole-node, ai-node, family-desktop
ctdev apply dev-workstation      # plan → confirm → install + batch-configure → next steps
ctdev diff dev-workstation       # later: check the machine hasn't drifted (cron-able)
# add your own: ~/.config/ctdev/profiles/<name>.toml

# 2b. or by hand: install the components you want (each runs its configure step afterward)
ctdev install zsh git gh node go docker tailscale vscode claude-code tmux

# 3. apply the system settings you want
ctdev configure ssh --batch      # SSH server + key-based auth hardening
ctdev configure sleep --batch    # never suspend (always-on box)
ctdev configure locale --batch   # en_US.UTF-8 (for Mosh)
ctdev configure linger --batch   # keep user services alive without a login
ctdev configure tunnel --batch   # VS Code tunnel (optional)
# ctdev configure ufw --batch    # firewall — skip on a DNS/proxy host (Pi-hole/Caddy)
```

`ctdev install <x>` pulls in declared dependencies first and runs `<x>`'s
configure step afterward; `ctdev configure <x>` configures without installing.
Everything is idempotent and safe to re-run.

**Manual steps**

- Add your client's SSH public key: `echo 'ssh-ed25519 ...' >> ~/.ssh/authorized_keys`, then re-run `ctdev configure ssh --batch` (password auth is disabled only once a key is present)
- Authenticate the VS Code tunnel once: `code tunnel user login`
- `gh auth login` · `ctdev configure git` · (optional) `sudo tailscale up`
- Verify everything: `ctdev verify`

## Pi-hole / Homelab Node

There's no "homelab mode" — you compose a node from individual components and
`configure` categories. To turn a freshly flashed **Raspberry Pi OS Lite** (or
Ubuntu/Debian) box into a Pi-hole node behind a Caddy reverse proxy serving
`https://*.<your-domain>` with a Let's Encrypt **wildcard** cert (Cloudflare
DNS-01, nothing exposed to the internet):

```bash
# after SSHing in and installing ctdev (see "Install" below):
ctdev apply pihole-node                  # ← the whole block below in one command,
                                         #    then follow its printed next steps
# — or step by step: —
ctdev install zsh git tailscale          # whatever base tools you want
sudo tailscale up                        # join the tailnet
ctdev install pihole                     # network-wide DNS ad blocker
ctdev configure pihole                   # upstreams, listening mode, blocking
ctdev install docker                     # caddy needs docker
ctdev configure caddy --domain example.com --acme-email you@example.com
                                         # prompts for the Cloudflare token (masked); or pass it
                                         # via env: CF_API_TOKEN=<token> ctdev configure caddy ...
                                         # (avoid --cf-token — flags land in shell history and ps)
ctdev install caddy                      # deploy the proxy stack + bring it up
ctdev install portainer                  # optional: Docker management web UI
ctdev install beszel                      # optional: server/container monitoring
ctdev install restic                      # optional: daily restic backups
ctdev configure restic                    # set repo + credentials, init, enable timer
```

`ctdev install portainer` brings up Portainer CE (a web UI to view and manage
the host's containers, images, volumes, and compose stacks). Caddy serves it at
`https://portainer.<domain>`; it's also reachable directly at `https://<node>:9443`.
Create the admin user on first login. It mounts the Docker socket, so keep it
off any public network.

`ctdev install beszel` brings up Beszel (lightweight server/container
monitoring — a hub web UI plus an agent that reports this host's CPU, memory,
disk, network, temps, and per-container stats with alerting). Caddy serves the
hub at `https://beszel.<domain>`. The install starts the hub first; create the
admin user, click "Add System", put the issued KEY/TOKEN in `~/beszel/.env`,
then re-run `ctdev install beszel` to start the agent. Keep it off any public
network (Tailscale only).

`ctdev install restic` installs restic, a daily backup timer, and the backup/restore
helper scripts. Then `ctdev configure restic` prompts for the repository (any restic
backend — Backblaze B2, S3, SFTP, or a local/USB path), backend credentials, and a
repository password (it can generate one), writes `/etc/restic/restic.env` (root-only,
never committed), seeds default exclude patterns, runs `restic init`, and enables the
timer. Snapshots are tagged with the hostname, so each machine backs up to its own repo
and sees only its own snapshots. Backups are **opt-in** — nothing is snapshotted until you
choose what to include with `ctdev backup paths`, a local web UI that browses the
filesystem with folder sizes and include/exclude buttons. (Includes are the trees to back
up; excludes carve junk out of them, e.g. include `~/Repos`, exclude `**/node_modules`.) `ctdev backup now` snapshots immediately; `ctdev backup
snapshots` lists them; `ctdev restore …` restores — **see [RECOVERY.md](RECOVERY.md)
for the complete disaster-recovery runbook.**

Secrets are **never stored in the repo**. Each (Cloudflare token, restic repo password
+ backend keys, Beszel KEY/TOKEN, Pi-hole password) is entered at its `configure` step
and stored only on the host; if lost, you reconfigure. restic backs up the rendered
`~/<svc>/.env` files, so a restore brings them back — keep the restic repo password
itself in your password manager, since restic can't restore its own credentials.

`ctdev configure caddy` writes `~/caddy/.env` (mode 600) and, when Pi-hole is
present, frees port 443 and points `*.<domain>` at this node's Tailscale IP. Then
set that Tailscale IP as a Global Nameserver (Override on) in the Tailscale admin
console, and `sudo pihole setpassword` for the admin UI.

**Skip `ctdev configure ufw` on a DNS/proxy host** — UFW's default-deny blocks
DNS (53) and the proxy (80/443) unless you open those ports first.

**Adding a service:** add the container to `~/caddy/docker-compose.yml` and a
route snippet in `~/caddy/sites/<svc>.caddy`, then `ctdev install caddy` (or
`sudo docker compose -f ~/caddy/docker-compose.yml up -d`). The wildcard DNS +
cert already cover it.

**Secrets.** A node's secrets are entered into its `configure` wizard (or `.env`) and
live only on that host — nothing secret is committed to the repo. Restoring a node from
its restic backup brings the `.env` files back; standing up a brand-new node means
re-entering them from your password manager. **Never commit a secret.**

## AI / MCP Node

An always-on box that holds the credentials so your laptops don't. Each MCP
server runs as a container reachable **only over Tailscale** — nothing is
published to the LAN or the internet.

```bash
ctdev apply ai-node                      # ← the whole block below in one command
# — or step by step: —
ctdev install zsh git tailscale docker
sudo tailscale up                        # join the tailnet
ctdev install mcp-email-server           # MCP server that reads your mailboxes over IMAP
ctdev configure mcp-email-server         # add mailboxes, publish it to the tailnet
ctdev install brain                      # the agent org + its scheduled runs
ctdev configure brain                    # checkout, schedule, Claude credential
ctdev install restic && ctdev configure restic   # back the mailbox config up
```

`ctdev install mcp-email-server` brings up
[mcp-email-server](https://github.com/ai-zerolab/mcp-email-server) — an MCP
server that reads (and optionally sends) mail over IMAP, so a laptop running
Claude Code reaches your mail through the tailnet instead of storing an email
password. `ctdev configure mcp-email-server` walks through adding each mailbox
over SSH (upstream's `ui` subcommand needs a browser), writes them to
`~/mcp-email-server/config/config.toml` on the node only, and puts
`tailscale serve` in front of the service. Point a laptop at it with:

```bash
claude mcp add --transport http email https://<node>.<tailnet>.ts.net/mcp
```

On a node that already runs Caddy — like a `pihole-node` — `configure` publishes
on tailnet port **8443** instead of 443, and says so. 443 is not free there:
`ctdev configure caddy` points `*.<domain>` at that node's *Tailscale* IP, and a
serve rule on 443 would intercept it, so every `https://<svc>.<domain>` on the
tailnet would start hitting the email server instead of Caddy. Override with
`--serve-port`. The endpoint is then
`https://<node>.<tailnet>.ts.net:8443/mcp`.

**The upstream server has no authentication of its own** — anything that can
reach its port reads every configured mailbox in full. Two things keep that
safe, and both matter: the container port is published on `127.0.0.1` only
(never `0.0.0.0` — Docker's iptables rules sit ahead of UFW, so a bare port
mapping is LAN-wide even on a firewalled host), and `tailscale serve` is the
only route in, which means TLS plus tailnet authentication. Use `serve`, never
`funnel`.

Mailbox passwords live in the managed catalog at
`~/mcp-email-server/config/managed.sqlite3`, mode 0600, owned by you — the
container runs as your uid, not root. They are **not encrypted at rest**; the
file mode and the fact that the directory never leaves one tailnet-only host are
what protect them. Use app-specific passwords, which you can revoke without
touching the account. Install and `configure` both print the catalog's path and
its actual mode, and adding an account runs a real IMAP login test so a wrong
password fails immediately rather than as an empty inbox days later.

`ctdev uninstall mcp-email-server` stops the container but keeps the catalog;
deleting it means re-issuing an app password for every mailbox. Include
`~/mcp-email-server` in `ctdev backup paths` so it lands in an encrypted restic
snapshot.

`ctdev install` refuses to run if something it didn't create is already serving
this role — a container built from another image, a `./config` belonging to a
different email server, or a `tailscale serve` mapping already on the port. It
names what it found and stops, because replacing such a server orphans its
mailbox accounts rather than migrating them, and the replacement then reports
"no accounts configured" instead of an error. Pass `--force` to replace it
anyway; `./config` is never deleted either way. Re-running install over its own
stack is idempotent and leaves the catalog untouched.

The stack is **built from the Dockerfile**, not pulled: upstream's published
container images stop at `0.16.0` while the Python package is on 1.x, so the
image line is a major version behind. The Dockerfile pins
`mcp-email-server==1.4.1` on `python:3.12-slim`. Bump that pin deliberately
after reading the upstream release notes, then re-run
`ctdev install mcp-email-server` (compose rebuilds).

### The brain

`ctdev install brain` puts your agent org — knowledge, agents, skills, commands
and the memory they accumulate — on the node, and runs its scheduled work there.
It is a git checkout and two systemd timers, not a container.

The point is that **the node becomes the only machine running agents on a
schedule.** A schedule that lives in a laptop's Claude Code session dies when the
window closes; worse, two laptops both running scheduled agents write to the same
memory files and quietly disagree. One always-on writer removes both problems.

```bash
ctdev install brain          # clone, run the repo's setup.sh, deploy the timers
ctdev configure brain        # checkout path, schedule, Claude credential
ctdev configure brain --show # what is configured, and what is still missing
sudo systemctl start brain-triage.service    # run it now
journalctl -fu brain-triage.service          # watch it
```

**Where it lives.** These paths are deliberately stable and not under anyone's
home directory, because a service that is not the timer — an app, later — has to
find the brain without reverse-engineering a systemd unit:

| Path | What |
| --- | --- |
| `/srv/brain` | the checkout, `brain:brain`, mode 2770 |
| `/var/lib/brain` | service state: run digests, the lock, the deploy key |
| `/etc/ctdev/brain.conf` | the pointer file — paths, remote, schedule. World-readable, holds nothing secret |
| `/etc/ctdev/brain-claude-token.cred` | the Claude token, encrypted to this host |
| `/usr/local/bin/brain-run` | the one entry point both timers call |

**It runs as its own service account.** Not you — `brain`, a system user, so its
commits are attributable to the node rather than to a person, and so a second
service account can join group `brain` and read the checkout later without being
the timer's user. The checkout is not world-readable: it holds household
financial detail.

**Two timers.** `brain-triage.timer` at **07:03 and 15:07** local, and
`brain-sync.timer` hourly at `:23` to keep the checkout current for whoever SSHes
in — which is how a phone reaches the brain, since a tailnet-only service is
unreachable from the Claude app but perfectly reachable from Claude Code over
SSH. Both are `Persistent=true`, so a run missed while the node was down happens
at boot. Change either with `ctdev configure brain`; it writes a drop-in and
leaves the shipped unit as the recorded default.

The two are enabled independently. Sync starts as soon as the checkout exists,
because it needs nothing but git; triage waits until a Claude credential is
stored, because enabling it earlier would buy two failed units a day. So a node
with a deploy key but no token yet still keeps its checkout current.

**The scheduled prompt points, it never restates.** It names the agent, the
skill and the memory file to read, and nothing else — because a prompt with the
rules copied into it is a snapshot that goes stale silently, and on an unattended
node that drift goes unnoticed for weeks. Put a `scheduled/triage.md` in the repo
and it wins over the shipped default, so the prompt can live with the rules it
points at.

**Headless authentication.** Claude Code's normal login is a browser flow, which
a timer cannot do. Run `claude setup-token` once on a machine that has a browser
— it mints a one-year token against your Pro/Max/Team/Enterprise subscription —
keep the master copy in 1Password, and paste it into `ctdev configure brain`.
It is encrypted to this host with `systemd-creds` and decrypted by systemd into
a private in-memory directory that only the triage unit can read. **No plaintext
copy is written anywhere**, the blob is inert on any other machine (including
inside a restic snapshot of `/etc`), and nothing has to reach a vault over the
network at 07:03 — which matters on a node that also serves the household's DNS.
Record the `op://` reference with `--token-ref` so `--show` can say where the
master copy lives; the reference is not a secret.

One thing that token cannot do: **fetch claude.ai connectors.** Gmail, Calendar,
Drive and Notion are unavailable to scheduled runs. Locally-configured MCP
servers work normally, which is what the tailnet mail server is.

**Pushing.** The node authenticates to git with a **repository deploy key** it
generates itself, so no credential is ever transported to it. `install` prints
the public half and the URL to paste it at; it needs **write access**, because
the node pushes what its runs learn. Revoking it is one click and touches no
human's account.

**One writer, and failures are loud.** Every run rebases onto `origin`
immediately before it works and pushes immediately after, so the window where the
node holds unpushed commits is seconds. A `flock` serializes the two timers
against each other and against a hand-run `brain-run`. Nothing is ever
force-pushed and no conflict is ever auto-resolved: a rejected push is retried
once after a rebase, and a genuine conflict aborts and **fails the unit**, which
is how it shows up in `systemctl --failed` and `ctdev status` instead of
accumulating in silence.

**What the scheduled session can reach** is narrower than a normal session. The
built-in tool set is cut to delegation and file access — **no shell, no web
fetch, no web search** — because `brain-run` does the git work itself, so the
session never needs one. MCP is an allow-list built from what the repo's own
setup registered, not a deny-list: add a server next year and it is *not*
automatically in reach of a session that reads attacker-controlled email.

`ctdev uninstall brain` stops the timers and removes the units and the runner.
It keeps the checkout, `memory/`, the state directory, the config and the
credential — `memory/` is accumulated learning that exists nowhere else.

## Install (ctdev only)

```bash
curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/install.sh | bash
```

## Diagnose a machine without installing anything

For a machine you're only visiting — a family member's laptop, a client's
desktop. Downloads to a temp directory, runs the report, and deletes itself.
Nothing is installed, no PATH is changed, and sudo is never used.

```bash
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/install.sh | bash -s -- --doctor
```

```powershell
# Windows — `irm | iex` cannot pass arguments, so wrap it in a scriptblock
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/install.ps1))) -Doctor
```

## Getting Started

```bash
ctdev configure                 # Walk through all system configuration categories
ctdev install zsh git gh        # Install components you need
ctdev configure git             # Set your git name, email, and signing key
```

Use `--dry-run` on any command to preview changes before applying.

## Commands

```bash
ctdev install <component...>    # Install specific components
ctdev uninstall <component...>  # Remove specific components
ctdev update [-y]               # Update system packages and components
ctdev update --check            # List available updates without installing
ctdev update --refresh-keys     # Refresh APT GPG keys before updating
ctdev info                      # Show system information
ctdev configure                 # Walk through all system configuration
ctdev configure <category>      # Configure a single category (gpu, boot, power, ...)
ctdev configure --show          # Show current system configuration
ctdev configure git             # Configure git user and SSH signing key
ctdev configure aws             # Configure AWS profile
ctdev configure ssh             # SSH server + key-based auth hardening
ctdev configure ufw             # UFW firewall (SSH/Mosh from private ranges)
ctdev configure pihole          # Pi-hole DNS (upstreams, listening mode, blocking)
ctdev configure caddy           # Caddy reverse proxy (domain, ACME email, CF token)
ctdev configure restic          # restic backups (repo, credentials, paths) — --show
ctdev configure mcp-email-server # mailboxes for the MCP email server (+ tailscale serve)
ctdev configure gpu             # NVIDIA driver/MOK signing + GPU settings (--show, --recover)
ctdev backup now                # Run a restic snapshot of this machine now
ctdev backup snapshots          # List this machine's restic snapshots
ctdev backup paths              # Pick what to back up in a local web UI
ctdev backup paths --listen tailnet  # ...reachable from another device on your tailnet
ctdev restore ls|files|in-place|check  # Inspect/restore from restic
ctdev cleanup                   # Reclaim disk space (scan, pick tasks, clean; --dry-run to preview)
ctdev verify                    # Verify the bootstrap installation
ctdev doctor                    # Diagnose this machine's network and hardware
ctdev doctor --deep             # + vendor APIs, Wi-Fi scan, path trace
ctdev doctor --report           # also write a shareable Markdown report
ctdev doctor --redact           # mask SSID, MACs, and public IP before sharing

# Read the UniFi controller too (radar events, airtime, mesh uplinks).
# Create a read-only key: Settings → Control Plane → Integrations.
CTDEV_UNIFI_API_KEY=<key> ctdev doctor --deep
```

Run `ctdev install` (no args) for an interactive component picker.

**Flags:** `--help`, `--dry-run`, `--verbose`, `--force`, `--version`

## Components

52 components across CLI tools, desktop apps, runtimes, security, infrastructure, and system utilities. Run `ctdev install` to browse interactively.

## DevContainers

Add to your VS Code `settings.json`:

```json
{
  "dotfiles.repository": "https://github.com/ConnerTechnology/dotfiles.git",
  "dotfiles.targetPath": "~/dotfiles",
  "dotfiles.installCommand": "./devcontainer.sh"
}
```

## Platform Support

- **Ubuntu/Debian/Linux Mint** - apt (primary target)
- **macOS** - Homebrew
- **Windows** - `ctdev doctor` only; every other command refuses up front

Other package managers (dnf, pacman) are not supported; components on those
systems report as skipped.

## Uninstall

```bash
ctdev uninstall <component...>   # Remove specific components
curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/uninstall.sh | bash
```

## License

MIT
