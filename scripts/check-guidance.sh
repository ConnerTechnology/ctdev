#!/usr/bin/env bash
# The unit test for the guidance itself.
#
# CLAUDE.md, the path-scoped rules and the agent docs are claims about this repo, and nothing
# compiles them. Two things go wrong on their own: they grow until every session pays for facts
# it does not need, and they name a path that has since moved. Both are mechanical, so they are
# a check rather than something a reader is expected to notice.
#
# Checks:
#   1. session start: the root CLAUDE.md is the only file loaded before anyone types, so its
#      size is the whole session-start budget
#   2. each .claude/rules/*.md stays small enough to be cheap when its glob matches
#   3. each rule is path-scoped — a rule with no paths: key loads at every session start
#   4. every backticked repo path in CLAUDE.md, a rule, or docs/agents/ exists, is gitignored,
#      or is allowlisted because the file names it on purpose to say it is not there
#
# Usage: scripts/check-guidance.sh   (exit 1 on any failure, all failures listed)

set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 1

ALLOW=scripts/check-guidance.allow
fail=0
say() { printf '%s\n' "$*"; }
bad() { fail=1; say "FAIL  $*"; }

allowed() { # allowed <kind> <value>
  grep -qxF "$1: $2" "$ALLOW" 2>/dev/null
}

bytes() { wc -c < "$1" | tr -d ' '; }

# 1. Session start ----------------------------------------------------------------------
# Measured 2026-09-18, right after CON-37 cut the root file to orientation: 3,121 bytes
# (it was 34,981). The ceiling is that plus about 15%. Hitting it means a fact belongs in a
# path-scoped rule or in docs/ — see the "Where knowledge lives" table. Raising it is allowed,
# in its own commit, with the new measurement and the reason.
SESSION_START_CEILING=3600
root=$(bytes CLAUDE.md)
if [ "$root" -gt "$SESSION_START_CEILING" ]; then
  bad "CLAUDE.md is $root bytes; the ceiling is $SESSION_START_CEILING. Move something to the home it belongs in (CLAUDE.md, 'Where knowledge lives')"
fi

# 2 and 3. Rules ------------------------------------------------------------------------
RULE_CEILING=6000
for f in .claude/rules/*.md; do
  [ -f "$f" ] || continue
  n=$(bytes "$f")
  if [ "$n" -gt "$RULE_CEILING" ]; then
    bad "$f is $n bytes; the ceiling is $RULE_CEILING. Split it into two rules with the same paths:"
  fi
  # Frontmatter is lines 2 through the closing ---, so a `paths:` mention in the body
  # does not count as scoping.
  if ! sed -n '2,/^---$/p' "$f" | grep -qE '^paths:'; then
    bad "$f has no paths: frontmatter, so it loads at every session start; scope it or move it"
  fi
done

# 4. Paths ------------------------------------------------------------------------------
# What counts as a repo path is deliberately narrow, because these files are full of host
# paths (/etc/restic/restic.env, ~/caddy/), Go identifiers (sysutil.Opts), command lines and
# URLs that are not files in this tree. A token qualifies only when it is spelled the way a
# path in this repo is spelled: allowed characters only, no leading / or ./, no placeholder
# ellipsis, and either rooted at one of our top-level directories, a dot-directory, or a bare
# Markdown filename. A bare filename resolves next to the file that names it as well as at
# the repo root, so a rule pointing at a sibling rule is checked.
guidance_files() {
  ls CLAUDE.md .claude/rules/*.md docs/agents/*.md 2>/dev/null
}

for f in $(guidance_files); do
  dir=$(dirname "$f")
  # SC2016: the backticks in the grep pattern are the literal character we are matching on,
  # so the single quotes are the point.
  # shellcheck disable=SC2016
  while read -r p; do
    [ -z "$p" ] && continue
    bare=${p%/}
    [ -e "$bare" ] && continue
    [ -e "$dir/$bare" ] && continue
    git check-ignore -q "$bare" && continue
    allowed path "$p" && continue
    bad "$f names $p, which does not exist"
  done < <(grep -oE '`[^`]+`' "$f" | tr -d '`' \
    | grep -vE '[^A-Za-z0-9._/-]' \
    | grep -vE '^/|^\./|\.\.\.' \
    | grep -E '^(ctdev|docs|scripts)/|^\.[^/]*/|^[^/]+\.md$' \
    | sort -u)
done

if [ "$fail" -eq 0 ]; then say "guidance check: ok"; fi
exit "$fail"
