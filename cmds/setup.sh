#!/usr/bin/env bash

# ctdev setup - Orchestrate a fresh system setup

OS=$(detect_os)

###############################################################################
# Help
###############################################################################

show_setup_help() {
    cat << 'EOF'
ctdev setup - Set up a fresh system

Usage: ctdev setup [OPTIONS]

Runs all setup steps in the correct order for the current OS.

Linux Mint steps:
    1. GPU driver signing setup (NVIDIA + Secure Boot)
    2. System configuration (GRUB, services, desktop settings,
       Bluetooth/audio/camera packages, WirePlumber LDAC config,
       xbindkeys mouse bindings)

macOS steps:
    1. System configuration (defaults)

Options:
    -h, --help           Show this help message
    -n, --dry-run        Preview changes without applying
    -f, --force          Force re-run even if already configured
    --show               Show current system configuration and exit
    --reset              Reset system configuration to defaults and exit
    --skip-gpu           Skip GPU setup step (Linux only)
    --skip-configure     Skip system configuration step

Examples:
    ctdev setup                     Run full setup
    ctdev setup --dry-run           Preview all changes
    ctdev setup --show              Show current system configuration
    ctdev setup --reset             Reset system configuration to defaults
    ctdev setup --skip-gpu          Skip GPU signing setup
EOF
}

###############################################################################
# GRUB helpers
###############################################################################

# Set a GRUB variable in /etc/default/grub
# Usage: set_grub_var "GRUB_TIMEOUT" "0"
set_grub_var() {
    local var="$1"
    local value="$2"
    local grub_file="/etc/default/grub"
    [[ -f "$grub_file" ]] || return 1

    if grep -q "^${var}=" "$grub_file"; then
        maybe_sudo sed -i "s/^${var}=.*/${var}=${value}/" "$grub_file"
    else
        echo "${var}=${value}" | maybe_sudo tee -a "$grub_file" > /dev/null
    fi
}

# Add a parameter to GRUB_CMDLINE_LINUX_DEFAULT if not already present
add_grub_cmdline_param() {
    local param="$1"
    local grub_file="/etc/default/grub"
    [[ -f "$grub_file" ]] || return 1

    if ! grep -q "$param" "$grub_file"; then
        maybe_sudo sed -i "s/\\(GRUB_CMDLINE_LINUX_DEFAULT=\"[^\"]*\\)\"/\\1 ${param}\"/" "$grub_file"
    fi
}

# Remove a parameter from GRUB_CMDLINE_LINUX_DEFAULT
remove_grub_cmdline_param() {
    local param="$1"
    local grub_file="/etc/default/grub"
    [[ -f "$grub_file" ]] || return 1

    maybe_sudo sed -i "s/ ${param}//" "$grub_file"
}

###############################################################################
# macOS configuration
###############################################################################

macos_show() {
    echo ""
    log_info "macOS Configuration"
    echo ""

    format_macos_bool() {
        case "$1" in
            1|true) echo "yes" ;;
            0|false) echo "no" ;;
            *) echo "$1" ;;
        esac
    }

    format_seconds() {
        local val="$1"
        if [[ "$val" == "<system default>" ]]; then
            echo "$val"
            return
        fi
        if ! [[ "$val" =~ ^[0-9]+$ ]]; then
            echo "$val"
            return
        fi
        if (( val >= 3600 && val % 3600 == 0 )); then
            echo "$(( val / 3600 )) hr"
        elif (( val >= 60 && val % 60 == 0 )); then
            echo "$(( val / 60 )) min"
        elif (( val >= 60 )); then
            echo "$(( val / 60 )) min $(( val % 60 )) sec"
        else
            echo "${val} sec"
        fi
    }

    format_search_scope() {
        case "$1" in
            SCcf) echo "current folder" ;;
            SCsp) echo "previous scope" ;;
            SCev) echo "this Mac" ;;
            *) echo "$1" ;;
        esac
    }

    format_view_style() {
        case "$1" in
            Nlsv) echo "list" ;;
            icnv) echo "icon" ;;
            clmv) echo "column" ;;
            glyv) echo "gallery" ;;
            *) echo "$1" ;;
        esac
    }

    # show_default DOMAIN KEY LABEL [FORMAT]
    # FORMAT: raw (default), bool, seconds, float_seconds, search_scope, view_style
    show_default() {
        local domain="$1"
        local key="$2"
        local label="$3"
        local format="${4:-raw}"
        local value
        value=$(defaults read "$domain" "$key" 2>/dev/null || echo "<system default>")
        case "$format" in
            bool) value=$(format_macos_bool "$value") ;;
            seconds) value=$(format_seconds "$value") ;;
            float_seconds)
                [[ "$value" != "<system default>" ]] && value="${value} sec"
                ;;
            search_scope) value=$(format_search_scope "$value") ;;
            view_style) value=$(format_view_style "$value") ;;
        esac
        printf "  %-40s %s\n" "$label:" "$value"
    }

    echo "Dock:"
    show_default "com.apple.dock" "autohide" "Auto-hide" bool
    show_default "com.apple.dock" "launchanim" "Launch animation" bool
    show_default "com.apple.dock" "show-recents" "Show recent apps" bool
    echo ""

    echo "Sound:"
    show_default "NSGlobalDomain" "com.apple.sound.beep.feedback" "Volume change feedback" bool
    echo ""

    echo "Finder:"
    show_default "com.apple.finder" "ShowPathbar" "Show path bar" bool
    show_default "com.apple.finder" "ShowStatusBar" "Show status bar" bool
    show_default "com.apple.desktopservices" "DSDontWriteNetworkStores" "No .DS_Store on network" bool
    show_default "com.apple.desktopservices" "DSDontWriteUSBStores" "No .DS_Store on USB" bool
    show_default "com.apple.finder" "FXDefaultSearchScope" "Default search scope" search_scope
    show_default "com.apple.finder" "FXPreferredViewStyle" "Preferred view style" view_style
    show_default "com.apple.finder" "QuitMenuItem" "Allow quit" bool
    echo ""

    echo "Keyboard:"
    show_default "NSGlobalDomain" "NSAutomaticQuoteSubstitutionEnabled" "Smart quotes" bool
    show_default "NSGlobalDomain" "NSAutomaticDashSubstitutionEnabled" "Smart dashes" bool
    show_default "NSGlobalDomain" "NSAutomaticSpellingCorrectionEnabled" "Auto-correct" bool
    show_default "NSGlobalDomain" "NSAutomaticCapitalizationEnabled" "Auto-capitalize" bool
    show_default "NSGlobalDomain" "NSAutomaticPeriodSubstitutionEnabled" "Double-space period" bool
    show_default "NSGlobalDomain" "KeyRepeat" "Key repeat rate"
    show_default "NSGlobalDomain" "InitialKeyRepeat" "Initial key repeat delay"
    echo ""

    echo "Dialogs:"
    show_default "NSGlobalDomain" "NSNavPanelExpandedStateForSaveMode" "Expand save dialogs" bool
    show_default "NSGlobalDomain" "NSNavPanelExpandedStateForSaveMode2" "Expand save dialogs 2" bool
    show_default "NSGlobalDomain" "PMPrintingExpandedStateForPrint" "Expand print dialogs" bool
    show_default "NSGlobalDomain" "PMPrintingExpandedStateForPrint2" "Expand print dialogs 2" bool
    echo ""

    echo "Security:"
    show_default "com.apple.screensaver" "askForPassword" "Require password" bool
    show_default "com.apple.screensaver" "askForPasswordDelay" "Password delay" seconds
    echo ""
}

