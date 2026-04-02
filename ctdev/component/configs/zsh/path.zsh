# Path Configuration
# All PATH setup consolidated here for clear ordering

# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ System Paths ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

path=(
  /usr/local/bin
  /usr/local/sbin
  $path
)

# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ Go ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

[[ -n "$GOPATH" && -d "$GOPATH/bin" ]] && path+=($GOPATH/bin)
[[ -n "$GOROOT" && -d "$GOROOT/bin" ]] && path+=($GOROOT/bin)

# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ Homebrew (macOS) ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

if [[ -d "/opt/homebrew" ]]; then
  # Apple Silicon
  eval "$(/opt/homebrew/bin/brew shellenv 2>/dev/null)" || true
elif [[ -d "/usr/local/Homebrew" ]]; then
  # Intel Mac
  eval "$(/usr/local/bin/brew shellenv 2>/dev/null)" || true
fi

# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ nodenv ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

if [[ -d "$HOME/.nodenv" ]]; then
  path=($HOME/.nodenv/bin $path)
  eval "$(nodenv init -)"
fi

# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ rbenv ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

if [[ -d "$HOME/.rbenv" ]]; then
  path=($HOME/.rbenv/bin $path)
  eval "$(rbenv init - zsh)"
fi

# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ User Paths ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

# Prepend last so user binaries take precedence over everything (including shims)
path=(
  $HOME/.local/bin
  $HOME/bin
  $path
)

# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ Cleanup ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

# Remove duplicates and non-existent directories
typeset -U path
path=($^path(N-/))

export PATH
