#!/usr/bin/env bash
set -euo pipefail

# bootstrap.sh — set up a fresh Linux Mint (Ubuntu 24.04 base) machine.
#
# Thin orchestrator: installs base apt packages, gets the ctdev binary, then
# delegates everything else to ctdev (components + `configure remote`).
#
# Run on a fresh machine:
#   bash <(curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/bootstrap.sh)
# Or from a clone (builds ctdev from source when Go is present):
#   ./bootstrap.sh

REPO="ConnerTechnology/dotfiles"
RAW_URL="https://raw.githubusercontent.com/${REPO}/main"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# Directory this script lives in — empty when piped via curl/process substitution.
SCRIPT_DIR=""
if [[ -n "${BASH_SOURCE[0]:-}" && -f "${BASH_SOURCE[0]}" ]]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fi

# Base packages ctdev itself or the toolchain needs, plus the remote-access stack.
BASE_PACKAGES=(git curl wget build-essential zsh tmux mosh openssh-server ufw htop jq ripgrep fd-find unzip)

# Components installed via ctdev (idempotent; each has its own command-exists guard).
CTDEV_COMPONENTS=(zsh git gh node go docker tailscale vscode claude-code tmux jq devcontainer)

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'; BLUE='\033[0;34m'; NC='\033[0m'
info()    { echo -e "${BLUE}[bootstrap]${NC} $1"; }
success() { echo -e "${GREEN}[bootstrap ✓]${NC} $1"; }
warn()    { echo -e "${YELLOW}[bootstrap !]${NC} $1"; }
die()     { echo -e "${RED}[bootstrap ✗]${NC} $1" >&2; exit 1; }

trap 'die "failed at line ${LINENO}"' ERR

require_linux_apt() {
    [[ "$(uname -s)" == "Linux" ]] || die "bootstrap.sh targets Linux (Mint/Ubuntu). Detected: $(uname -s)"
    command -v apt-get >/dev/null 2>&1 || die "apt-get not found — this script supports apt-based distros (Linux Mint / Ubuntu)."
}

# prime_sudo prompts for the password once and keeps the credential warm for the
# rest of the run, so nothing prompts again mid-bootstrap.
prime_sudo() {
    info "Requesting sudo (you'll be prompted once)..."
    sudo -v || die "sudo is required"
    ( while true; do sudo -n true || true; sleep 50; kill -0 "$$" 2>/dev/null || exit; done ) 2>/dev/null &
}

apt_update() {
    info "Updating apt package lists..."
    sudo apt-get update -qq
}

install_base_packages() {
    local missing=() pkg
    for pkg in "${BASE_PACKAGES[@]}"; do
        dpkg -s "$pkg" >/dev/null 2>&1 || missing+=("$pkg")
    done
    if [[ ${#missing[@]} -eq 0 ]]; then
        success "Base packages already installed"
        return
    fi
    info "Installing base packages: ${missing[*]}"
    sudo apt-get install -y -qq "${missing[@]}"
    success "Base packages installed"
}

enable_sshd() {
    info "Enabling SSH server..."
    sudo systemctl enable --now ssh
    success "sshd enabled"
}

have_go()   { command -v go >/dev/null 2>&1 || [[ -x /usr/local/go/bin/go ]]; }
go_binary() { if command -v go >/dev/null 2>&1; then command -v go; else echo /usr/local/go/bin/go; fi; }

# install_ctdev builds from a local clone when one is present and Go is
# available (Option B — lets you test before cutting a release); otherwise it
# downloads the latest released binary.
install_ctdev() {
    mkdir -p "$INSTALL_DIR"
    if [[ -n "$SCRIPT_DIR" && -f "$SCRIPT_DIR/ctdev/go.mod" ]] && have_go; then
        local gobin version
        gobin="$(go_binary)"
        version="$(cat "$SCRIPT_DIR/VERSION" 2>/dev/null || echo dev)-local"
        info "Building ctdev from local source ($SCRIPT_DIR/ctdev)..."
        ( cd "$SCRIPT_DIR/ctdev" && "$gobin" build -ldflags "-X main.version=${version}" -o "$INSTALL_DIR/ctdev" . )
        success "Built ctdev → $INSTALL_DIR/ctdev"
    else
        info "Installing ctdev from latest release..."
        curl -fsSL "$RAW_URL/install.sh" | bash
    fi
    export PATH="$INSTALL_DIR:$PATH"
    command -v ctdev >/dev/null 2>&1 || die "ctdev not found on PATH after install"
}

install_components() {
    info "Installing components via ctdev: ${CTDEV_COMPONENTS[*]}"
    ctdev install "${CTDEV_COMPONENTS[@]}"
    success "Components installed"
}

configure_remote() {
    info "Configuring remote access (ctdev configure remote)..."
    ctdev configure remote --batch
    success "Remote access configured"
}

print_manual_steps() {
    cat <<'EOF'

────────────────────────────────────────────────────────────
  Bootstrap complete. Remaining manual steps:
────────────────────────────────────────────────────────────
  1. Add your iPad's SSH public key, then re-run configure:
       echo 'ssh-ed25519 AAAA... your-ipad' >> ~/.ssh/authorized_keys
       ctdev configure remote --batch
     (password auth is only disabled once a key is present)

  2. VS Code on iPad (Safari → vscode.dev):
       code tunnel user login    # one-time GitHub/Microsoft auth

  3. GitHub auth:   gh auth login
  4. Git identity:  ctdev configure git
  5. (optional)     sudo tailscale up

  Reboot (or log out/in) to apply: docker group membership,
  suspend masking, and WiFi power-save settings.

  Verify everything:  ctdev verify
────────────────────────────────────────────────────────────
EOF
}

main() {
    require_linux_apt
    prime_sudo
    apt_update
    install_base_packages
    enable_sshd
    install_ctdev
    install_components
    configure_remote
    print_manual_steps
}

main "$@"