macos_apply() {
    log_step "Configuring macOS System Defaults"

    if [[ "${DRY_RUN:-false}" == "true" ]]; then
        log_info "[DRY-RUN] Would configure Dock settings (auto-hide, animations, recent apps)"
        log_info "[DRY-RUN] Would configure Sound settings (volume change feedback)"
        log_info "[DRY-RUN] Would configure Finder settings (path bar, status bar)"
        log_info "[DRY-RUN] Would configure Keyboard settings (disable smart quotes/dashes)"
        log_info "[DRY-RUN] Would configure Dialog settings (expand save/print dialogs)"
        log_info "[DRY-RUN] Would configure Security settings (require password after sleep)"
        log_info "[DRY-RUN] Would restart Dock and Finder"
        log_success "macOS defaults would be configured"
        return 0
    fi

    # Dock Settings
    log_info "Configuring Dock..."
    defaults write com.apple.dock autohide -bool true
    defaults write com.apple.dock launchanim -bool false
    defaults write com.apple.dock show-recents -bool false

    # Sound Settings
    log_info "Configuring Sound..."
    defaults write NSGlobalDomain com.apple.sound.beep.feedback -bool true

    # Finder Settings
    log_info "Configuring Finder..."
    defaults write com.apple.finder ShowPathbar -bool true
    defaults write com.apple.finder ShowStatusBar -bool true
    defaults write com.apple.desktopservices DSDontWriteNetworkStores -bool true
    defaults write com.apple.desktopservices DSDontWriteUSBStores -bool true
    defaults write com.apple.finder FXDefaultSearchScope -string "SCcf"
    defaults write com.apple.finder FXPreferredViewStyle -string "Nlsv"
    defaults write com.apple.finder QuitMenuItem -bool true

    # Keyboard Settings
    log_info "Configuring Keyboard..."
    defaults write NSGlobalDomain NSAutomaticQuoteSubstitutionEnabled -bool false
    defaults write NSGlobalDomain NSAutomaticDashSubstitutionEnabled -bool false
    defaults write NSGlobalDomain NSAutomaticSpellingCorrectionEnabled -bool false
    defaults write NSGlobalDomain NSAutomaticCapitalizationEnabled -bool false
    defaults write NSGlobalDomain NSAutomaticPeriodSubstitutionEnabled -bool false
    defaults write NSGlobalDomain KeyRepeat -int 2
    defaults write NSGlobalDomain InitialKeyRepeat -int 15

    # Dialog Settings
    log_info "Configuring Dialogs..."
    defaults write NSGlobalDomain NSNavPanelExpandedStateForSaveMode -bool true
    defaults write NSGlobalDomain NSNavPanelExpandedStateForSaveMode2 -bool true
    defaults write NSGlobalDomain PMPrintingExpandedStateForPrint -bool true
    defaults write NSGlobalDomain PMPrintingExpandedStateForPrint2 -bool true

    # Security Settings
    log_info "Configuring Security..."
    defaults write com.apple.screensaver askForPassword -int 1
    defaults write com.apple.screensaver askForPasswordDelay -int 0

    # Apply changes
    log_info "Applying changes..."
    killall Dock 2>/dev/null || true
    killall Finder 2>/dev/null || true

    log_success "macOS defaults configured"
    log_info "Some settings may require logout/restart to take full effect"
}

