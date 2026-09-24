#!/usr/bin/env bash
# The unit test for the guidance itself.
#
# AGENTS.md, the path-scoped rules and the agent docs are claims about this repo, and nothing
# compiles them. Two things go wrong on their own: they grow until every session pays for facts
# it does not need, and they name a path that has since moved. Both are mechanical, so they are
# a check rather than something a reader is expected to notice. The README and the docs tree
# make the same kind of claim with every relative link, so those are checked here too.
#
# Checks:
#   1. session start: the root CLAUDE.md and the AGENTS.md it imports are the only files loaded
#      before anyone types, so their size is the whole session-start budget
#   2. each .claude/rules/*.md stays small enough to be cheap when its glob matches
#   3. each rule is path-scoped — a rule with no paths: key loads at every session start
#   4. every backticked repo path in AGENTS.md, a rule, or docs/agents/ exists, is gitignored,
#      or is allowlisted because the file names it on purpose to say it is not there
#   5. every relative Markdown link in README.md or under docs/ points at a file or directory
#      that exists, and its heading anchor, where it has one, is a heading in that file
#   6. the README stays a table of contents rather than growing back into the manual
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
# (it was 34,981). Raised 2026-09-24 by CON-86, which moved the text into AGENTS.md (CLAUDE.md
# is the one line that imports it) and added the working agreement from repo-template: 5,606
# bytes. The ceiling is that plus about 15%. Hitting it means a fact belongs in a path-scoped
# rule or in docs/ — see the "Where knowledge lives" table. Raising it is allowed, in its own
# commit, with the new measurement and the reason.
SESSION_START_CEILING=6400
root=$(( $(bytes CLAUDE.md) + $(bytes AGENTS.md) ))
if [ "$root" -gt "$SESSION_START_CEILING" ]; then
  bad "CLAUDE.md and AGENTS.md are $root bytes; the ceiling is $SESSION_START_CEILING. Move something to the home it belongs in (AGENTS.md, 'Where knowledge lives')"
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
  ls AGENTS.md .claude/rules/*.md docs/agents/*.md 2>/dev/null
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

# 5. Links ------------------------------------------------------------------------------
# The README is a table of contents, so a link that goes nowhere is the one way it can be
# wrong. Only what this tree can answer is checked: anything with a scheme (https:, mailto:)
# is skipped, because a run that fails when someone else's site is down is noise. A link
# that is dead on purpose goes in the allowlist as `link: <target>`, spelled as the link
# spells it.
link_files() {
  ls README.md 2>/dev/null
  find docs -name '*.md' 2>/dev/null | sort
}

# The file with fenced code blocks blanked rather than dropped, so a reported line number is
# still the line in the file. A fence closes only on the marker that opened it, which is what
# lets a ~~~ block show a ``` block.
prose() { # prose <file>
  # SC2016: the backticks are the fence marker being matched, not a command substitution.
  # shellcheck disable=SC2016
  awk '
    match($0, /^ ? ? ?(```|~~~)/) {
      mark = substr($0, RSTART + RLENGTH - 3, 3)
      if (fence == "") fence = mark
      else if (mark == fence) fence = ""
      print ""
      next
    }
    { print (fence == "" ? $0 : "") }
  ' "$1"
}

# "<line> <target>" for every inline link, image and reference definition. Matching on the
# `](target)` tail alone means link text that wraps across lines, as the README's does, is
# still found. Inline code is removed first: a link inside backticks is an example.
#
# This is a pattern match, not a Markdown parser, and it errs toward the file's own habits:
# a target with a ) in it, a link in an indented code block or an HTML comment, and an
# <a href> are not understood. The first two fail loudly and can be allowlisted; an <a href>
# is not checked at all.
links() { # links <file>
  local text
  # shellcheck disable=SC2016
  text=$(prose "$1" | sed 's/`[^`]*`//g')
  {
    grep -noE '\]\([^)]+\)' <<<"$text" | sed -E 's/^([0-9]+):\]\(/\1 /; s/\)$//'
    grep -noE '^ ? ? ?\[[^]^][^]]*\]:[[:space:]]*[^[:space:]]+' <<<"$text" \
      | sed -E 's/^([0-9]+):[^]]*\]:[[:space:]]*/\1 /'
  } | sed -E -e 's/^([0-9]+) <([^>]*)>.*/\1 \2/' -e t -e 's/^([0-9]+) ([^[:space:]]+).*/\1 \2/'
}

