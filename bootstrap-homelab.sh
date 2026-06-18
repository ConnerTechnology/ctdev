#!/usr/bin/env bash
set -euo pipefail

# bootstrap-homelab.sh — provision a fresh Debian/Ubuntu box (e.g. Raspberry Pi
# OS Lite) as a Conner Technology homelab node: Docker + Tailscale + Pi-hole +
# Caddy reverse proxy serving https://*.<domain> with a Let's Encrypt wildcard.
#
# Run from a clone after flashing + SSHing in:
#   ./bootstrap-homelab.sh ctpi01
#
# The argument is the node name; it must match an encrypted host config at
# ctdev/component/configs/homelab/hosts/<name>.sops.env. Idempotent.

REPO="ConnerTechnology/dotfiles"
RAW_URL="https://raw.githubusercontent.com/${REPO}/main"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
AGE_KEY="${SOPS_AGE_KEY_FILE:-$HOME/.config/sops/age/keys.txt}"

SCRIPT_DIR=""
if [[ -n "${BASH_SOURCE[0]:-}" && -f "${BASH_SOURCE[0]}" ]]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fi

# Server-oriented base packages (no desktop stack).
BASE_PACKAGES=(git curl wget ca-certificates zsh tmux mosh openssh-server ufw htop jq unzip)

# Components for a headless homelab node. 'homelab' pulls in docker + sops.
PRE_COMPONENTS=(zsh git tmux jq docker tailscale sops age pihole)

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'; BLUE='\033[0;34m'; NC='\033[0m'
info()    { echo -e "${BLUE}[homelab]${NC} $1"; }
success() { echo -e "${GREEN}[homelab ✓]${NC} $1"; }
warn()    { echo -e "${YELLOW}[homelab !]${NC} $1"; }
die()     { echo -e "${RED}[homelab ✗]${NC} $1" >&2; exit 1; }

trap 'die "failed at line ${LINENO}"' ERR

NODE="${1:-$(hostname)}"

require_linux_apt() {
    [[ "$(uname -s)" == "Linux" ]] || die "targets Linux (Raspberry Pi OS / Ubuntu/Debian). Detected: $(uname -s)"
    command -v apt-get >/dev/null 2>&1 || die "apt-get not found — this script supports apt-based distros."
}

require_host_config() {
    local cfg="$SCRIPT_DIR/ctdev/component/configs/homelab/hosts/${NODE}.sops.env"
    [[ -f "$cfg" ]] || die "no host config for '${NODE}' at ${cfg#"$SCRIPT_DIR"/}. Create it with: sops ctdev/component/configs/homelab/hosts/${NODE}.sops.env"
}

prime_sudo() {
    info "Requesting sudo (you'll be prompted once)..."
    sudo -v || die "sudo is required"
    ( while true; do sudo -n true || true; sleep 50; kill -0 "$$" 2>/dev/null || exit; done ) 2>/dev/null &
}

install_base_packages() {
    info "Updating apt and installing base packages..."
    sudo apt-get update -qq
    local missing=() pkg
    for pkg in "${BASE_PACKAGES[@]}"; do
        dpkg -s "$pkg" >/dev/null 2>&1 || missing+=("$pkg")
    done
    [[ ${#missing[@]} -eq 0 ]] || sudo apt-get install -y -qq "${missing[@]}"
    sudo systemctl enable --now ssh
    success "Base packages installed, sshd enabled"
}

have_go()   { command -v go >/dev/null 2>&1 || [[ -x /usr/local/go/bin/go ]]; }
go_binary() { if command -v go >/dev/null 2>&1; then command -v go; else echo /usr/local/go/bin/go; fi; }

install_ctdev() {
    mkdir -p "$INSTALL_DIR"
    if [[ -n "$SCRIPT_DIR" && -f "$SCRIPT_DIR/ctdev/go.mod" ]] && have_go; then
        local version
        version="$(cat "$SCRIPT_DIR/VERSION" 2>/dev/null || echo dev)-local"
        info "Building ctdev from local source..."
        ( cd "$SCRIPT_DIR/ctdev" && "$(go_binary)" build -ldflags "-X main.version=${version}" -o "$INSTALL_DIR/ctdev" . )
    else
        info "Installing ctdev from latest release..."
        curl -fsSL "$RAW_URL/install.sh" | bash
    fi
    export PATH="$INSTALL_DIR:$PATH"
    command -v ctdev >/dev/null 2>&1 || die "ctdev not found on PATH after install"
    success "ctdev ready"
}

set_hostname() {
    if [[ "$(hostname)" != "$NODE" ]]; then
        info "Setting hostname → ${NODE}..."
        sudo hostnamectl set-hostname "$NODE"
        sudo sed -i "s/\b$(hostname)\b/${NODE}/g" /etc/hosts || true
        if command -v tailscale >/dev/null 2>&1; then
            sudo tailscale set --hostname="$NODE" 2>/dev/null || true
        fi
        success "Hostname is ${NODE}"
    fi
}

install_components() {
    info "Installing components: ${PRE_COMPONENTS[*]}"
    ctdev install "${PRE_COMPONENTS[@]}"
    ctdev configure remote --batch
    success "Components installed, remote access configured"
}

install_homelab() {
    if [[ ! -f "$AGE_KEY" ]]; then
        warn "age key not found at ${AGE_KEY} — skipping the homelab stack."
        warn "Copy the homelab age private key there, then run: CTDEV_HOMELAB_HOST=${NODE} ctdev install homelab"
        return
    fi
    info "Bringing up the homelab stack (decrypt config, deploy Caddy, wire Pi-hole)..."
    CTDEV_HOMELAB_HOST="$NODE" ctdev install homelab
    success "Homelab stack up"
}

print_manual_steps() {
    cat <<EOF

────────────────────────────────────────────────────────────
  Homelab bootstrap complete for ${NODE}. Remaining steps:
────────────────────────────────────────────────────────────
  1. Tailscale:   sudo tailscale up
       Then finish DNS wiring (needs the Tailscale IP):
       CTDEV_HOMELAB_HOST=${NODE} ctdev install homelab --force
  2. In the Tailscale admin console (DNS): set this node's
     Tailscale IP as a Global Nameserver with Override ON, so
     all devices use Pi-hole.
  3. Pi-hole admin password:   sudo pihole setpassword
  4. Add your SSH key, then:   ctdev configure remote --batch
  5. Verify:                   ctdev verify

  Services are reachable at https://<name>.<your domain>
  (Pi-hole at https://pihole.<your domain>/admin).
────────────────────────────────────────────────────────────
EOF
}

main() {
    require_linux_apt
    require_host_config
    prime_sudo
    install_base_packages
    install_ctdev
    set_hostname
    install_components
    install_homelab
    print_manual_steps
}

main "$@"
