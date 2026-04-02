# Path to your Oh My Zsh installation.
export ZSH="$HOME/.oh-my-zsh"

# Set name of the theme to load --- if set to "random", it will
# load a random theme each time Oh My Zsh is loaded, in which case,
# to know which specific one was loaded, run: echo $RANDOM_THEME
# See https://github.com/ohmyzsh/ohmyzsh/wiki/Themes
ZSH_THEME=""

# Uncomment the following line to use case-sensitive completion.
# CASE_SENSITIVE="true"

# Uncomment the following line to disable auto-setting terminal title.
# DISABLE_AUTO_TITLE="true"

# ZSH_DISABLE_COMPFIX=true

# Which plugins would you like to load?
# Standard plugins can be found in $ZSH/plugins/
# Custom plugins may be added to $ZSH_CUSTOM/plugins/
# Example format: plugins=(rails git textmate ruby lighthouse)
# Add wisely, as too many plugins slow down shell startup.

# Base plugins (always available)
plugins=(
  common-aliases
  zsh-autosuggestions
  zsh-completions
)

# Fix autosuggestion color (use a visible gray instead of default black)
ZSH_AUTOSUGGEST_HIGHLIGHT_STYLE='fg=244'

# Conditionally load plugins that require installed tools
command -v git &>/dev/null && plugins+=(git)
command -v docker &>/dev/null && plugins+=(docker)
command -v kubectl &>/dev/null && plugins+=(kubectl)
command -v go &>/dev/null && plugins+=(golang)
command -v ruby &>/dev/null && plugins+=(ruby)
command -v rake &>/dev/null && plugins+=(rake)
command -v bundle &>/dev/null && plugins+=(bundler)
[[ -d "$HOME/.rbenv" ]] && plugins+=(rbenv)

source $ZSH/oh-my-zsh.sh

# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ Pure Prompt ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

PURE_GIT_PULL=0

# Add Pure prompt paths to fpath
# Both the pure directory and functions directory for compatibility
fpath+=("$HOME/.zsh/pure" "$HOME/.zsh/functions")

autoload -U promptinit; promptinit
prompt pure

# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ History ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

HISTFILE=~/.zsh_history
HISTSIZE=100000
SAVEHIST=100000

setopt HIST_IGNORE_SPACE  # Don't save when prefixed with space
setopt HIST_IGNORE_DUPS   # Don't save duplicate lines
setopt SHARE_HISTORY      # Share history between sessions

# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ Completion ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

# Dedupe fpath entries
typeset -U fpath

# oh-my-zsh zsh-completions (ensure this is BEFORE compinit)
# Adjust if you keep plugins elsewhere
ZSH_CUSTOM="${ZSH_CUSTOM:-$HOME/.oh-my-zsh/custom}"
if [ -d "$ZSH_CUSTOM/plugins/zsh-completions/src" ]; then
  fpath=("$ZSH_CUSTOM/plugins/zsh-completions/src" $fpath)
fi

# User completions
[ -d "$HOME/.zfunc" ] && fpath+=("$HOME/.zfunc")

# nodenv/rbenv completions (use NODENV_ROOT/RBENV_ROOT if present)
if command -v nodenv >/dev/null 2>&1; then
  : "${NODENV_ROOT:=${NODENV_ROOT:-$HOME/.nodenv}}"
  [ -d "$NODENV_ROOT/completions" ] && fpath+=("$NODENV_ROOT/completions")
elif [ -d "$HOME/.nodenv" ]; then
  fpath+=("$HOME/.nodenv/completions")
fi

if command -v rbenv >/dev/null 2>&1; then
  : "${RBENV_ROOT:=${RBENV_ROOT:-$HOME/.rbenv}}"
  [ -d "$RBENV_ROOT/completions" ] && fpath+=("$RBENV_ROOT/completions")
elif [ -d "$HOME/.rbenv" ]; then
  fpath+=("$HOME/.rbenv/completions")
fi

autoload -Uz compinit
zmodload zsh/complist 2>/dev/null || true

# Use cached .zcompdump; create if missing, skip expensive checks on first run
ZCDUMP="${ZDOTDIR:-$HOME}/.zcompdump"
if [[ -f "$ZCDUMP" ]]; then
  compinit -d "$ZCDUMP"
else
  compinit -C -d "$ZCDUMP"
fi
# Interactive menu when multiple matches
zstyle ':completion:*' menu select

# Show descriptions and group results nicely
zstyle ':completion:*' descriptions ' (%d)'
zstyle ':completion:*' group-name ''

# Case-insensitive + smart separators matching
zstyle ':completion:*' matcher-list \
  'm:{a-z}={A-Za-z}' \
  'r:|[._-]=* r:|=*'

# Don’t complete uninteresting files
zstyle ':completion:*:complete:(^rm):*:*files' ignored-patterns '*?.o' '*~'

# Use cache for some completers (speeds up)
zstyle ':completion:*' use-cache on
zstyle ':completion:*' cache-path "${XDG_CACHE_HOME:-$HOME/.cache}/zsh"

# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ SSH ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

if [ -z "$SSH_AUTH_SOCK" ]; then
    eval "$(ssh-agent -s)" > /dev/null
fi

# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ Secrets ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

# Load secrets from ~/.secrets (not tracked in git)
[[ -f ~/.secrets ]] && source ~/.secrets

# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ PATH ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

# All PATH setup consolidated in path.zsh (system, homebrew, nodenv, rbenv, user paths)
[[ -f ~/.zsh/path.zsh ]] && source ~/.zsh/path.zsh

# bun completions
[ -s "/home/thomas/.bun/_bun" ] && source "/home/thomas/.bun/_bun"

# bun
export BUN_INSTALL="$HOME/.bun"
export PATH="$BUN_INSTALL/bin:$PATH"
