# dotfiles

Modular dotfiles for macOS and Linux. Managed via the `ctdev` CLI.

## Fresh Machine Setup

There's no all-in-one bootstrap — install the `ctdev` binary, then compose the
machine from the components and `configure` categories you want.

```bash
# 1. install ctdev
curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/install.sh | bash

# 2. install the components you want (each runs its configure step afterward)
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
ctdev install zsh git tailscale          # whatever base tools you want
sudo tailscale up                        # join the tailnet
ctdev install pihole                     # network-wide DNS ad blocker
ctdev configure pihole                   # upstreams, listening mode, blocking
ctdev install docker sops                # caddy needs docker
ctdev configure caddy --domain example.com --acme-email you@example.com --cf-token <token>
ctdev install caddy                      # deploy the proxy stack + bring it up
```

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

**Secrets via SOPS (optional).** If you'd rather keep a node's `.env` encrypted
in the repo than type it into the wizard, store it at
`ctdev/component/configs/caddy/hosts/<node>.sops.env` (age recipient in
`.sops.yaml`) and decrypt it on the node into `~/caddy/.env` with `sops`. **Never
commit a plaintext host config or an age private key.**

## Install (ctdev only)

```bash
curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/install.sh | bash
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
ctdev gpu info                  # Show GPU hardware info and signing status
ctdev gpu setup                 # Configure MOK signing for NVIDIA drivers
ctdev cleanup                   # Run all cleanup tasks
ctdev verify                    # Verify the bootstrap installation
```

Run `ctdev install` (no args) for an interactive component picker.

**Flags:** `--help`, `--dry-run`, `--verbose`, `--force`, `--version`

## Components

39 components across CLI tools, desktop apps, runtimes, security, infrastructure, and system utilities. Run `ctdev install` to browse interactively.

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

Other package managers (dnf, pacman) are not supported; components on those
systems report as skipped.

## Uninstall

```bash
ctdev uninstall <component...>   # Remove specific components
curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/uninstall.sh | bash
```

## License

MIT
