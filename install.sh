#!/usr/bin/env bash
set -euo pipefail

# Install ctdev - development environment manager
# Usage: curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/ctdev/main/install.sh | bash
#
# Or, to check a machine without installing anything at all:
#   curl -fsSL .../install.sh | bash -s -- --doctor
#
# Or, to install one exact release system-wide (as root, e.g. a boot script):
#   CTDEV_VERSION=v1.2.3 INSTALL_DIR=/usr/local/bin bash install.sh

REPO="ConnerTechnology/ctdev"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# EPHEMERAL runs ctdev from a temporary directory and deletes it afterwards,
# for machines you're only visiting. It touches no install directory, changes
# no PATH, and never calls sudo.
EPHEMERAL=0
DOCTOR_ARGS=()

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# In ephemeral mode the diagnostic report owns stdout, so installer chatter
# goes to stderr — that way `| tee report.txt` captures the report alone.
log() {
    if [[ $EPHEMERAL -eq 1 ]]; then
        echo -e "$1" >&2
    else
        echo -e "$1"
    fi
}

info() { log "${BLUE}==>${NC} $1"; }
success() { log "${GREEN}[✓]${NC} $1"; }
warn() { log "${YELLOW}[!]${NC} $1"; }
error() { echo -e "${RED}[✗]${NC} $1" >&2; exit 1; }

usage() {
    cat <<'EOF'
Usage: install.sh [--doctor [doctor options]]

Installs the ctdev binary. With no arguments it installs the latest release
to ~/.local/bin. If the binary already there reports the version being
installed, it exits without downloading anything.

Options:
  --doctor [args]    Don't install. Download ctdev to a temporary directory,
                     run 'ctdev doctor', then delete it. Nothing is left
                     behind and sudo is never used. Any following arguments
                     are passed to doctor (--deep, --report, --redact).
  -h, --help         Show this help.

Environment:
  INSTALL_DIR        Where to install (default: ~/.local/bin)
  CTDEV_VERSION      Release to install, e.g. v17.1.0 (default: latest)
  CTDEV_SKIP_VERIFY  Set to 1 to skip checksum verification
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --doctor)
            EPHEMERAL=1
            shift
            # Everything after --doctor belongs to doctor, including
            # anything that looks like an installer flag.
            DOCTOR_ARGS=("$@")
            break
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            error "Unknown option: $1\n  Run with --help for usage."
            ;;
    esac
done

# One cleanup path for every temporary thing, so the traps can't clobber each
# other as the script sets more of them up.
TMP=""
SUMS_TMP=""
RUN_DIR=""
cleanup() {
    if [[ -n "$TMP" ]]; then rm -f "$TMP"; fi
    if [[ -n "$SUMS_TMP" ]]; then rm -f "$SUMS_TMP"; fi
    if [[ -n "$RUN_DIR" ]]; then rm -rf "$RUN_DIR"; fi
    return 0
}
trap cleanup EXIT

if [[ $EPHEMERAL -eq 1 ]]; then
    RUN_DIR=$(mktemp -d)
    INSTALL_DIR="$RUN_DIR"
fi

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

if [[ $EPHEMERAL -eq 0 ]]; then
    echo
    echo "  ┌─────────────────────────────────────┐"
    echo "  │  ctdev installer                    │"
    echo "  └─────────────────────────────────────┘"
    echo
fi

# Detect platform
PLATFORM=$(detect_platform)
info "Detected platform: $PLATFORM"

if [[ -n "${CTDEV_VERSION:-}" ]]; then
    # The value goes straight into download URLs, so accept a release tag
    # shape and nothing else.
    if [[ ! "$CTDEV_VERSION" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        error "CTDEV_VERSION must look like v1.2.3, got: $CTDEV_VERSION"
    fi
    VERSION="v${CTDEV_VERSION#v}"
    info "Requested version: $VERSION"
else
    info "Checking latest release..."
    VERSION=$(get_latest_version)
    if [[ -z "$VERSION" ]]; then
        error "Could not determine latest version. Check https://github.com/${REPO}/releases"
    fi
    info "Latest version: $VERSION"
fi

# Clean up old bash-based install. Skipped entirely when running ephemerally:
# removing things from someone else's machine is exactly what --doctor
# promises not to do.
if [[ $EPHEMERAL -eq 0 ]]; then
    if [[ -L "$INSTALL_DIR/ctdev" ]]; then
        old_target=$(readlink "$INSTALL_DIR/ctdev" 2>/dev/null || true)
        if [[ "$old_target" == */dotfiles/ctdev ]] || [[ "$old_target" == */dotfiles/ctdev.sh ]]; then
            info "Removing old bash ctdev symlink..."
            rm -f "$INSTALL_DIR/ctdev"
        fi
    fi
    # /usr/local/bin/ctdev is a leftover only when it isn't the install
    # target. Root installing system-wide (a cloud node's boot script, or
    # `ctdev update` of a system install) puts it there on purpose.
    if [[ "${INSTALL_DIR%/}" != "/usr/local/bin" ]] && { [[ -f "/usr/local/bin/ctdev" ]] || [[ -L "/usr/local/bin/ctdev" ]]; }; then
        # sudo -n only: a script running from a curl|bash pipe should never sit on a
        # password prompt (and training people to type sudo passwords into piped
        # scripts is its own problem).
        info "Removing old ctdev from /usr/local/bin..."
        sudo -n rm -f /usr/local/bin/ctdev 2>/dev/null || warn "Could not remove /usr/local/bin/ctdev (needs sudo) — run: sudo rm /usr/local/bin/ctdev"
    fi
fi

# Already there? A boot script runs this on every boot, and re-downloading a
# binary it already has is wasted time and one more way to fail. Ephemeral runs
# start from an empty temp directory, so there is never anything to find.
if [[ $EPHEMERAL -eq 0 ]] && [[ -x "$INSTALL_DIR/ctdev" ]]; then
    installed=$("$INSTALL_DIR/ctdev" --version 2>/dev/null | awk '{print $NF}' || true)
    if [[ "$installed" == "${VERSION#v}" ]]; then
        success "ctdev ${VERSION} is already installed at $INSTALL_DIR/ctdev"
        exit 0
    fi
fi

# Create install directory
mkdir -p "$INSTALL_DIR"

# Download binary
BINARY_URL="https://github.com/${REPO}/releases/download/${VERSION}/ctdev-${PLATFORM}"
info "Downloading ctdev-${PLATFORM}..."

# Download into INSTALL_DIR so the final mv is a same-filesystem rename —
# atomic, so a crash mid-install can't leave a truncated binary in place.
TMP=$(mktemp "$INSTALL_DIR/.ctdev-download.XXXXXX")

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
TMP=""
rm -f "$SUMS_TMP"
SUMS_TMP=""

# Ephemeral: run from the temp directory and let the EXIT trap delete it.
# Nothing was installed, no PATH was touched, and the exit status is whatever
# doctor reported.
if [[ $EPHEMERAL -eq 1 ]]; then
    info "Running doctor (nothing is installed)..."
    log ""
    # The ${arr[@]+...} form keeps this working under `set -u` with an empty
    # array on bash 3.2, which is still what ships on macOS.
    "$INSTALL_DIR/ctdev" doctor ${DOCTOR_ARGS[@]+"${DOCTOR_ARGS[@]}"}
    exit $?
fi

trap - EXIT
RUN_DIR=""

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