macos_reset() {
    log_step "Resetting macOS System Defaults"

    if [[ "${DRY_RUN:-false}" == "true" ]]; then
        log_info "[DRY-RUN] Would reset Dock, Finder, Keyboard, Dialog, Security settings"
        log_info "[DRY-RUN] Would restart Dock and Finder"
        log_success "macOS defaults would be reset"
        return 0
    fi

    log_info "Resetting Dock settings..."
    defaults delete com.apple.dock autohide 2>/dev/null || true
    defaults delete com.apple.dock launchanim 2>/dev/null || true
    defaults delete com.apple.dock show-recents 2>/dev/null || true

    log_info "Resetting Sound settings..."
    defaults delete NSGlobalDomain com.apple.sound.beep.feedback 2>/dev/null || true

    log_info "Resetting Finder settings..."
    defaults delete com.apple.finder AppleShowAllFiles 2>/dev/null || true
    defaults delete com.apple.finder ShowPathbar 2>/dev/null || true
    defaults delete com.apple.finder ShowStatusBar 2>/dev/null || true
    defaults delete com.apple.desktopservices DSDontWriteNetworkStores 2>/dev/null || true
    defaults delete com.apple.desktopservices DSDontWriteUSBStores 2>/dev/null || true
    defaults delete com.apple.finder FXDefaultSearchScope 2>/dev/null || true
    defaults delete com.apple.finder FXPreferredViewStyle 2>/dev/null || true
    defaults delete com.apple.finder QuitMenuItem 2>/dev/null || true

    log_info "Resetting Keyboard settings..."
    defaults delete NSGlobalDomain NSAutomaticQuoteSubstitutionEnabled 2>/dev/null || true
    defaults delete NSGlobalDomain NSAutomaticDashSubstitutionEnabled 2>/dev/null || true
    defaults delete NSGlobalDomain NSAutomaticSpellingCorrectionEnabled 2>/dev/null || true
    defaults delete NSGlobalDomain NSAutomaticCapitalizationEnabled 2>/dev/null || true
    defaults delete NSGlobalDomain NSAutomaticPeriodSubstitutionEnabled 2>/dev/null || true
    defaults delete NSGlobalDomain KeyRepeat 2>/dev/null || true
    defaults delete NSGlobalDomain InitialKeyRepeat 2>/dev/null || true
    defaults delete NSGlobalDomain AppleKeyboardUIMode 2>/dev/null || true

    log_info "Resetting Dialog settings..."
    defaults delete NSGlobalDomain NSNavPanelExpandedStateForSaveMode 2>/dev/null || true
    defaults delete NSGlobalDomain NSNavPanelExpandedStateForSaveMode2 2>/dev/null || true
    defaults delete NSGlobalDomain PMPrintingExpandedStateForPrint 2>/dev/null || true
    defaults delete NSGlobalDomain PMPrintingExpandedStateForPrint2 2>/dev/null || true

    log_info "Resetting Security settings..."
    defaults delete com.apple.screensaver askForPassword 2>/dev/null || true
    defaults delete com.apple.screensaver askForPasswordDelay 2>/dev/null || true

    # Apply changes
    log_info "Applying changes..."
    killall Dock 2>/dev/null || true
    killall Finder 2>/dev/null || true

    log_success "macOS defaults reset to system defaults"
    log_info "Some settings may require logout/restart to take full effect"
}

###############################################################################
# Linux Mint configuration
###############################################################################

