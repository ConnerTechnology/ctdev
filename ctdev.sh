#!/usr/bin/env bash
set -euo pipefail

# ctdev - Conner Technology Dev CLI
# A unified CLI for managing dotfiles

# Get the directory where this script is located (resolving symlinks)
SOURCE="${BASH_SOURCE[0]}"
while [[ -L "$SOURCE" ]]; do
    DIR="$(cd -P "$(dirname "$SOURCE")" && pwd)"
    SOURCE="$(readlink "$SOURCE")"
    # If SOURCE is relative, resolve it relative to the symlink's directory
    [[ "$SOURCE" != /* ]] && SOURCE="$DIR/$SOURCE"
done
SCRIPT_DIR="$(cd -P "$(dirname "$SOURCE")" && pwd)"
export DOTFILES_ROOT="$SCRIPT_DIR"

# Source libraries
source "${SCRIPT_DIR}/lib/utils.sh"
source "${SCRIPT_DIR}/lib/cli.sh"
source "${SCRIPT_DIR}/lib/components.sh"

# Initialize flags
SHOW_HELP=false
SHOW_VERSION=false
VERBOSE=${VERBOSE:-false}
DRY_RUN=${DRY_RUN:-false}
FORCE=${FORCE:-false}

# Parse global flags
eval "$(parse_global_flags "$@")"
if [[ ${#REMAINING_ARGS[@]} -gt 0 ]]; then
    set -- "${REMAINING_ARGS[@]}"
else
    set --
fi

# Handle --version
if [[ "$SHOW_VERSION" == "true" ]]; then
    show_version
    exit 0
fi

# Get the command (first positional argument)
COMMAND="${1:-}"

# Handle --help with no command
if [[ "$SHOW_HELP" == "true" ]] && [[ -z "$COMMAND" ]]; then
    show_main_help
    exit 0
fi

# Handle no command
if [[ -z "$COMMAND" ]]; then
    show_main_help
    exit 0
fi

# Shift past the command
shift

# Re-parse for any flags after the command
eval "$(parse_global_flags "$@")"
if [[ ${#REMAINING_ARGS[@]} -gt 0 ]]; then
    set -- "${REMAINING_ARGS[@]}"
else
    set --
fi

# Handle --help for specific commands
if [[ "$SHOW_HELP" == "true" ]]; then
    case "$COMMAND" in
        install)   show_install_help ;;
        uninstall) show_uninstall_help ;;
        update)    show_update_help ;;
        info)      show_info_help ;;
        configure) show_configure_help ;;
        gpu)       show_gpu_help ;;
        setup)     show_setup_help ;;
        cleanup)   show_cleanup_help ;;
        *)         show_main_help ;;
    esac
    exit 0
fi

# Validate command
if ! require_command "$COMMAND"; then
    log_error "Unknown command: $COMMAND"
    echo ""
    show_main_help
    exit 1
fi

# Check if dotfiles repo has updates available
# This function is wrapped to ensure it never causes script failure
check_for_updates() {
    # Run update check in a subshell to prevent any git errors from propagating
    (
        set +e  # Disable errexit within this subshell

        if [[ ! -d "$DOTFILES_ROOT/.git" ]]; then
            exit 0
        fi

        # Fetch latest without changing anything
        git -C "$DOTFILES_ROOT" fetch --quiet 2>/dev/null || exit 0

        # Check if we're behind origin (may fail if no upstream configured)
        local local_ref remote_ref
        local_ref=$(git -C "$DOTFILES_ROOT" rev-parse HEAD 2>/dev/null) || exit 0
        remote_ref=$(git -C "$DOTFILES_ROOT" rev-parse '@{upstream}' 2>/dev/null) || exit 0

        if [[ "$local_ref" != "$remote_ref" ]]; then
            local behind
            behind=$(git -C "$DOTFILES_ROOT" rev-list --count 'HEAD..@{upstream}' 2>/dev/null) || exit 0

            if [[ "$behind" -gt 0 ]]; then
                log_warning "ctdev is $behind commit(s) behind origin"

                # Only prompt if we're in an interactive terminal
                if [[ -t 0 ]]; then
                    printf "Would you like to pull the latest changes before proceeding? [y/N] "
                    # Use timeout to prevent hanging in automated environments
                    if read -r -t 30 answer && [[ "$answer" =~ ^[Yy]$ ]]; then
                        log_info "Pulling latest changes..."
                        if git -C "$DOTFILES_ROOT" pull --ff-only; then
                            log_success "ctdev updated"
                            echo
                        else
                            log_error "Failed to pull (you may have local changes)"
                            log_info "Continuing with current version..."
                            echo
                        fi
                    else
                        log_info "Continuing with current version..."
                        echo
                    fi
                else
                    # Non-interactive: just warn and continue
                    log_info "Run 'git -C $DOTFILES_ROOT pull' to update"
                fi
            fi
        fi
    ) || true  # Ensure the subshell failure never propagates
}

check_for_updates

# Enable verbose mode if requested
if [[ "$VERBOSE" == "true" ]]; then
    set -x
fi

# Dispatch to command handler
CMD_SCRIPT="${SCRIPT_DIR}/cmds/${COMMAND}.sh"

if [[ ! -f "$CMD_SCRIPT" ]]; then
    log_error "Command script not found: $CMD_SCRIPT"
    exit 1
fi

# Source and run the command
# shellcheck source=/dev/null
source "$CMD_SCRIPT"
"cmd_${COMMAND}" "$@"
