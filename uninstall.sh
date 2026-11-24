#!/usr/bin/env bash
set -euo pipefail

# Uninstall ctdev
# Usage: ./uninstall.sh [--purge]
#
# Removes the binary, the scheduled-backup runner and ctdev's own config and state.
# It keeps ~/.config/ctdev/backup, which holds this computer's backup destinations and
# its key to them: without it this computer can't open its backups. --purge removes it too.

GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}==>${NC} $1"; }
success() { echo -e "${GREEN}[✓]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }

PURGE=false
for arg in "$@"; do
    case "$arg" in
        --purge) PURGE=true ;;
        -h|--help)
            echo "Usage: uninstall.sh [--purge]"
            echo "  --purge   Also remove ~/.config/ctdev/backup, this computer's backup keys."
            exit 0
            ;;
        *)
            echo "Unknown option: $arg (see --help)" >&2
            exit 1
            ;;
    esac
done

echo
echo "  ┌─────────────────────────────────────┐"
echo "  │  ctdev uninstaller                  │"
echo "  └─────────────────────────────────────┘"
echo

removed=false

# The scheduled-backup runner first, so nothing fires against a binary that is gone. The
# unit and agent names are the ones ctdev/backup/scheduler.go installs.
unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
if [[ -f "$unit_dir/ctdev-backup.timer" || -f "$unit_dir/ctdev-backup.service" ]]; then
    info "Removing the scheduled-backup timer..."
    if command -v systemctl >/dev/null 2>&1; then
        systemctl --user disable --now ctdev-backup.timer 2>/dev/null || true
    fi
    rm -f "$unit_dir/ctdev-backup.timer" "$unit_dir/ctdev-backup.service"
    if command -v systemctl >/dev/null 2>&1; then
        systemctl --user daemon-reload 2>/dev/null || true
    fi
    removed=true
fi

agent="$HOME/Library/LaunchAgents/io.connertechnology.ctdev.backup.plist"
if [[ -f "$agent" ]]; then
    info "Removing the scheduled-backup launch agent..."
    launchctl bootout "gui/$(id -u)/io.connertechnology.ctdev.backup" 2>/dev/null || true
    rm -f "$agent" "$HOME/Library/Logs/ctdev-backup.log"
    removed=true
fi

# Every place install.sh can put the binary: INSTALL_DIR when set, then its default, then
# the system-wide location.
seen=":"
for dir in "${INSTALL_DIR:-}" "$HOME/.local/bin" "/usr/local/bin"; do
    [[ -n "$dir" ]] || continue
    loc="${dir%/}/ctdev"
    [[ "$seen" == *":$loc:"* ]] && continue
    seen="$seen$loc:"
    if [[ -f "$loc" ]] || [[ -L "$loc" ]]; then
        info "Removing $loc..."
        if [[ -w "$dir" ]]; then
            rm -f "$loc"
        else
            sudo rm -f "$loc" 2>/dev/null || warn "Could not remove $loc — run: sudo rm $loc"
        fi
        removed=true
    fi
done

# State directory
STATE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/ctdev"
if [[ -d "$STATE_DIR" ]]; then
    info "Removing state directory..."
    rm -rf "$STATE_DIR"
    success "Removed $STATE_DIR"
fi

# Config directory, except the backup keys unless --purge.
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/ctdev"
if [[ -d "$CONFIG_DIR" ]]; then
    if [[ "$PURGE" == true ]]; then
        info "Removing config directory, including the backup keys..."
        rm -rf "$CONFIG_DIR"
        success "Removed $CONFIG_DIR"
    else
        info "Removing config directory..."
        find "$CONFIG_DIR" -mindepth 1 -maxdepth 1 ! -name backup -exec rm -rf {} +
        if [[ -d "$CONFIG_DIR/backup" ]]; then
            warn "Kept $CONFIG_DIR/backup: this computer's backup destinations and keys."
            warn "Run ./uninstall.sh --purge to remove it too."
        else
            rmdir "$CONFIG_DIR" 2>/dev/null || true
        fi
        success "Removed ctdev's config"
    fi
fi

if [[ "$removed" == true ]]; then
    echo
    success "ctdev has been uninstalled"
else
    info "ctdev was not found in any known location"
fi
echo