linux_mint_show() {
    echo ""
    log_info "Linux Mint Configuration"
    echo ""

    # Strip type prefixes (uint32, int32) and surrounding quotes from raw values
    clean_value() {
        local val="$1"
        val="${val#uint32 }"
        val="${val#int32 }"
        val="${val#int64 }"
        val="${val#\'}"
        val="${val%\'}"
        echo "$val"
    }

    format_bool() {
        local val
        val=$(clean_value "$1")
        case "$val" in
            true) echo "yes" ;;
            false) echo "no" ;;
            *) echo "$val" ;;
        esac
    }

    format_seconds() {
        local val
        val=$(clean_value "$1")
        if [[ "$val" == "<system default>" || "$val" == "<unavailable>" ]]; then
            echo "$val"
            return
        fi
        if ! [[ "$val" =~ ^[0-9]+$ ]]; then
            echo "$val"
            return
        fi
        if (( val >= 3600 && val % 3600 == 0 )); then
            echo "$(( val / 3600 )) hr"
        elif (( val >= 60 && val % 60 == 0 )); then
            echo "$(( val / 60 )) min"
        elif (( val >= 60 )); then
            echo "$(( val / 60 )) min $(( val % 60 )) sec"
        else
            echo "${val} sec"
        fi
    }

    # show_dconf KEY LABEL [FORMAT]
    # FORMAT: raw (default), bool, seconds, ms, speed
    show_dconf() {
        local key="$1"
        local label="$2"
        local format="${3:-raw}"
        local value
        value=$(dconf read "$key" 2>/dev/null || echo "<system default>")
        [[ -z "$value" ]] && value="<system default>"
        case "$format" in
            bool) value=$(format_bool "$value") ;;
            seconds) value=$(format_seconds "$value") ;;
            ms)
                value=$(clean_value "$value")
                [[ "$value" != "<system default>" ]] && value="${value} ms"
                ;;
            speed)
                value=$(clean_value "$value")
                if [[ "$value" != "<system default>" ]]; then
                    value=$(awk "BEGIN { printf \"%.0f%%\", $value * 100 }")
                fi
                ;;
            *) value=$(clean_value "$value") ;;
        esac
        printf "  %-40s %s\n" "$label:" "$value"
    }

    show_gsetting() {
        local schema="$1"
        local key="$2"
        local label="$3"
        local format="${4:-raw}"
        local value
        value=$(gsettings get "$schema" "$key" 2>/dev/null || echo "<system default>")
        case "$format" in
            bool) value=$(format_bool "$value") ;;
            seconds) value=$(format_seconds "$value") ;;
            ms)
                value=$(clean_value "$value")
                [[ "$value" != "<system default>" ]] && value="${value} ms"
                ;;
            *) value=$(clean_value "$value") ;;
        esac
        printf "  %-40s %s\n" "$label:" "$value"
    }

    echo "Power:"
    local profile
    profile=$(powerprofilesctl get 2>/dev/null || echo "<unavailable>")
    printf "  %-40s %s\n" "Power profile:" "$profile"
    show_dconf "/org/cinnamon/settings-daemon/plugins/power/sleep-display-ac" "Display sleep on AC" seconds
    show_dconf "/org/cinnamon/settings-daemon/plugins/power/sleep-inactive-ac-timeout" "Inactive sleep on AC" seconds
    show_dconf "/org/cinnamon/settings-daemon/plugins/power/lock-on-suspend" "Lock on suspend" bool
    echo ""

    echo "Screensaver:"
    show_dconf "/org/cinnamon/desktop/session/idle-delay" "Idle delay" seconds
    show_dconf "/org/cinnamon/desktop/screensaver/lock-enabled" "Lock enabled" bool
    show_dconf "/org/cinnamon/desktop/screensaver/lock-delay" "Lock delay" seconds
    echo ""

    echo "Keyboard:"
    show_gsetting "org.cinnamon.desktop.peripherals.keyboard" "repeat" "Key repeat" bool
    show_gsetting "org.cinnamon.desktop.peripherals.keyboard" "delay" "Repeat delay" ms
    show_gsetting "org.cinnamon.desktop.peripherals.keyboard" "repeat-interval" "Repeat interval" ms
    show_gsetting "org.cinnamon.desktop.peripherals.keyboard" "numlock-state" "Numlock state" bool
    local xset_rate
    xset_rate=$(xset q 2>/dev/null | grep -A1 "auto repeat delay" | head -1 | sed 's/.*delay: *\([0-9]*\).*rate: *\([0-9]*\).*/\1ms delay, \2 cps/' || echo "<unavailable>")
    printf "  %-40s %s\n" "xset repeat rate:" "$xset_rate"
    echo ""

    echo "Mouse:"
    show_dconf "/org/cinnamon/desktop/peripherals/mouse/accel-profile" "Acceleration profile"
    show_dconf "/org/cinnamon/desktop/peripherals/mouse/speed" "Speed" speed
    show_dconf "/org/cinnamon/desktop/peripherals/mouse/natural-scroll" "Natural scroll" bool
    echo ""

    echo "Sound:"
    show_dconf "/org/cinnamon/desktop/sound/event-sounds" "Event sounds" bool
    echo ""

    echo "Nemo (File Manager):"
    show_dconf "/org/nemo/preferences/default-folder-viewer" "Default view"
    echo ""

    echo "Bluetooth & Audio:"
    local bt_state
    bt_state=$(systemctl is-enabled bluetooth.service 2>/dev/null || echo "not found")
    printf "  %-40s %s\n" "bluetooth.service:" "$bt_state"
    if [[ -f /etc/wireplumber/wireplumber.conf.d/51-ldac-hq.conf ]]; then
        printf "  %-40s %s\n" "WirePlumber LDAC config:" "installed"
    else
        printf "  %-40s %s\n" "WirePlumber LDAC config:" "not installed"
    fi
    local ldac_installed="no"
    dpkg -s libldacbt-enc2 &>/dev/null && ldac_installed="yes"
    printf "  %-40s %s\n" "LDAC libraries:" "$ldac_installed"
    local bt_spa="no"
    dpkg -s libspa-0.2-bluetooth &>/dev/null && bt_spa="yes"
    printf "  %-40s %s\n" "PipeWire Bluetooth plugin:" "$bt_spa"
    echo ""

    echo "Camera:"
    local v4l_installed="no"
    command -v v4l2-ctl &>/dev/null && v4l_installed="yes"
    printf "  %-40s %s\n" "v4l-utils:" "$v4l_installed"
    local libcamera_spa="no"
    dpkg -s libspa-0.2-libcamera &>/dev/null && libcamera_spa="yes"
    printf "  %-40s %s\n" "PipeWire libcamera plugin:" "$libcamera_spa"
    echo ""

    echo "xbindkeys:"
    local xbk_installed="no"
    command -v xbindkeys &>/dev/null && xbk_installed="yes"
    printf "  %-40s %s\n" "xbindkeys:" "$xbk_installed"
    local xdotool_installed="no"
    command -v xdotool &>/dev/null && xdotool_installed="yes"
    printf "  %-40s %s\n" "xdotool:" "$xdotool_installed"
    if [[ -L "$HOME/.xbindkeysrc" ]]; then
        printf "  %-40s %s\n" "Config symlink:" "yes"
    elif [[ -f "$HOME/.xbindkeysrc" ]]; then
        printf "  %-40s %s\n" "Config symlink:" "no (regular file)"
    else
        printf "  %-40s %s\n" "Config symlink:" "not present"
    fi
    if [[ -f "$HOME/.config/autostart/xbindkeys.desktop" ]]; then
        printf "  %-40s %s\n" "Autostart entry:" "yes"
    else
        printf "  %-40s %s\n" "Autostart entry:" "no"
    fi
    echo ""

    if lsmod | grep -q "^nvidia "; then
        echo "NVIDIA Suspend:"
        local cmdline
        cmdline=$(cat /proc/cmdline 2>/dev/null || echo "")
        if echo "$cmdline" | grep -q "NVreg_PreserveVideoMemoryAllocations=1"; then
            printf "  %-40s %s\n" "PreserveVideoMemoryAllocations:" "enabled"
        else
            printf "  %-40s %s\n" "PreserveVideoMemoryAllocations:" "not set"
        fi
        for svc in nvidia-suspend nvidia-resume nvidia-hibernate nvidia-persistenced; do
            local state
            state=$(systemctl is-enabled "${svc}.service" 2>/dev/null || echo "not found")
            printf "  %-40s %s\n" "${svc}.service:" "$state"
        done
        echo ""
    fi

    echo "Storage:"
    local fstrim_state
    fstrim_state=$(systemctl is-enabled "fstrim.timer" 2>/dev/null || echo "not found")
    printf "  %-40s %s\n" "fstrim.timer:" "$fstrim_state"
    echo ""

    if [[ -f /etc/default/grub ]]; then
        echo "GRUB:"
        local grub_val
        grub_val=$(grep "^GRUB_TIMEOUT_STYLE=" /etc/default/grub 2>/dev/null | cut -d= -f2 || echo "<not set>")
        [[ -z "$grub_val" ]] && grub_val="<not set>"
        printf "  %-40s %s\n" "Timeout style:" "$grub_val"

        grub_val=$(grep "^GRUB_TIMEOUT=" /etc/default/grub 2>/dev/null | cut -d= -f2 || echo "<not set>")
        [[ -z "$grub_val" ]] && grub_val="<not set>"
        printf "  %-40s %s\n" "Timeout:" "$grub_val"

        grub_val=$(grep "^GRUB_DISABLE_OS_PROBER=" /etc/default/grub 2>/dev/null | cut -d= -f2 || echo "<not set>")
        [[ -z "$grub_val" ]] && grub_val="<not set>"
        printf "  %-40s %s\n" "OS prober disabled:" "$grub_val"

        grub_val=$(grep "^GRUB_CMDLINE_LINUX_DEFAULT=" /etc/default/grub 2>/dev/null | sed 's/^GRUB_CMDLINE_LINUX_DEFAULT=//' || echo "<not set>")
        [[ -z "$grub_val" ]] && grub_val="<not set>"
        printf "  %-40s %s\n" "Kernel params:" "$grub_val"
        echo ""
    fi
}

