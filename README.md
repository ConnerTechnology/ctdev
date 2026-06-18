# dotfiles

Modular dotfiles for macOS and Linux. Managed via the `ctdev` CLI.

## Fresh Machine Setup

On a fresh Linux Mint 22.x (Ubuntu 24.04 base) machine, one command bootstraps
the whole development + remote-access environment:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/bootstrap.sh)
```

Or, from a clone (builds `ctdev` from source when Go is present):

```bash
./bootstrap.sh
```

`bootstrap.sh` is a thin orchestrator — it primes sudo once, installs base apt
packages, installs the `ctdev` binary, then delegates to `ctdev`. It's
idempotent and safe to re-run.

**What it installs**

- Base apt packages: `git curl wget build-essential zsh tmux mosh openssh-server ufw htop jq ripgrep fd-find unzip`
- Via `ctdev install`: zsh (+ Oh My Zsh, Pure prompt, plugins, dotfiles), git, gh,
  Node (nodenv), Go (official tarball), Docker (official repo + Compose), Tailscale,
  VS Code (Microsoft repo), Claude Code, tmux, jq, and the Dev Containers CLI (`@devcontainers/cli` + the `dx` wrapper)

**What it configures** (`ctdev configure remote`)

- SSH server enabled; key-based auth hardened (password auth disabled only once an authorized key exists)
- UFW allowing SSH (22/tcp) + Mosh (60000:61000/udp) from private LAN ranges
- `en_US.UTF-8` locale (required by Mosh)
- Sleep/suspend/hibernate masked (always-on desktop)
- WiFi power-save disabled (NetworkManager drop-in)
- systemd lingering enabled (keeps user services alive without a login session)
- VS Code tunnel service (reach this machine from a browser at vscode.dev)

**Manual steps after the script runs**

- Add your client's SSH public key: `echo 'ssh-ed25519 ...' >> ~/.ssh/authorized_keys`, then re-run `ctdev configure remote --batch`
- Authenticate the VS Code tunnel once: `code tunnel user login`
- `gh auth login` · `ctdev configure git` · (optional) `sudo tailscale up`
- Reboot (or log out/in) to apply docker group membership, suspend masking, and WiFi power-save
- Verify everything: `ctdev verify`

## Homelab Node Setup

Turn a freshly flashed **Raspberry Pi OS Lite** (or Ubuntu/Debian) box into a
homelab node — Docker, Tailscale, Pi-hole, and a Caddy reverse proxy serving
`https://*.<your-domain>` with a Let's Encrypt **wildcard** cert (Cloudflare
DNS-01, so nothing is exposed to the internet). After flashing and SSHing in,
from a clone:

```bash
./bootstrap-homelab.sh ctpi01
```

The node name must match an encrypted host config at
`ctdev/component/configs/homelab/hosts/<node>.sops.env`. The script installs the
server components, sets the hostname, configures remote access, and brings up
the proxy stack.

**Per-node config & secrets (SOPS).** Each node's domain, ACME email, and
Cloudflare API token live in a SOPS-encrypted dotenv, decrypted on the node into
`~/homelab/.env` (mode 600). To add a node:

```bash
# 1. put the homelab age PRIVATE key on the node (out-of-band; never commit it)
mkdir -p ~/.config/sops/age && cp keys.txt ~/.config/sops/age/keys.txt

# 2. create/edit the encrypted host config (uses the age recipient in .sops.yaml)
sops ctdev/component/configs/homelab/hosts/<node>.sops.env
#    keys: HOSTNAME, HOMELAB_DOMAIN, HOMELAB_ACME_EMAIL, CF_API_TOKEN
```

**Manual steps after the script:** `sudo tailscale up`, then finish DNS wiring
with `CTDEV_HOMELAB_HOST=<node> ctdev install homelab --force`; set the node's
Tailscale IP as a Global Nameserver (Override on) in the Tailscale admin console;
`sudo pihole setpassword`.

**Adding a service** to a node: add the container to `~/homelab/docker-compose.yml`
and a route snippet in `~/homelab/sites/<svc>.caddy`, then
`sudo docker compose -f ~/homelab/docker-compose.yml up -d && ... restart caddy`.
The wildcard DNS + cert already cover it — no DNS or cert changes needed.

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
ctdev configure remote          # Configure SSH/Mosh/UFW/tunnel for remote access
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
