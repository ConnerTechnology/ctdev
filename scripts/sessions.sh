#!/usr/bin/env bash
# Past agent sessions, from the checkpoints Entire records in this repo (refs/entire/checkpoints/).
# Reads them on this machine; nothing is sent anywhere. Checkpoints are never pushed (this repo is
# public), so only the sessions recorded in this clone are here (docs/agents/goal-loop.md).
#
#   scripts/sessions.sh digest [--since YYYY-MM-DD] [--until YYYY-MM-DD]   signals for a retro (default: yesterday)
#   scripts/sessions.sh search [--since YYYY-MM-DD] <term>...               where past sessions mention every term
#
# Built from scripts/sessions (Go) into ~/.cache/ctdev/tools/ on each run; the build is cached.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
bin="${XDG_CACHE_HOME:-$HOME/.cache}/ctdev/tools/sessions"
mkdir -p "$(dirname "$bin")"
(cd "$root/scripts/sessions" && go build -o "$bin" .)
cd "$root"
exec "$bin" "$@"