linux_mint_apply() {
    log_step "Configuring Linux Mint System Defaults"

    if [[ "${DRY_RUN:-false}" == "true" ]]; then
        log_info "[DRY-RUN] Would install Bluetooth/audio/camera packages"
        log_info "[DRY-RUN] Would configure Power settings (performance profile, sleep timers)"
        log_info "[DRY-RUN] Would configure Screensaver settings (idle delay, lock)"
        log_info "[DRY-RUN] Would configure Keyboard settings (repeat rate, numlock)"
        log_info "[DRY-RUN] Would configure Mouse settings (acceleration, speed, natural scroll)"
        log_info "[DRY-RUN] Would configure Sound settings (disable event sounds)"
        log_info "[DRY-RUN] Would configure Nemo settings (list view)"
        log_info "[DRY-RUN] Would configure GRUB (timeout=10, menu, os-prober enabled)"
        if lsmod | grep -q "^nvidia "; then
            log_info "[DRY-RUN] Would configure NVIDIA suspend (GRUB parameters, systemd services)"
        fi
        log_info "[DRY-RUN] Would install and configure xbindkeys (mouse button bindings)"
        if lspci -d 14c3:0717 2>/dev/null | grep -q . || lsmod | grep -q "^mt7925e "; then
            log_info "[DRY-RUN] Would install WiFi suspend fix for MT7925E (systemd sleep hook)"
        fi
        log_info "[DRY-RUN] Would enable fstrim.timer for SSD TRIM"
        log_info "[DRY-RUN] Would enable bluetooth.service"
        log_info "[DRY-RUN] Would deploy WirePlumber LDAC Bluetooth config"
        log_success "Linux Mint defaults would be configured"
        return 0
    fi

    # Bluetooth, audio, and camera packages
    log_info "Installing Bluetooth/audio/camera packages..."
    local bt_audio_packages=(
        libspa-0.2-bluetooth  # PipeWire Bluetooth SPA plugin (LDAC, SBC, AAC codecs)
        libldacbt-abr2         # LDAC adaptive bitrate library
        libldacbt-enc2         # LDAC encoder library
        libspa-0.2-libcamera   # libcamera SPA plugin for PipeWire
        v4l-utils              # Video4Linux utilities (camera diagnostics)
        linux-firmware         # WiFi/Bluetooth firmware (MediaTek MT7925, etc.)
    )
    # Single apt update, then install all packages at once
    maybe_sudo apt-get update -qq
    maybe_sudo apt-get install -y -qq "${bt_audio_packages[@]}"
    log_success "Bluetooth/audio/camera packages installed"

    # Power Settings
    log_info "Configuring Power..."
    powerprofilesctl set performance
    dconf write /org/cinnamon/settings-daemon/plugins/power/sleep-display-ac 3600
    dconf write /org/cinnamon/settings-daemon/plugins/power/sleep-inactive-ac-timeout 2700
    dconf write /org/cinnamon/settings-daemon/plugins/power/lock-on-suspend true

    # Screensaver Settings
    log_info "Configuring Screensaver..."
    dconf write /org/cinnamon/desktop/session/idle-delay "uint32 1800"
    dconf write /org/cinnamon/desktop/screensaver/lock-enabled false
    dconf write /org/cinnamon/desktop/screensaver/lock-delay "uint32 2"

    # Keyboard Settings
    log_info "Configuring Keyboard..."
    gsettings set org.cinnamon.desktop.peripherals.keyboard repeat true
    gsettings set org.cinnamon.desktop.peripherals.keyboard delay 200
    gsettings set org.cinnamon.desktop.peripherals.keyboard repeat-interval 20
    gsettings set org.cinnamon.desktop.peripherals.keyboard numlock-state true
    xset r rate 200 50

    # Mouse Settings
    log_info "Configuring Mouse..."
    dconf write /org/cinnamon/desktop/peripherals/mouse/accel-profile "'flat'"
    dconf write /org/cinnamon/desktop/peripherals/mouse/speed 0.65126050420168058
    dconf write /org/cinnamon/desktop/peripherals/mouse/natural-scroll true

    # Sound Settings
    log_info "Configuring Sound..."
    dconf write /org/cinnamon/desktop/sound/event-sounds false

    # Nemo Settings
    log_info "Configuring Nemo..."
    dconf write /org/nemo/preferences/default-folder-viewer "'list-view'"

    # GRUB Configuration
    log_info "Configuring GRUB..."
    set_grub_var "GRUB_TIMEOUT_STYLE" "menu"
    set_grub_var "GRUB_TIMEOUT" "10"
    set_grub_var "GRUB_DISABLE_OS_PROBER" "false"

    # NVIDIA kernel parameters (only if NVIDIA driver is loaded)
    if lsmod | grep -q "^nvidia "; then
        log_info "Configuring NVIDIA kernel parameters..."
        add_grub_cmdline_param "nvidia.NVreg_PreserveVideoMemoryAllocations=1"
        add_grub_cmdline_param "nvidia.NVreg_EnableS0ixPowerManagement=0"
        add_grub_cmdline_param "pcie_aspm=off"
    fi

    maybe_sudo update-grub
    log_info "GRUB configured with 10s timeout menu and OS prober enabled."

    # NVIDIA suspend services (only if NVIDIA driver is loaded)
    if lsmod | grep -q "^nvidia "; then
        log_info "Configuring NVIDIA suspend services..."
        for svc in nvidia-suspend nvidia-resume nvidia-hibernate nvidia-persistenced; do
            if systemctl list-unit-files "${svc}.service" &>/dev/null; then
                maybe_sudo systemctl enable "${svc}.service" 2>/dev/null || true
            fi
        done
        log_success "NVIDIA suspend stability configured"
    fi

    # WiFi suspend fix for MediaTek MT7925E
    # The mt7925e PCI power management resume path is buggy (returns ETIMEDOUT / -110),
    # so we unload the module before suspend and reload after resume for a clean firmware reload.
    if lspci -d 14c3:0717 2>/dev/null | grep -q . || lsmod | grep -q "^mt7925e "; then
        log_info "Installing WiFi suspend fix for MT7925E..."
        local sleep_hook="/usr/lib/systemd/system-sleep/wifi-mt7925"
        maybe_sudo tee "$sleep_hook" > /dev/null << 'HOOK'
#!/bin/bash
case "$1" in
    pre)
        modprobe -r mt7925e 2>/dev/null || true
        ;;
    post)
        modprobe mt7925e 2>/dev/null || true
        ;;
