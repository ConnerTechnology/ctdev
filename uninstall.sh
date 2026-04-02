#!/usr/bin/env bash
set -euo pipefail

# Uninstall ctdev
# Usage: ./uninstall.sh

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}==>${NC} $1"; }
success() { echo -e "${GREEN}[✓]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }

echo
echo "  ┌─────────────────────────────────────┐"
echo "  │  ctdev uninstaller                  │"
echo "  └─────────────────────────────────────┘"
echo

removed=false

# Check both known install locations
for loc in "$HOME/.local/bin/ctdev" "/usr/local/bin/ctdev"; do
    if [[ -f "$loc" ]] || [[ -L "$loc" ]]; then
        info "Removing $loc..."
        if [[ "$loc" == /usr/local/* ]]; then
            sudo rm -f "$loc" 2>/dev/null || warn "Could not remove $loc — run: sudo rm $loc"
        else
            rm -f "$loc"
        fi
        removed=true
    fi
done

# Remove state directory
STATE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/ctdev"
if [[ -d "$STATE_DIR" ]]; then
    info "Removing state directory..."
    rm -rf "$STATE_DIR"
    success "Removed $STATE_DIR"
fi

# Remove config directory
CONFIG_DIR="$HOME/.config/ctdev"
if [[ -d "$CONFIG_DIR" ]]; then
    info "Removing config directory..."
    rm -rf "$CONFIG_DIR"
    success "Removed $CONFIG_DIR"
fi

if [[ "$removed" == true ]]; then
    echo
    success "ctdev has been uninstalled"
else
    info "ctdev was not found in any known location"
fi
echo
