#!/usr/bin/env bash
# The unit test for the guidance itself.
#
# AGENTS.md is a claim about this repo, and nothing compiles it. The way it goes wrong on its
# own is naming a path that has since moved. That is mechanical, so it is a check rather than
# something a reader is expected to notice.
#
# Checks:
#   1. every backticked repo path in AGENTS.md exists or is gitignored
#   2. no tracked file names a real tailnet (a tail<hex>.ts.net MagicDNS name)
#
# Usage: scripts/check-guidance.sh   (exit 1 on any failure, all failures listed)

set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 1

fail=0
say() { printf '%s\n' "$*"; }
bad() { fail=1; say "FAIL  $*"; }

# 1. Paths ------------------------------------------------------------------------------
# What counts as a repo path is deliberately narrow, because AGENTS.md also names host paths,
# Go identifiers, command lines and URLs that are not files in this tree. A token qualifies
# only when it is spelled the way a path in this repo is spelled: allowed characters only, no
# leading / or ./, no placeholder ellipsis, and either rooted at one of our top-level
# directories, a dot-directory, or a bare Markdown filename.
# SC2016: the backticks in the grep pattern are the literal character we are matching on,
# so the single quotes are the point.
# shellcheck disable=SC2016
while read -r p; do
  [ -z "$p" ] && continue
  bare=${p%/}
  [ -e "$bare" ] && continue
  git check-ignore -q "$bare" && continue
  bad "AGENTS.md names $p, which does not exist"
done < <(grep -oE '`[^`]+`' AGENTS.md | tr -d '`' \
  | grep -vE '[^A-Za-z0-9._/-]' \
  | grep -vE '^/|^\./|\.\.\.' \
  | grep -E '^(ctdev|scripts)/|^\.[^/]*/|^[^/]+\.md$' \
  | sort -u)

# 2. Tailnet names ----------------------------------------------------------------------
# The repo is public and carries no real family or client data (AGENTS.md, "Guardrails"). A
# real tailnet's MagicDNS suffix is tail<hex>.ts.net, so the pattern catches any tailnet
# without this script naming ours. Examples use tailnet-example.ts.net, which it does not
# match. A hit is reported by file and line only, so a real name never reaches the public CI log.
while IFS= read -r hit; do
  bad "$hit names a real tailnet (tail<hex>.ts.net); use tailnet-example.ts.net instead"
done < <(git grep -nIE 'tail[0-9a-f]+\.ts\.net' | cut -d: -f1,2)

if [ "$fail" -eq 0 ]; then say "guidance check: ok"; fi
exit "$fail"