esac
HOOK
        maybe_sudo chmod 755 "$sleep_hook"
        log_success "WiFi suspend fix installed at $sleep_hook"
    fi

    # Enable SSD TRIM timer
    log_info "Configuring SSD TRIM..."
    if systemctl list-unit-files "fstrim.timer" &>/dev/null; then
        maybe_sudo systemctl enable fstrim.timer 2>/dev/null || true
        maybe_sudo systemctl start fstrim.timer 2>/dev/null || true
        log_success "fstrim.timer enabled"
    fi

    # Bluetooth service
    log_info "Configuring Bluetooth..."
    maybe_sudo systemctl enable bluetooth 2>/dev/null || true
    maybe_sudo systemctl start bluetooth 2>/dev/null || true
    log_success "bluetooth.service enabled"

    # WirePlumber LDAC Bluetooth audio config
    log_info "Configuring WirePlumber Bluetooth audio..."
    local wp_conf_dir="/etc/wireplumber/wireplumber.conf.d"
    local wp_conf_file="$wp_conf_dir/51-ldac-hq.conf"
    local wp_template="$DOTFILES_ROOT/config/linux/wireplumber/51-ldac-hq.conf"
    maybe_sudo mkdir -p "$wp_conf_dir"
    if [[ -f "$wp_conf_file" ]] && diff -q "$wp_template" "$wp_conf_file" > /dev/null 2>&1; then
        log_info "WirePlumber LDAC config already up to date"
    else
        maybe_sudo cp "$wp_template" "$wp_conf_file"
        # Restart PipeWire stack to pick up new config
        systemctl --user restart pipewire pipewire-pulse wireplumber 2>/dev/null || true
        log_success "WirePlumber LDAC Bluetooth config deployed"
    fi

    # xbindkeys (mouse button bindings)
    log_info "Configuring xbindkeys..."
    maybe_sudo apt-get install -y -qq xbindkeys xdotool
    local xbk_config="$DOTFILES_ROOT/config/linux/xbindkeys/.xbindkeysrc"
    safe_symlink "$xbk_config" "$HOME/.xbindkeysrc"
    mkdir -p "$HOME/.config/autostart"
    cp "$DOTFILES_ROOT/config/linux/xbindkeys/xbindkeys.desktop" "$HOME/.config/autostart/xbindkeys.desktop"
    # Restart xbindkeys to pick up config
    killall xbindkeys 2>/dev/null || true
    xbindkeys 2>/dev/null || true
    log_success "xbindkeys configured"

    # Verbose verification output
    if [[ "${VERBOSE:-false}" == "true" ]]; then
        echo ""
        log_step "Verification"
        log_info "Bluetooth adapter:"
        bluetoothctl show 2>/dev/null | head -5 || log_warning "No Bluetooth adapter found"
        echo ""
        log_info "LDAC libraries:"
        dpkg -l 2>/dev/null | grep ldac || log_warning "LDAC libraries not found"
        echo ""
        log_info "WirePlumber config:"
        cat "$wp_conf_file" 2>/dev/null || log_warning "WirePlumber config not found"
        echo ""
        log_info "Camera devices:"
        v4l2-ctl --list-devices 2>/dev/null || log_info "No camera detected"
        echo ""
        log_info "PipeWire Bluetooth nodes:"
        pw-cli ls Node 2>/dev/null | grep -i bluez || log_info "No Bluetooth audio devices connected"
        echo ""
    fi

    log_success "Linux Mint defaults configured"
    log_info "Some settings may require logout/restart to take full effect"

    # Hint about manual steps that can't be automated
    if lsmod | grep -q "^amdgpu "; then
        log_warning "amdgpu module is loaded — if you have a dual-GPU system (Ryzen iGPU + NVIDIA),"
        log_warning "consider disabling the iGPU in BIOS to prevent suspend/freeze issues."
        log_warning "See: TROUBLESHOOTING.md → Linux → Desktop Freezes"
    fi
}

