#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

# Install ctdev binary, then set up shell environment
bash install.sh
export PATH="$HOME/.local/bin:$PATH"
ctdev --force install zsh
