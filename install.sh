#!/usr/bin/env bash
set -euo pipefail

# Install ctdev - development environment manager
# Usage: curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/install.sh | bash

REPO="ConnerTechnology/dotfiles"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}==>${NC} $1"; }
success() { echo -e "${GREEN}[✓]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
error() { echo -e "${RED}[✗]${NC} $1"; exit 1; }

detect_platform() {
    local os arch
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    arch=$(uname -m)

    case "$os" in
        linux)  os="linux" ;;
        darwin) os="darwin" ;;
        *)      error "Unsupported OS: $os" ;;
    esac

    case "$arch" in
        x86_64|amd64)  arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *)             error "Unsupported architecture: $arch" ;;
    esac

    echo "${os}-${arch}"
}

get_latest_version() {
    local url="https://api.github.com/repos/${REPO}/releases/latest"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$url" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//'
    elif command -v wget >/dev/null 2>&1; then
        wget -qO- "$url" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//'
    else
        error "curl or wget is required"
    fi
}

download() {
    local url="$1" dest="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL -o "$dest" "$url"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$dest" "$url"
    else
        error "curl or wget is required"
    fi
}

echo
echo "  ┌─────────────────────────────────────┐"
echo "  │  ctdev installer                    │"
echo "  └─────────────────────────────────────┘"
echo

# Detect platform
PLATFORM=$(detect_platform)
info "Detected platform: $PLATFORM"

# Get latest version
info "Checking latest release..."
VERSION=$(get_latest_version)
if [[ -z "$VERSION" ]]; then
    error "Could not determine latest version. Check https://github.com/${REPO}/releases"
fi
info "Latest version: $VERSION"

# Clean up old bash-based install
if [[ -L "$INSTALL_DIR/ctdev" ]]; then
    old_target=$(readlink "$INSTALL_DIR/ctdev" 2>/dev/null || true)
    if [[ "$old_target" == */dotfiles/ctdev ]] || [[ "$old_target" == */dotfiles/ctdev.sh ]]; then
        info "Removing old bash ctdev symlink..."
        rm -f "$INSTALL_DIR/ctdev"
    fi
fi
if [[ -f "/usr/local/bin/ctdev" ]] || [[ -L "/usr/local/bin/ctdev" ]]; then
    # sudo -n only: a script running from a curl|bash pipe should never sit on a
    # password prompt (and training people to type sudo passwords into piped
    # scripts is its own problem).
    info "Removing old ctdev from /usr/local/bin..."
    sudo -n rm -f /usr/local/bin/ctdev 2>/dev/null || warn "Could not remove /usr/local/bin/ctdev (needs sudo) — run: sudo rm /usr/local/bin/ctdev"
fi

# Create install directory
mkdir -p "$INSTALL_DIR"

# Download binary
BINARY_URL="https://github.com/${REPO}/releases/download/${VERSION}/ctdev-${PLATFORM}"
info "Downloading ctdev-${PLATFORM}..."

# Download into INSTALL_DIR so the final mv is a same-filesystem rename —
# atomic, so a crash mid-install can't leave a truncated binary in place.
TMP=$(mktemp "$INSTALL_DIR/.ctdev-download.XXXXXX")
trap 'rm -f "$TMP"' EXIT

if ! download "$BINARY_URL" "$TMP"; then
    error "Download failed. Check that a release exists for $PLATFORM at:\n  https://github.com/${REPO}/releases/tag/${VERSION}"
fi

# Verify checksum against the release's SHA256SUMS. Fail closed: a missing
# SUMS file or sha256sum binary aborts the install — anyone who can tamper
# with the download path could otherwise just break the SUMS fetch to skip
# verification. CTDEV_SKIP_VERIFY=1 is the explicit escape hatch for old
# releases that predate checksum publishing.
SUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/SHA256SUMS"
SUMS_TMP=$(mktemp)
trap 'rm -f "$TMP" "$SUMS_TMP"' EXIT
if [[ "${CTDEV_SKIP_VERIFY:-}" == "1" ]]; then
    warn "CTDEV_SKIP_VERIFY=1 — skipping checksum verification"
else
    if ! command -v sha256sum >/dev/null 2>&1; then
        error "sha256sum is required to verify the download (or set CTDEV_SKIP_VERIFY=1 to skip)"
    fi
    if ! download "$SUMS_URL" "$SUMS_TMP" 2>/dev/null; then
        error "Could not fetch SHA256SUMS for ${VERSION} — refusing to install an unverified binary.\n  Retry, or set CTDEV_SKIP_VERIFY=1 if this release predates checksums."
    fi
    expected=$(grep "ctdev-${PLATFORM}\$" "$SUMS_TMP" | awk '{print $1}')
    if [[ -z "$expected" ]]; then
        error "No checksum entry for ctdev-${PLATFORM} in SHA256SUMS — refusing to install (or set CTDEV_SKIP_VERIFY=1)"
    fi
    actual=$(sha256sum "$TMP" | awk '{print $1}')
    if [[ "$expected" != "$actual" ]]; then
        error "Checksum mismatch for ctdev-${PLATFORM}.\n  expected: $expected\n  actual:   $actual"
    fi
    info "Checksum verified"
fi

# Install
chmod +x "$TMP"
mv "$TMP" "$INSTALL_DIR/ctdev"
rm -f "$SUMS_TMP"
trap - EXIT

success "ctdev ${VERSION} installed to $INSTALL_DIR/ctdev"

# Check PATH
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    warn "$INSTALL_DIR is not in your PATH"
    echo
    echo "  Add this to your shell profile (~/.bashrc or ~/.zshrc):"
    echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
    echo
fi

# Verify
if command -v ctdev >/dev/null 2>&1; then
    success "Ready! Run 'ctdev --help' to get started."
else
    success "Installed! Restart your terminal, then run 'ctdev --help'."
fi
echo
