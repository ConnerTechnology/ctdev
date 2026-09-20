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
declare otherwise: the ones that do privileged work on every run (`restic`,
`caddy`, `nomachine`, `smartmontools`, `brain`) and the ones that never need it
at all, because they install entirely inside `$HOME` or through the Docker
socket. ctdev asks for a sudo password only when something in the run is
actually going to use it. See [Security](../security.md).

## The registry

53 components, counted in `ctdev/component/registry.go` on 2026-09-20. A
component unsupported on this OS reports as skipped rather than failing.

### CLI Tools

| Component | What it is | Platforms |
| --- | --- | --- |
| `bat` | cat with syntax highlighting | Linux, macOS |
| `btop` | Resource monitor | Linux, macOS |
| `bun` | JavaScript runtime and package manager | Linux, macOS |
| `claude-code` | Claude Code CLI and configuration | Linux, macOS |
| `devcontainer` | Dev Containers CLI + dx wrapper | Linux, macOS |
| `direnv` | Per-directory environment variables | Linux, macOS |
| `docker` | Docker container runtime | Linux, macOS |
| `doctl` | DigitalOcean CLI | Linux, macOS |
| `fd` | Fast, friendly find alternative | Linux, macOS |
| `fzf` | Fuzzy finder for the shell | Linux, macOS |
| `gh` | GitHub CLI | Linux, macOS |
| `git-spice` | Git Spice stacked branches tool | Linux, macOS |
| `helm` | Kubernetes package manager | Linux, macOS |
| `jq` | JSON processor | Linux, macOS |
| `kubectl` | Kubernetes CLI | Linux, macOS |
| `lazygit` | Terminal UI for git | Linux, macOS |
| `mosh` | Mobile shell (SSH that survives roaming/sleep) | Linux, macOS |
| `ripgrep` | Fast recursive grep | Linux, macOS |
| `shellcheck` | Shell script linter | Linux, macOS |
| `syncthing` | Peer-to-peer file sync between your machines | Linux, macOS |
| `tmux` | Terminal multiplexer | Linux, macOS |
| `zoxide` | Smarter cd that learns your directories | Linux, macOS |

### Desktop Applications

| Component | What it is | Platforms |
| --- | --- | --- |
| `1password` | 1Password password manager | Linux, macOS |
| `chrome` | Google Chrome browser | Linux, macOS |
| `claude-desktop` | Claude desktop application | macOS |
| `cleanmymac` | CleanMyMac system cleaner | macOS |
| `dbeaver` | DBeaver database tool | Linux, macOS |
| `linear` | Linear issue tracker | macOS |
| `logi-options` | Logitech Options+ | macOS |
| `nomachine` | NoMachine remote desktop server | Linux |
| `slack` | Slack messaging | Linux, macOS |
| `tradingview` | TradingView desktop charts | Linux, macOS |
| `vscode` | Visual Studio Code | Linux, macOS |

### Development Runtimes

| Component | What it is | Platforms |
| --- | --- | --- |
| `fonts` | Nerd Fonts for terminal | Linux, macOS |
| `git` | Git configuration and aliases | Linux, macOS |
| `go` | Go toolchain (official tarball) | Linux, macOS |
| `node` | Node.js via nodenv | Linux, macOS |
| `ruby` | Ruby via rbenv | Linux, macOS |
| `zsh` | Zsh, Oh My Zsh, Pure prompt, plugins | Linux, macOS |

### Security & Encryption

| Component | What it is | Platforms |
| --- | --- | --- |
| `age` | age file encryption tool | Linux, macOS |
| `sops` | Mozilla SOPS secrets manager | Linux, macOS |
| `tailscale` | Tailscale VPN | Linux, macOS |

### Infrastructure

| Component | What it is | Platforms |
| --- | --- | --- |
| `beszel` | Beszel server/container monitoring (Docker) | Linux |
| `brain` | ConnerTechnology/brain agent org + its scheduled runs (systemd) | Linux |
| `caddy` | Caddy reverse proxy (Cloudflare DNS-01 wildcard) | Linux |
| `mcp-email-server` | MCP server exposing your mailboxes over IMAP (Docker) | Linux |
| `pihole` | Pi-hole network-wide DNS ad blocker (Docker) | Linux |
| `portainer` | Portainer CE Docker management UI (Docker) | Linux |
| `restic` | restic backups to B2 + local USB (systemd timer) | Linux |
| `terraform` | Terraform infrastructure tool | Linux, macOS |

### System Tools

| Component | What it is | Platforms |
| --- | --- | --- |
| `earlyoom` | Early OOM killer for Linux | Linux |
| `smartmontools` | SMART disk-health monitoring (smartd) | Linux |
| `solaar` | Logitech Unifying/Bolt receiver manager | Linux |
## Adding one

The template, the `Root` rules and the two-phase install contract are in
[Adding a component](../adding-a-component.md).