linux_mint_reset() {
    log_step "Resetting Linux Mint System Defaults"

    if [[ "${DRY_RUN:-false}" == "true" ]]; then
        log_info "[DRY-RUN] Would reset Power, Screensaver, Keyboard, Mouse, Sound, Nemo settings"
        log_info "[DRY-RUN] Would reset GRUB settings (timeout=10, menu, os-prober enabled)"
        if lsmod | grep -q "^nvidia "; then
            log_info "[DRY-RUN] Would reset NVIDIA suspend settings (GRUB parameters, systemd services)"
        fi
        log_info "[DRY-RUN] Would disable fstrim.timer"
        log_info "[DRY-RUN] Would reset xbindkeys (stop, remove autostart and config symlink)"
        log_info "[DRY-RUN] Would remove WirePlumber LDAC config"
        log_success "Linux Mint defaults would be reset"
        return 0
    fi

    log_info "Resetting Power settings..."
    powerprofilesctl set balanced
    dconf reset /org/cinnamon/settings-daemon/plugins/power/sleep-display-ac
    dconf reset /org/cinnamon/settings-daemon/plugins/power/sleep-inactive-ac-timeout
    dconf reset /org/cinnamon/settings-daemon/plugins/power/lock-on-suspend

    log_info "Resetting Screensaver settings..."
    dconf reset /org/cinnamon/desktop/session/idle-delay
    dconf reset /org/cinnamon/desktop/screensaver/lock-enabled
    dconf reset /org/cinnamon/desktop/screensaver/lock-delay

    log_info "Resetting Keyboard settings..."
    gsettings reset org.cinnamon.desktop.peripherals.keyboard repeat
    gsettings reset org.cinnamon.desktop.peripherals.keyboard delay
    gsettings reset org.cinnamon.desktop.peripherals.keyboard repeat-interval
    gsettings reset org.cinnamon.desktop.peripherals.keyboard numlock-state
    xset r rate

    log_info "Resetting Mouse settings..."
    dconf reset /org/cinnamon/desktop/peripherals/mouse/accel-profile
    dconf reset /org/cinnamon/desktop/peripherals/mouse/speed
    dconf reset /org/cinnamon/desktop/peripherals/mouse/natural-scroll

    log_info "Resetting Sound settings..."
    dconf reset /org/cinnamon/desktop/sound/event-sounds

    log_info "Resetting Nemo settings..."
    dconf reset /org/nemo/preferences/default-folder-viewer

    log_info "Removing WirePlumber LDAC config..."
    if [[ -f /etc/wireplumber/wireplumber.conf.d/51-ldac-hq.conf ]]; then
        maybe_sudo rm /etc/wireplumber/wireplumber.conf.d/51-ldac-hq.conf
        systemctl --user restart pipewire pipewire-pulse wireplumber 2>/dev/null || true
    fi

    log_info "Resetting xbindkeys..."
    killall xbindkeys 2>/dev/null || true
    rm -f "$HOME/.config/autostart/xbindkeys.desktop"
    if [[ -L "$HOME/.xbindkeysrc" ]]; then
        rm -f "$HOME/.xbindkeysrc"
    fi

    log_info "Resetting GRUB settings..."
    set_grub_var "GRUB_TIMEOUT_STYLE" "menu"
    set_grub_var "GRUB_TIMEOUT" "10"
    set_grub_var "GRUB_DISABLE_OS_PROBER" "false"

    local mint_grub_cfg="/etc/default/grub.d/50_linuxmint.cfg"
    if [[ -f "$mint_grub_cfg" ]] && grep -q "GRUB_DISABLE_OS_PROBER=true" "$mint_grub_cfg"; then
        maybe_sudo sed -i "s/GRUB_DISABLE_OS_PROBER=true/GRUB_DISABLE_OS_PROBER=false/" "$mint_grub_cfg"
    fi

    if lsmod | grep -q "^nvidia "; then
        log_info "Resetting NVIDIA suspend settings..."
        remove_grub_cmdline_param "nvidia.NVreg_PreserveVideoMemoryAllocations=1"
        remove_grub_cmdline_param "nvidia.NVreg_EnableS0ixPowerManagement=0"
        remove_grub_cmdline_param "pcie_aspm=off"
        for svc in nvidia-suspend nvidia-resume nvidia-hibernate nvidia-persistenced; do
            maybe_sudo systemctl disable "${svc}.service" 2>/dev/null || true
        done
    fi

    # Remove WiFi suspend fix
    local sleep_hook="/usr/lib/systemd/system-sleep/wifi-mt7925"
    if [[ -f "$sleep_hook" ]]; then
        log_info "Removing WiFi suspend fix..."
        maybe_sudo rm -f "$sleep_hook"
        log_success "WiFi suspend fix removed"
    fi

    log_info "Resetting fstrim..."
    maybe_sudo systemctl stop fstrim.timer 2>/dev/null || true
    maybe_sudo systemctl disable fstrim.timer 2>/dev/null || true

    maybe_sudo update-grub

    log_success "Linux Mint defaults reset to system defaults"
    log_info "Some settings may require logout/restart to take full effect"
}

