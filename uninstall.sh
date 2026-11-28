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
config_roots=("$HOME/.config")
if [[ "${XDG_CONFIG_HOME:-}" == /* && "$XDG_CONFIG_HOME" != "$HOME/.config" ]]; then
    config_roots+=("$XDG_CONFIG_HOME")
fi

for config_root in "${config_roots[@]}"; do
    unit_dir="$config_root/systemd/user"
    enable_link="$unit_dir/timers.target.wants/ctdev-backup.timer"
    if [[ -f "$unit_dir/ctdev-backup.timer" || -f "$unit_dir/ctdev-backup.service" || -L "$enable_link" ]]; then
        info "Removing the scheduled-backup timer..."
        if command -v systemctl >/dev/null 2>&1; then
            systemctl --user disable --now ctdev-backup.timer 2>/dev/null || true
        fi
        rm -f "$unit_dir/ctdev-backup.timer" "$unit_dir/ctdev-backup.service" "$enable_link"
        if command -v systemctl >/dev/null 2>&1; then
            systemctl --user daemon-reload 2>/dev/null || true
        fi
        removed=true
    fi
done

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

# Backup credentials live under ~/.config even when systemd reads its units
# from XDG_CONFIG_HOME. Preserve backup directories in both roots unless purging.
for config_root in "${config_roots[@]}"; do
    CONFIG_DIR="$config_root/ctdev"
    [[ -d "$CONFIG_DIR" ]] || continue
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
    removed=true
done

if [[ "$removed" == true ]]; then
    echo
    success "ctdev has been uninstalled"
else
    info "ctdev was not found in any known location"
fi
echo