# The anchors GitHub gives a file's headings: lowercased, everything but letters, digits,
# spaces, - and _ dropped, each space a hyphen, and a repeated heading numbered -1, -2.
# The UTF-8 locale is what makes an em dash count as punctuation rather than as three bytes
# sed has no opinion on; the headings here use them. Explicit <a name="..."> and id="..."
# anchors count too. Only ATX (#) headings are read.
anchors() { # anchors <file>
  # shellcheck disable=SC2016
  prose "$1" | grep -E '^ ? ? ?#{1,6}[[:space:]]' \
    | sed -E 's/^ *#+[[:space:]]+//; s/[[:space:]]+#*[[:space:]]*$//; s/\[([^]]*)\]\([^)]*\)/\1/g' \
    | tr '[:upper:]' '[:lower:]' \
    | LC_ALL=C.UTF-8 sed 's/[^[:alnum:] _-]//g' \
    | tr ' ' '-' \
    | awk '{ n = seen[$0]++; print (n ? $0 "-" n : $0) }'
  prose "$1" | grep -oE '(name|id)="[^"]+"' | sed -E 's/^[a-z]+="//; s/"$//'
}

for f in $(link_files); do
  dir=$(dirname "$f")
  while read -r line target; do
    [ -z "$target" ] && continue
    [[ $target =~ ^[A-Za-z][A-Za-z0-9+.-]*: ]] && continue
    allowed link "$target" && continue
    path=${target%%#*}
    anchor=
    [ "$path" != "$target" ] && anchor=${target#*#}
    # A bare #anchor points into the file it is in; a leading / is the repo root, as GitHub
    # reads it; anything else is relative to the file.
    case $path in
      '') dest=$f ;;
      /*) dest=${path#/} ;;
      *) dest=$dir/$path ;;
    esac
    dest=${dest#./}
    if [ ! -e "$dest" ]; then
      bad "$f:$line links to $target, but $dest does not exist"
      continue
    fi
    [ -n "$anchor" ] || continue
    # Only Markdown has headings to check; an anchor into anything else (file.go#L10) is
    # GitHub's business.
    [[ -f $dest && $dest == *.md ]] || continue
    # Not `anchors | grep -q`: under pipefail that pipeline fails whenever anchors does, and
    # anchors ends on a grep that finds nothing in most files, so every heading would read
    # as missing.
    if ! grep -qxF -- "$anchor" <<<"$(anchors "$dest")"; then
      bad "$f:$line links to $target, but $dest has no heading that makes #$anchor"
    fi
  done < <(links "$f")
done

# 6. README size ------------------------------------------------------------------------
# Measured 2026-09-20, right after CON-56 cut the README to a table of contents: 5,597 bytes
# (it was 23,997). The ceiling is that plus about 15%, the same way the session-start ceiling
# is set. Hitting it means a section has grown into a page of its own — put it under docs/ and
# leave a paragraph and a link behind. Raising it is allowed, in its own commit, with the new
# measurement and the reason.
README_CEILING=6400
readme=$(bytes README.md)
if [ "$readme" -gt "$README_CEILING" ]; then
  bad "README.md is $readme bytes; the ceiling is $README_CEILING. It is a table of contents: move the detail to a page under docs/ and link it"
fi

if [ "$fail" -eq 0 ]; then say "guidance check: ok"; fi
exit "$fail"