###############################################################################
# Main command
###############################################################################

cmd_setup() {
    local skip_gpu=false
    local skip_configure=false
    local show_mode=false
    local reset_mode=false

    for arg in "$@"; do
        case "$arg" in
            -h|--help)
                show_setup_help
                return 0
                ;;
            --show)           show_mode=true ;;
            --reset)          reset_mode=true ;;
            --skip-gpu)       skip_gpu=true ;;
            --skip-configure) skip_configure=true ;;
        esac
    done

    # --show: display current config and exit
    if [[ "$show_mode" == "true" ]]; then
        case "$OS" in
            linuxmint) linux_mint_show ;;
            macos)     macos_show ;;
            *)         log_warning "No system configuration available for $OS" ;;
        esac
        return 0
    fi

    # --reset: reset config to defaults and exit
    if [[ "$reset_mode" == "true" ]]; then
        case "$OS" in
            linuxmint) linux_mint_reset ;;
            macos)     macos_reset ;;
            *)         log_warning "No system configuration available for $OS" ;;
        esac
        return 0
    fi

    local step=1

    # =========================================================================
    # Step 1: GPU setup (Linux only)
    # =========================================================================
    if [[ "$OS" != "macos" ]]; then
        if [[ "$skip_gpu" == "true" ]]; then
            log_info "Step $step: GPU setup (skipped)"
        else
            log_step "Step $step: GPU Driver Signing"
            # shellcheck source=lib/gpu.sh
            source "$DOTFILES_ROOT/lib/gpu.sh"
            # shellcheck source=cmds/gpu.sh
            source "$DOTFILES_ROOT/cmds/gpu.sh"

            if command -v lspci >/dev/null 2>&1 && lspci | grep -i nvidia >/dev/null 2>&1; then
                if is_secure_boot_enabled; then
                    gpu_setup
                else
                    log_info "Secure Boot is disabled, skipping GPU driver signing"
                fi
            else
                log_info "No NVIDIA hardware detected, skipping GPU driver signing"
            fi
        fi
        step=$((step + 1))
        echo
    fi

    # =========================================================================
    # Step 2: System configuration
    # =========================================================================
    if [[ "$skip_configure" == "true" ]]; then
        log_info "Step $step: System configuration (skipped)"
    else
        log_step "Step $step: System Configuration"

        case "$OS" in
            linuxmint)
                linux_mint_apply
                ;;
            macos)
                macos_apply
                ;;
            *)
                log_warning "No system configuration available for $OS"
                ;;
        esac
    fi
    step=$((step + 1))
    echo

    # =========================================================================
    # Done
    # =========================================================================
    log_success "Setup complete!"
    log_info "Some changes may require a logout or reboot to take full effect."
}
