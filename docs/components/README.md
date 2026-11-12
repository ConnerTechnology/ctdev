# Components

A component is one installable thing — a CLI tool, a desktop app, a runtime, a
container stack — that ctdev knows how to put on a machine and take off again.

```bash
ctdev install                    # interactive picker
ctdev install zsh git gh         # install by name
ctdev uninstall <component...>   # remove by name
```

`ctdev install <x>` pulls in `<x>`'s declared dependencies first, then runs its
configuration step if it has one: a `configure <name>` category, or a dedicated
wizard for the ones that need secrets. Re-running over something already
installed says so and jumps straight to configuration. Everything is idempotent.

## When root is used

Most components declare nothing and take the default: root is needed to put the
software in place — a package manager, `/usr/local`, a systemd unit — while
re-running over one already installed only re-syncs files under `$HOME`. A few
declare otherwise: the ones that do privileged work on every run (`caddy`,
`nomachine`, `smartmontools`) and the ones that never need it
at all, because they install entirely inside `$HOME` or through the Docker
socket. ctdev asks for a sudo password only when something in the run is
actually going to use it. See [Security](../security.md).

## The registry

64 components, counted in `ctdev/component/registry.go` on 2026-11-12. A
component unsupported on this OS reports as skipped rather than failing.

### CLI Tools

| Component | What it is | Platforms |
| --- | --- | --- |
| `acli` | Atlassian CLI (Jira, Confluence) | Linux, macOS |
| `aws` | AWS CLI v2 | Linux, macOS |
| `bat` | cat with syntax highlighting | Linux, macOS |
| `btop` | Resource monitor | Linux, macOS |
| `bun` | JavaScript runtime and package manager | Linux, macOS |
| `ccstatusline` | Claude Code status line | Linux, macOS |
| `ccusage` | Claude Code token usage and cost reports | Linux, macOS |
| `claude-code` | Claude Code CLI and configuration | Linux, macOS |
| `devcontainer` | Dev Containers CLI + dx wrapper | Linux, macOS |
| `direnv` | Per-directory environment variables | Linux, macOS |
| `docker` | Docker container runtime | Linux, macOS |
| `doctl` | DigitalOcean CLI | Linux, macOS |
| `fd` | Fast, friendly find alternative | Linux, macOS |
| `fzf` | Fuzzy finder for the shell | Linux, macOS |
| `gh` | GitHub CLI | Linux, macOS |
| `git-spice` | Git Spice stacked branches tool | Linux, macOS |
| `gws` | Google Workspace CLI (Drive, Gmail, Calendar, and more) | Linux, macOS |
| `helm` | Kubernetes package manager | Linux, macOS |
| `herdr` | Background runtime for coding agents: sessions keep running when you disconnect | Linux, macOS |
| `jq` | JSON processor | Linux, macOS |
| `kubectl` | Kubernetes CLI | Linux, macOS |
| `lazygit` | Terminal UI for git | Linux, macOS |
| `mosh` | Mobile shell (SSH that survives roaming/sleep) | Linux, macOS |
| `ripgrep` | Fast recursive grep | Linux, macOS |
| `shellcheck` | Shell script linter | Linux, macOS |
| `syncthing` | Peer-to-peer file sync between your machines | Linux, macOS |
| `t3code` | T3 Code CLI: one UI for Claude Code, Codex and other coding agents | Linux, macOS (Apple Silicon) |
| `tmux` | Terminal multiplexer | Linux, macOS |
| `zoxide` | Smarter cd that learns your directories | Linux, macOS |

### Desktop Applications

| Component | What it is | Platforms |
| --- | --- | --- |
| `1password` | 1Password password manager | Linux, macOS |
| `betterbird` | Betterbird email client (Thunderbird fork) | Linux |
| `chrome` | Google Chrome browser | Linux, macOS |
| `claude-desktop` | Claude desktop application | Linux, macOS |
| `cleanmymac` | CleanMyMac system cleaner | macOS |
| `dbeaver` | DBeaver database tool | Linux, macOS |
| `linear` | Linear issue tracker | macOS |
| `logi-options` | Logitech Options+ | macOS |
| `nomachine` | NoMachine remote desktop server | Linux |
| `paper` | Paper design tool for interfaces and prototypes | Linux, macOS (Apple Silicon) |
| `slack` | Slack messaging | Linux, macOS |
| `t3code-desktop` | T3 Code desktop app | Linux, macOS (Apple Silicon) |
| `tradingview` | TradingView desktop charts | Linux, macOS |
| `vscode` | Visual Studio Code | Linux, macOS |

### Development Runtimes

| Component | What it is | Platforms |
| --- | --- | --- |
| `fonts` | Nerd Fonts for terminal | Linux, macOS |
| `git` | Git configuration and aliases | Linux, macOS |
| `go` | Go toolchain (official tarball) | Linux, macOS |
| `node` | Node.js via nodenv | Linux, macOS |
| `python` | Python via pyenv | Linux, macOS |
| `ruby` | Ruby via rbenv | Linux, macOS |
| `zsh` | Zsh, Oh My Zsh, Pure prompt, plugins | Linux, macOS |

### Security & Encryption

| Component | What it is | Platforms |
| --- | --- | --- |
| `age` | age file encryption tool | Linux, macOS |
| `endorctl` | Endor Labs CLI (dependency and secret scanning) | Linux, macOS |
| `sops` | Mozilla SOPS secrets manager | Linux, macOS |
| `tailscale` | Tailscale VPN | Linux, macOS |

### Infrastructure

| Component | What it is | Platforms |
| --- | --- | --- |
| `beszel` | Beszel server/container monitoring (Docker) | Linux |
| `caddy` | Caddy reverse proxy (Cloudflare DNS-01 wildcard) | Linux |
| `kopia` | Kopia backup tool, pinned binary only | Linux |
| `pihole` | Pi-hole network-wide DNS ad blocker (Docker) | Linux |
| `portainer` | Portainer CE Docker management UI (Docker) | Linux |
| `restic` | restic backup program | Linux, macOS |
| `terraform` | Terraform infrastructure tool | Linux, macOS |

### System Tools

| Component | What it is | Platforms |
| --- | --- | --- |
| `earlyoom` | Early OOM killer for Linux | Linux |
| `smartmontools` | SMART disk-health monitoring (smartd) | Linux |
| `solaar` | Logitech Unifying/Bolt receiver manager | Linux |

## Docker on Linux

On Linux, `docker` comes from Docker's own apt repository. Before ctdev trusts
the repository key, it checks that the key's fingerprint is Docker's
(`9DC8 5822 9FC7 DD38 854A E2D8 8D81 803C 0EBF CD88`). If the key is different,
or the file carries anything besides that one public key, the install stops and
nothing gets added to `/etc/apt/keyrings`. If there is no
`/etc/docker/daemon.json`, the install writes one that caps each container's
json-file log at three 10 MB files. Without that cap the logs grow until the
disk is full. An existing `daemon.json` is never changed. The cap applies only
to containers created after the file is written.

The file is written only when Docker is actually installed. If Docker is
already there, `ctdev install docker` stops at "Docker already installed", so
an existing machine gets the cap through `ctdev install docker --force`. On a
machine where Docker is running and has no `daemon.json`, `--force` restarts
Docker, which stops running containers unless `live-restore` is on. On macOS,
Docker Desktop comes from Homebrew.

## Adding one

The template, the `Root` rules and the two-phase install contract are in
[Adding a component](../adding-a-component.md).
