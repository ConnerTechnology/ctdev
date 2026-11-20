#!/usr/bin/env bash
# The release's prompt audit (AGENTS.md, "Releases"), run headless with the bundled skills back on
# (docs/agents/where-things-go.md). Its findings depend only on the files that steer sessions, so
# when none changed since the last release tag it says so and stops: on 2026-11-19 three releases
# ran it three times, 7.3M tokens. A headless run can't answer a permission prompt, so it gets the
# read-only tools it needs and is kept to this repo; each refused call was spent tokens.
#
# Usage: scripts/prompt-audit.sh [--force]   (--force runs it even when nothing changed)
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

steering=(CLAUDE.md AGENTS.md .claude)
last=$(git describe --tags --abbrev=0 --match 'v*' 2>/dev/null || true)
if [ "${1:-}" != --force ] && [ -n "$last" ] && git diff --quiet "$last" -- "${steering[@]}"; then
  echo "prompt audit: nothing that steers sessions changed since $last, so it would find the same; skipped"
  exit 0
fi

exec claude -p --settings '{"disableBundledSkills":false}' \
  --allowedTools Read Grep Glob Agent \
  --append-system-prompt "Audit only files inside this repository: ~/.claude is out of scope here (ADR-0007). Read files with Read, Grep and Glob, not Bash." \
  "/doctor prompt-audit"
