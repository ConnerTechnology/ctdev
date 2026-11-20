#!/usr/bin/env bash
# The tests for scripts/goal.sh.
#
# goal.sh moves branches, pushes, removes worktrees and merges pull requests, so a mistake in it
# loses work or ships it early. Each test runs it against a throwaway repo: a bare repo as origin,
# a clone as the primary checkout, and worktrees under the same temp dir. A stand-in gh, first on
# PATH, logs its arguments and answers from state files the tests write, through goal.sh's own jq
# filters; it merges a pull request by merging into the bare repo, so landing and its cleanup run
# for real. A stand-in checks command logs its arguments, fails on demand and can be held, so two
# commands run at once for real. Both also log to one events log, so the order of land's steps shows. A stand-in docker fails any call. Nothing touches GitHub, Docker,
# the host or the real checks.
#
# Checks:
#   1. start: the goal branch is cut from origin's main, gets its worktree, is pushed, and has a
#      draft pull request; bad slugs and an existing goal are refused
#   2. slice: the slice branch is cut from the goal's head into its own worktree; the name goal
#      and a duplicate are refused
#   3. merge: merges, checks the changed files, pushes and cleans up; refuses uncommitted work;
#      failing checks undo the merge; a conflict is left to resolve and a re-run finishes it
#   4. drop: refuses a slice with commits not in the goal, removes a clean one; --force discards
#      commits but never uncommitted work
#   5. status: lists open slices and what is left, or says the goal is ready to land
#   6. land: refuses open slices; runs the full checks, then marks a draft ready, then merges
#      exactly the goal's head, and never asks GitHub for a CI run; failing checks touch no pull
#      request; a refused merge keeps the goal; on success removes the goal and fast-forwards the
#      primary checkout's main
#   7. a refs/entire/ ref on origin makes the next push fail loudly
#   8. two merges and a slice on one goal at once: the later ones wait for the lock, then run on
#      what the first left; both slices land in the goal and its worktree stays clean
#   9. two checks.sh runs at once name every container differently
#  10. a slug used again: the second goal lands through its own pull request, with the full
#      checks, and the first goal's merged one is never used
#  11. jq's // reads false as missing: a ready pull request (isDraft false) is read as ready and
#      not marked ready again
#  12. main moving under a goal: status counts how far behind it is and points at sync; sync
#      merges main, checks what it brings, undoes it when they fail, pushes; a conflict is left
#      to resolve and a re-run finishes it
#  13. a land killed after GitHub merged finishes its cleanup when run again, without checks
#  14. nothing called docker
#
# Usage: scripts/goal-test.sh   (exit 1 on any failure, all failures listed)

set -uo pipefail

REPO=$(cd "$(dirname "$0")/.." && pwd)
GOAL=$REPO/scripts/goal.sh
[ -x "$GOAL" ] || { echo "no executable $GOAL" >&2; exit 1; }

T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT

# Keep the user's git config, hooks and environment out of it.
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME="Example Tester" GIT_AUTHOR_EMAIL=test@example.invalid
export GIT_COMMITTER_NAME="Example Tester" GIT_COMMITTER_EMAIL=test@example.invalid
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES
unset GOAL_REMOTE GOAL_BASE
export GOAL_CHECKS=$T/bin/checks
REAL_GIT=$(command -v git)
export REAL_GIT
export PATH="$T/bin:$PATH"
mkdir -p "$T/bin"

# A git that fails on demand, for the failures a real remote has: an unreachable ls-remote, a
# commit that a hook refuses.
cat >"$T/bin/git" <<'EOF'
#!/usr/bin/env bash
case " $* " in
  *" ls-remote "*"refs/entire/"*)
    [ ! -f "${GH_STATE:-/nonexistent}/lsremote.fail" ] || { echo "fatal: unable to access the remote" >&2; exit 128; } ;;
  *" commit "*"chore: start goal"*)
    [ ! -f "${GH_STATE:-/nonexistent}/startcommit.fail" ] || { echo "a hook refused the commit" >&2; exit 1; } ;;
  *" fetch "*)
    [ ! -f "${GH_STATE:-/nonexistent}/fetch.fail" ] || { echo "fatal: unable to access the remote" >&2; exit 128; } ;;
esac
exec "$REAL_GIT" "$@"
EOF
chmod +x "$T/bin/git"

# Docker, for the whole run: goal.sh never needs it and the stand-in checks never call it, so any
# call is a mistake. It is logged, fails loudly, and the run fails at the end if anything called it.
# The test of checks.sh's container names puts its own docker ahead of this one.
cat >"$T/bin/docker" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$DOCKER_LOG"
echo "goal-test: docker $* was called; nothing in these tests may reach Docker" >&2
exit 97
EOF
chmod +x "$T/bin/docker"
export DOCKER_LOG=$T/docker.log
: >"$DOCKER_LOG"

# The stand-in gh. Every call is logged; a form goal.sh does not use is an error, so a new gh call
# in goal.sh needs a stub answer here too.
cat >"$T/bin/gh" <<'EOF'
#!/usr/bin/env bash
set -uo pipefail
s=$GH_STATE
printf '%s\n' "$*" >>"$s/gh.log"
printf 'gh %s\n' "$*" >>"$s/events.log"
json="" jq=""
args=("$@")
for ((i = 0; i < ${#args[@]}; i++)); do
  case "${args[i]}" in
    --json) json=${args[i+1]} ;;
    --jq) jq=${args[i+1]} ;;
    --head) head=${args[i+1]} ;;
    --base) base=${args[i+1]} ;;
    --match-head-commit) match=${args[i+1]} ;;
    --state) want_state=${args[i+1]} ;;
  esac
done
unhandled() { echo "gh stub: unhandled: $*" >&2; exit 64; }
numbers() { find "$s/pr" -mindepth 1 -maxdepth 1 -printf '%f\n' | sort -n; }
# pr <number or branch>: the pull request's state dir. Given a branch, the oldest pull request for
# it, merged or not: gh pr view <branch> can resolve a reused branch to an earlier goal's pull
# request, and goal.sh must not ask it that (the tests check it never does).
pr() {
  local n
  for n in $(numbers); do
    if [ "$1" = "$n" ] || [ "$1" = "$(cat "$s/pr/$n/branch")" ]; then
      printf '%s' "$s/pr/$n"; return 0
    fi
  done
  echo "no pull requests found for $1" >&2; exit 1
}
tip() { git --git-dir="$GH_BARE" rev-parse --verify --quiet "refs/heads/$1"; }
case "$1 $2" in
  "pr create")
    [ ! -f "$s/prcreate.fail" ] || { echo "pull request create failed" >&2; exit 1; }
    for n in $(numbers); do
      if [ "$(cat "$s/pr/$n/branch")" = "$head" ] && [ "$(cat "$s/pr/$n/state")" = OPEN ]; then
        echo "a pull request for branch \"$head\" already exists" >&2; exit 1
      fi
    done
    n=$(( $(numbers | wc -l) + 1 ))
    mkdir -p "$s/pr/$n"
    echo "$head" >"$s/pr/$n/branch"; echo "$base" >"$s/pr/$n/base"; echo OPEN >"$s/pr/$n/state"
    tip "$head" >"$s/pr/$n/head"
    if [[ " $* " == *" --draft "* ]]; then echo true; else echo false; fi >"$s/pr/$n/draft"
    echo "https://example.invalid/pull/$n" ;;
  "pr view")
    d=$(pr "$3"); n=$(basename "$d")
    case "$json" in
      state) cat "$d/state" ;;
      *) unhandled "$@" ;;
    esac ;;
  "pr list")
    # GitHub's JSON, newest first, through goal.sh's own --jq filter: isDraft is a real boolean, so
    # a filter that reads false as missing gets it wrong here as it would against GitHub. A merged
    # pull request keeps the head it was merged at; an open one's is its branch's tip.
    [ -n "${head:-}" ] && [ -n "$json" ] && [ -n "$jq" ] || unhandled "$@"
    for f in ${json//,/ }; do
      case "$f" in number|state|isDraft|url|headRefOid|headRefName) ;; *) unhandled "$@" ;; esac
    done
    want=$(printf '%s' "${want_state:-open}" | tr '[:lower:]' '[:upper:]')
    for n in $(numbers | sort -nr); do
      d=$s/pr/$n
      [ "$(cat "$d/branch")" = "$head" ] || continue
      st=$(cat "$d/state")
      [ "$want" = ALL ] || [ "$want" = "$st" ] || continue
      oid=$(cat "$d/head")
      [ "$st" != OPEN ] || oid=$(tip "$head") || oid=$(cat "$d/head")
      jq -n --argjson number "$n" --arg state "$st" --argjson isDraft "$(cat "$d/draft")" \
        --arg url "https://example.invalid/pull/$n" --arg headRefOid "$oid" --arg headRefName "$head" \
        '{number: $number, state: $state, isDraft: $isDraft, url: $url, headRefOid: $headRefOid, headRefName: $headRefName}'
    done | jq -s . | jq -r "$jq" ;;
  "pr ready")
    [ $# -eq 3 ] || unhandled "$@"
    d=$(pr "$3")
    echo false >"$d/draft" ;;
  "pr merge")
    [ ! -f "$s/merge.fail" ] || { echo "merge refused: a review is required" >&2; exit 1; }
    d=$(pr "$3"); branch=$(cat "$d/branch")
    [[ " $* " == *" --merge "* ]] || unhandled "$@"
    [ "$(cat "$d/draft")" = false ] || { echo "a draft pull request can't be merged" >&2; exit 1; }
    tip=$(git --git-dir="$GH_BARE" rev-parse "refs/heads/$branch")
    [ "${match:-}" = "$tip" ] || { echo "head is $tip, not ${match:-unset}" >&2; exit 1; }
    w=$s/merge-work
    git clone -q "$GH_BARE" "$w" &&
      git -C "$w" merge -q --no-ff -m "Merge pull request #$(basename "$d")" "origin/$branch" &&
      git -C "$w" push -q origin "HEAD:refs/heads/$(cat "$d/base")" || exit 1
    rm -rf "$w"
    echo "$tip" >"$d/head"
    echo MERGED >"$d/state"
    # $s/merge.interrupt: GitHub merged, then goal.sh is killed before its cleanup, once.
    if [ -f "$s/merge.interrupt" ]; then rm -f "$s/merge.interrupt"; kill -TERM "$PPID"; fi ;;
  *) unhandled "$@" ;;
esac
EOF

cat >"$T/bin/checks" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$GH_STATE/checks.log"
printf 'checks %s\n' "$*" >>"$GH_STATE/events.log"
[ ! -x "$GH_STATE/checks.hook" ] || "$GH_STATE/checks.hook" "$@"
[ ! -e "$GH_STATE/checks.fail" ]
EOF
chmod +x "$T/bin/gh" "$T/bin/checks"

# The checks stand-in's hook for the tests of commands run at once: while $GH_STATE/hold is there,
# the checks wait (the command running them holds the goal's lock), and $GH_STATE/held says so.
cat >"$T/bin/hold-hook" <<'EOF'
#!/usr/bin/env bash
[ -e "$GH_STATE/hold" ] || exit 0
touch "$GH_STATE/held"
for ((i = 0; i < 400; i++)); do [ -e "$GH_STATE/hold" ] || exit 0; sleep 0.05; done
echo "checks stand-in: never released" >&2
exit 1
EOF
chmod +x "$T/bin/hold-hook"

fail=0
passed=0
say() { printf '%s\n' "$*"; }
ok() { passed=$((passed + 1)); }
bad() { # bad <message>: a failure, with the last goal.sh output under it
  fail=1
  say "FAIL  $*"
  [ ! -s "$E/out" ] || sed 's/^/      | /' "$E/out"
}
check() { # check <description> <command...>: the command must succeed
  local what=$1; shift
  if "$@"; then ok; else bad "$what"; fi
}

# fresh <name>: a new origin with one commit on main, a clone of it as the primary checkout, and
# empty gh and checks state. Sets E, BARE, PRIMARY, WT.
fresh() {
  E=$T/$1
  BARE=$E/origin.git PRIMARY=$E/primary WT=$E/worktrees
  mkdir -p "$E/state/pr"
  : >"$E/out"; : >"$E/state/gh.log"; : >"$E/state/checks.log"; : >"$E/state/events.log"
  export GH_STATE=$E/state GH_BARE=$BARE GOAL_WORKTREES=$WT
  git init -q --bare "$BARE"
  git --git-dir="$BARE" symbolic-ref HEAD refs/heads/main
  git clone -q "$BARE" "$PRIMARY" 2>/dev/null
  git -C "$PRIMARY" symbolic-ref HEAD refs/heads/main
  commit "$PRIMARY" README.md "base"
  git -C "$PRIMARY" push -q -u origin main
}

goal() { # goal <args...>: run goal.sh from the primary checkout, output in $E/out
  (cd "$PRIMARY" && "$GOAL" "$@") >"$E/out" 2>&1
}
succeeds() { # succeeds <description> <goal args...>
  local what=$1; shift
  if goal "$@"; then ok; else bad "$what: goal.sh $* exited non-zero"; fi
}
fails() { # fails <description> <text the output must contain> <goal args...>
  local what=$1 text=$2; shift 2
  if goal "$@"; then
    bad "$what: goal.sh $* succeeded"
  elif ! grep -qF -- "$text" "$E/out"; then
    bad "$what: goal.sh $* failed without saying '$text'"
  else
    ok
  fi
}

commit() { # commit <dir> <file> <content>: write the file and commit it
  mkdir -p "$(dirname "$1/$2")"
  printf '%s\n' "$3" >"$1/$2"
  git -C "$1" add -- "$2" && git -C "$1" commit -q -m "test: write $2"
}
advance_main() { # advance_main <file>: a commit on origin's main from somewhere else
  local w=$E/elsewhere
  git clone -q "$BARE" "$w" && commit "$w" "$1" "elsewhere" && git -C "$w" push -q origin main
  rm -rf "$w"
}

rev() { git -C "$PRIMARY" rev-parse --verify --quiet "$1"; }
origin_rev() { git --git-dir="$BARE" rev-parse --verify --quiet "$1"; }
same() { [ -n "$1" ] && [ "$1" = "$2" ]; }
no_branch() { ! git -C "$PRIMARY" show-ref --verify --quiet "refs/heads/$1"; }
has_branch() { git -C "$PRIMARY" show-ref --verify --quiet "refs/heads/$1"; }
logged() { grep -qxF -- "$2" "$E/state/$1.log"; }        # logged <gh|checks> <exact line>
logged_like() { grep -qF -- "$2" "$E/state/$1.log"; }    # logged_like <gh|checks> <text>
not_logged_like() { ! logged_like "$@"; }
no_gh_run() { ! grep -q '^run ' "$E/state/gh.log"; }   # no_gh_run: goal.sh asked GitHub for no CI run
# land_steps: the steps land took, in order, from the events log: checks --full, pr ready, pr merge.
land_steps() { grep -E '^(checks .*--full|gh pr ready|gh pr merge)' "$E/state/events.log" | cut -d' ' -f1-3; }
count() { grep -cF -- "$2" "$E/state/$1.log"; }          # count <gh|checks> <text>
out_has() { grep -qF -- "$1" "$E/out"; }
not_out_has() { ! grep -qF -- "$1" "$E/out"; }
clean_tree() { [ -z "$(git -C "$1" status --porcelain)" ]; }
lock_free() { flock -n "$WT/$1/.lock" true; }            # lock_free <slug>: no goal command holds it
in_goal() { git -C "$PRIMARY" cat-file -e "goal/$1:$2" 2>/dev/null; }   # in_goal <slug> <file>
goal_bg() { # goal_bg <output file> <goal args...>: run goal.sh in the background; its pid in $!
  local out=$1; shift
  (cd "$PRIMARY" && exec "$GOAL" "$@") >"$out" 2>&1 &
}
wait_for() { # wait_for <description> <command...>: poll for up to 20 seconds
  local what=$1 i; shift
  for ((i = 0; i < 400; i++)); do "$@" && return 0; sleep 0.05; done
  bad "timed out waiting: $what"; return 1
}
has_text() { grep -qF -- "$2" "$1" 2>/dev/null; }
not_in_goal() { ! in_goal "$@"; }
# killed_land <slug>: a land the stand-in gh kills after GitHub merged; the subshell keeps the
# shell's "Terminated" notice out of the test's output.
killed_land() { (goal land "$1") 2>/dev/null; }          # has_text <file> <text>
# hold: the next checks run blocks, holding the goal's lock, until release. held says it is there.
hold() { cp "$T/bin/hold-hook" "$E/state/checks.hook"; touch "$E/state/hold"; }
release() { rm -f "$E/state/hold" "$E/state/checks.hook"; }
no_merge_in_progress() { [ ! -f "$(git -C "$1" rev-parse --git-path MERGE_HEAD)" ]; }

start_time=$(date +%s)

# 1. start -------------------------------------------------------------------------------------
say "start"
fresh start
advance_main later.txt   # origin's main is now ahead of the primary checkout's
main=$(origin_rev main)
succeeds "start" start demo "Demo goal"
gdir=$WT/demo/goal
check "start prints the goal worktree" same "$(tail -n 1 "$E/out")" "$gdir"
check "the goal worktree is at \$GOAL_WORKTREES/demo/goal on goal/demo" \
  same "$(git -C "$gdir" symbolic-ref --short HEAD 2>/dev/null)" goal/demo
check "the goal branch is cut from origin/main, not the local main" same "$(rev goal/demo~1)" "$main"
check "the goal branch is pushed" same "$(origin_rev goal/demo)" "$(rev goal/demo)"
check "a draft pull request is opened against main" \
  logged gh "pr create --draft --base main --head goal/demo --title Demo goal --body Goal \`demo\`. Built in slices with \`scripts/goal.sh\`; the description is written before it lands."
fails "a slug with capitals" "use lowercase" start Bad
fails "a slug with .." "use lowercase" start a..b
check "a bad slug makes no branch" no_branch goal/Bad
fails "starting an existing goal" "already exists" start demo
check "a refused start opens no second pull request" same "$(count gh 'pr create')" 1

# 2. slice -------------------------------------------------------------------------------------
say "slice"
succeeds "slice" slice demo one
check "slice prints its worktree" same "$(tail -n 1 "$E/out")" "$WT/demo/one"
check "the slice worktree is at \$GOAL_WORKTREES/demo/one on slice/demo/one" \
  same "$(git -C "$WT/demo/one" symbolic-ref --short HEAD 2>/dev/null)" slice/demo/one
check "the slice is cut from the goal's head" same "$(rev slice/demo/one)" "$(rev goal/demo)"
fails "a slice named goal" "can't be named 'goal'" slice demo goal
fails "a duplicate slice" "already exists" slice demo one
fails "a slice of a goal that does not exist" "no goal 'nope'" slice nope one
succeeds "path of the goal" path demo
check "path prints the goal worktree" same "$(cat "$E/out")" "$gdir"
succeeds "path of a slice" path demo one
check "path prints the slice worktree" same "$(cat "$E/out")" "$WT/demo/one"

# 3. merge -------------------------------------------------------------------------------------
say "merge"
commit "$WT/demo/one" one.txt "one"
commit "$WT/demo/one" sub/two.txt "two"
before=$(rev goal/demo); sliced=$(rev slice/demo/one)
succeeds "merge" merge demo one
check "the goal head is a merge of the slice" same "$(rev goal/demo^2)" "$sliced"
check "the goal head's first parent is the old goal head" same "$(rev goal/demo^1)" "$before"
check "the merged goal is pushed" same "$(origin_rev goal/demo)" "$(rev goal/demo)"
check "the slice worktree is removed" test ! -e "$WT/demo/one"
check "the slice branch is removed" no_branch slice/demo/one
check "the checks ran on the slice's files" logged checks "-C $gdir --changed one.txt sub/two.txt"
check "the goal lock is released" lock_free demo

succeeds "slice for uncommitted work" slice demo dirty
printf 'unsaved\n' >"$WT/demo/dirty/README.md"
before=$(rev goal/demo)
fails "merging a slice with uncommitted changes" "uncommitted changes" merge demo dirty
check "an uncommitted slice is not merged" same "$(rev goal/demo)" "$before"
check "an uncommitted slice is kept" test -d "$WT/demo/dirty"
git -C "$WT/demo/dirty" checkout -q -- README.md
succeeds "dropping the clean slice" drop demo dirty

# An ignored file is invisible to git status, and git worktree remove deletes it: what goal.sh
# removes must not take one with it, except what Entire and the build leave behind.
printf '*.local\n.entire/\n' >>"$(git -C "$PRIMARY" rev-parse --path-format=absolute --git-common-dir)/info/exclude"
succeeds "slice for an ignored file" slice demo ignored
commit "$WT/demo/ignored" ignored.txt "ignored"
printf 'notes\n' >"$WT/demo/ignored/notes.local"
mkdir -p "$WT/demo/ignored/.entire/logs" && printf 'log\n' >"$WT/demo/ignored/.entire/logs/x.log"
before=$(rev goal/demo)
fails "merging a slice with an ignored file" "notes.local" merge demo ignored
check "a slice with an ignored file is not merged" same "$(rev goal/demo)" "$before"
check "the ignored file is kept" test -f "$WT/demo/ignored/notes.local"
check "Entire's files are not named" not_out_has ".entire"
fails "dropping a slice with an ignored file" "notes.local" drop demo ignored --force
check "a refused drop keeps the ignored file" test -f "$WT/demo/ignored/notes.local"
rm "$WT/demo/ignored/notes.local"
succeeds "merging once the ignored file is gone, Entire's logs aside" merge demo ignored
check "the slice with only Entire's logs is removed" test ! -e "$WT/demo/ignored"

succeeds "slice for failing checks" slice demo red
commit "$WT/demo/red" red.txt "red"
before=$(rev goal/demo); pushed=$(origin_rev goal/demo)
touch "$E/state/checks.fail"
fails "merging when the checks fail" "merge was undone" merge demo red
rm -f "$E/state/checks.fail"
check "failing checks ran on the slice's files" logged checks "-C $gdir --changed red.txt"
check "failing checks leave the goal head where it was" same "$(rev goal/demo)" "$before"
check "failing checks push nothing" same "$(origin_rev goal/demo)" "$pushed"
check "failing checks leave the goal worktree clean" clean_tree "$gdir"
check "failing checks keep the slice branch" has_branch slice/demo/red
check "failing checks keep the slice worktree" test -d "$WT/demo/red"
check "failing checks release the goal lock" lock_free demo
succeeds "merging once the checks pass" merge demo red
check "the fixed slice is pushed" same "$(origin_rev goal/demo)" "$(rev goal/demo)"

succeeds "first conflicting slice" slice demo left
succeeds "second conflicting slice" slice demo right
commit "$WT/demo/left" README.md "left"
commit "$WT/demo/right" README.md "right"
succeeds "merging the first" merge demo left
pushed=$(origin_rev goal/demo)
fails "merging a conflicting slice" "conflicts with the goal" merge demo right
check "a conflict is left in the goal worktree to resolve" test -f "$(git -C "$gdir" rev-parse --git-path MERGE_HEAD)"
check "a conflict pushes nothing" same "$(origin_rev goal/demo)" "$pushed"
check "a conflict keeps the slice" has_branch slice/demo/right
check "a conflict releases the goal lock" lock_free demo
fails "merging again before resolving" "merge in progress" merge demo right
printf 'left and right\n' >"$gdir/README.md"
git -C "$gdir" add README.md && git -C "$gdir" commit -q --no-edit
: >"$E/state/checks.log"
succeeds "merging again after resolving" merge demo right
check "the resolution is pushed" same "$(origin_rev goal/demo)" "$(rev goal/demo)"
check "the resolved slice is removed" no_branch slice/demo/right
check "the resolved slice's worktree is removed" test ! -e "$WT/demo/right"
check "the checks ran on what the resolution adds" logged checks "-C $gdir --changed README.md"

say "merge: work that changes while the checks run"
succeeds "slice" slice demo late
commit "$WT/demo/late" late1.txt "first"
cat >"$E/state/checks.hook" <<EOF2
#!/usr/bin/env bash
printf 'second\n' >"$WT/demo/late/late2.txt"
git -C "$WT/demo/late" add late2.txt && git -C "$WT/demo/late" commit -q -m "test: late"
EOF2
chmod +x "$E/state/checks.hook"
fails "merging while the slice gains a commit" "commits made after it was merged" merge demo late
rm -f "$E/state/checks.hook"
check "a slice that gained a commit keeps its branch" has_branch slice/demo/late
succeeds "merging the late commit" merge demo late
check "the goal has the late commit" test -f "$gdir/late2.txt"

succeeds "slice" slice demo addgo
commit "$WT/demo/addgo" ctdev/a.go "package a"
succeeds "merging a Go file" merge demo addgo
succeeds "slice" slice demo mvgo
git -C "$WT/demo/mvgo" mv ctdev/a.go b.go
git -C "$WT/demo/mvgo" commit -q -m "test: move a.go out of ctdev"
: >"$E/state/checks.log"
succeeds "merging a move out of ctdev/" merge demo mvgo
check "the checks see the moved file's old path" logged_like checks "ctdev/a.go"

# 4. drop --------------------------------------------------------------------------------------
say "drop"
succeeds "slice to keep" slice demo kept
commit "$WT/demo/kept" kept.txt "kept"
fails "dropping a slice with commits" "1 commit(s) not in the goal" drop demo kept
check "a refused drop keeps the branch" has_branch slice/demo/kept
check "a refused drop keeps the worktree" test -d "$WT/demo/kept"
succeeds "empty slice" slice demo empty
succeeds "dropping an empty slice" drop demo empty
check "a dropped slice's branch is gone" no_branch slice/demo/empty
check "a dropped slice's worktree is gone" test ! -e "$WT/demo/empty"
succeeds "slice to discard" slice demo discard
commit "$WT/demo/discard" discard.txt "discard"
printf 'unsaved\n' >"$WT/demo/discard/discard.txt"
fails "drop --force with uncommitted changes" "uncommitted changes" drop demo discard --force
check "drop --force keeps uncommitted work" has_branch slice/demo/discard
check "drop --force keeps the worktree with uncommitted work" test -f "$WT/demo/discard/discard.txt"
git -C "$WT/demo/discard" checkout -q -- discard.txt
fails "drop with an unknown flag" "Usage:" drop demo discard --hard
before=$(rev goal/demo)
succeeds "drop --force a slice with commits" drop demo discard --force
check "drop --force says what it discarded" out_has "1 unmerged commit(s) are discarded"
check "drop --force removes the branch" no_branch slice/demo/discard
check "drop --force removes the worktree" test ! -e "$WT/demo/discard"
check "drop --force leaves the goal alone" same "$(rev goal/demo)" "$before"

# 5. status ------------------------------------------------------------------------------------
say "status"
succeeds "status with a slice open" status demo
check "status lists the open slice and its commits" out_has "kept"
check "status counts the open slice's commits" out_has "1 commit(s) to merge"
check "status says what is left" out_has "left     merge or drop every slice"
check "status shows the draft pull request" out_has "pr       https://example.invalid/pull/1  OPEN draft"
check "status says the goal is pushed" out_has "pushed   yes"
succeeds "merging the last slice" merge demo kept
succeeds "status with no slice open" status demo
check "status says the goal is ready to land" out_has "ready to land (scripts/goal.sh land demo)"

# 6. land --------------------------------------------------------------------------------------
say "land"
succeeds "slice left open" slice demo open
: >"$E/state/gh.log"; : >"$E/state/checks.log"
fails "landing with a slice open" "slices are still open" land demo
check "an open slice stops land before the checks" test ! -s "$E/state/checks.log"
check "an open slice stops land before the pull request is touched" test ! -s "$E/state/gh.log"
succeeds "dropping the open slice" drop demo open

: >"$E/state/gh.log"; : >"$E/state/events.log"
touch "$E/state/checks.fail"
fails "landing when the full checks fail" "full checks failed" land demo
rm -f "$E/state/checks.fail"
check "the full checks ran on the goal worktree" logged checks "-C $gdir --full"
check "failing full checks mark nothing ready" test "$(count gh 'pr ready')" = 0
check "failing full checks merge nothing" test "$(count gh 'pr merge')" = 0
check "failing full checks leave the pull request a draft" same "$(cat "$E/state/pr/1/draft")" true

touch "$E/state/merge.fail"
fails "landing when GitHub refuses the merge" "GitHub refused to merge pull request #1; it stays open" land demo
rm -f "$E/state/merge.fail"
check "a refused merge keeps the goal worktree" test -d "$gdir"
check "a refused merge keeps the goal branch" has_branch goal/demo
check "a refused merge leaves the pull request open" same "$(cat "$E/state/pr/1/state")" OPEN
check "a refused merge releases the goal lock" lock_free demo

# What the full checks didn't see must not merge: a commit in the goal worktree, or main moving.
cat >"$E/state/checks.hook" <<EOF2
#!/usr/bin/env bash
case " \$* " in *" --full "*) git -C "$gdir" commit -q --allow-empty -m "test: unchecked"; rm -f "\$0" ;; esac
EOF2
chmod +x "$E/state/checks.hook"
: >"$E/state/gh.log"
fails "landing when the goal gains a commit during the checks" "changed during the full checks" land demo
check "a commit made during the checks merges nothing" test "$(count gh 'pr merge')" = 0
cat >"$E/state/checks.hook" <<EOF2
#!/usr/bin/env bash
case " \$* " in *" --full "*)
  git clone -q "$BARE" "$E/during" && printf 'x\n' >"$E/during/during.txt" &&
    git -C "$E/during" add during.txt && git -C "$E/during" commit -q -m "test: during" &&
    git -C "$E/during" push -q origin main; rm -rf "$E/during" "\$0" ;;
esac
EOF2
chmod +x "$E/state/checks.hook"
fails "landing when main moves during the checks" "moved during the full checks" land demo
check "main moving during the checks merges nothing" test "$(count gh 'pr merge')" = 0

# Success: main moved meanwhile, and the pull request is a draft.
git clone -q "$BARE" "$E/elsewhere"
commit "$E/elsewhere" .github/workflows/ci.yml "name: CI"
git -C "$E/elsewhere" push -q origin main
rm -rf "$E/elsewhere"
moved=$(origin_rev main)
echo true >"$E/state/pr/1/draft"
: >"$E/state/gh.log"; : >"$E/state/checks.log"; : >"$E/state/events.log"
succeeds "land" land demo
landed=$(sed -n 's/^pr merge 1 --merge --match-head-commit //p' "$E/state/gh.log")
check "land merges the main that moved into the goal" same "$(git --git-dir="$BARE" rev-parse "$landed^2")" "$moved"
check "land deletes the remote goal branch" test -z "$(origin_rev goal/demo)"
check "land runs the full checks after merging main" logged checks "-C $gdir --full"
check "land runs the full checks, then marks the draft ready, then merges" \
  same "$(land_steps | tr '\n' ' ')" "checks -C $gdir gh pr ready gh pr merge "
check "land merges exactly the goal's head" logged gh "pr merge 1 --merge --match-head-commit $landed"
check "land never asks GitHub for a CI run" no_gh_run
check "origin's main has the goal" git --git-dir="$BARE" merge-base --is-ancestor "$landed" main
check "the goal worktree is removed" test ! -e "$gdir"
check "the goal's directory is removed" test ! -e "$WT/demo"
check "the local goal branch is removed" no_branch goal/demo
check "the primary checkout's main is fast-forwarded" same "$(rev main)" "$(origin_rev main)"
check "the primary checkout is clean" clean_tree "$PRIMARY"

# 7. refs/entire/ ------------------------------------------------------------------------------
say "refs/entire/"
fresh entire
succeeds "start before a checkpoint ref" start ent
succeeds "slice" slice ent work
commit "$WT/ent/work" work.txt "work"
git --git-dir="$BARE" update-ref refs/entire/checkpoints/v1 "$(origin_rev main)"
fails "merging with a refs/entire/ ref on origin" "refs/entire/checkpoints/v1" merge ent work
check "the refusal says why" out_has "session transcripts may be published"

fresh entire-branch
succeeds "start" start ent
succeeds "slice" slice ent work
commit "$WT/ent/work" work.txt "work"
git --git-dir="$BARE" update-ref refs/heads/entire/checkpoints/v1 "$(origin_rev main)"
fails "merging with Entire's legacy checkpoint branch on origin" "refs/heads/entire/checkpoints/v1" merge ent work

fresh entire-start
git --git-dir="$BARE" update-ref refs/entire/checkpoints/v1 "$(origin_rev main)"
fails "starting with a refs/entire/ ref on origin" "refs/entire/checkpoints/v1" start ent
check "no pull request is opened" test "$(count gh 'pr create')" = 0
check "a refused start leaves no goal branch" no_branch goal/ent
check "a refused start leaves no goal worktree" test ! -e "$WT/ent"
check "a refused start takes the goal branch off origin" test -z "$(origin_rev goal/ent)"

fresh entire-unreachable
succeeds "start" start ent
succeeds "slice" slice ent work
commit "$WT/ent/work" work.txt "work"
touch "$E/state/lsremote.fail"
fails "merging when the remote's refs can't be listed" "could not list the refs on origin" merge ent work
rm -f "$E/state/lsremote.fail"

fresh start-commit-fails
touch "$E/state/startcommit.fail"
fails "starting when the first commit is refused" "was not started" start ent
check "a start refused before the push leaves no goal branch" no_branch goal/ent
check "a start refused before the push leaves no goal worktree" test ! -e "$WT/ent"
rm -f "$E/state/startcommit.fail"
succeeds "starting again after a refused first commit" start ent

fresh start-fails
touch "$E/state/prcreate.fail"
fails "starting when the pull request can't be opened" "was not started" start ent
check "a failed start leaves no goal branch" no_branch goal/ent
check "a failed start leaves no goal worktree" test ! -e "$WT/ent"
check "a failed start takes the goal branch off origin" test -z "$(origin_rev goal/ent)"
rm -f "$E/state/prcreate.fail"
succeeds "starting again after a failed start" start ent

# 8. two commands on one goal at once ------------------------------------------------------------
# The first merge holds the goal's lock through its checks; a second merge and a slice wait for it
# rather than race on the goal worktree, and then run on what it left.
say "two commands on one goal at once"
fresh concurrent
succeeds "start" start demo
gdir=$WT/demo/goal
succeeds "slice a" slice demo a
succeeds "slice b" slice demo b
commit "$WT/demo/a" a.txt "a"
commit "$WT/demo/b" b.txt "b"
hold
goal_bg "$E/out-a" merge demo a; pid_a=$!
wait_for "the first merge's checks" test -e "$E/state/held"
goal_bg "$E/out-b" merge demo b; pid_b=$!
goal_bg "$E/out-c" slice demo c; pid_c=$!
wait_for "the second merge to wait" has_text "$E/out-b" "waiting for another command on goal 'demo'"
wait_for "the slice to wait" has_text "$E/out-c" "waiting for another command on goal 'demo'"
check "the second merge waits while the first holds the lock" not_in_goal demo b.txt
check "the slice waits too" no_branch slice/demo/c
release
wait "$pid_a"; rc_a=$?
wait "$pid_b"; rc_b=$?
wait "$pid_c"; rc_c=$?
for x in a b c; do sed "s/^/[$x] /" "$E/out-$x"; done >"$E/out"
check "the first merge succeeds" test "$rc_a" = 0
check "the merge that waited succeeds" test "$rc_b" = 0
check "the slice that waited succeeds" test "$rc_c" = 0
check "the goal has the first slice" in_goal demo a.txt
check "the goal has the second slice" in_goal demo b.txt
check "the goal is pushed with both" same "$(origin_rev goal/demo)" "$(rev goal/demo)"
check "the goal worktree is clean" clean_tree "$gdir"
check "the goal worktree has no merge in progress" no_merge_in_progress "$gdir"
check "both slices are removed" eval "no_branch slice/demo/a && no_branch slice/demo/b"
check "each merge checked only its own slice's files" \
  eval "logged checks '-C $gdir --changed a.txt' && logged checks '-C $gdir --changed b.txt'"
check "the slice that waited is cut after the merge it waited for" \
  git -C "$PRIMARY" cat-file -e slice/demo/c:a.txt
check "the lock is free afterwards" lock_free demo

# 9. checks.sh: two runs at once get their own containers ----------------------------------------
# checks.sh in a throwaway repo, with a docker stand-in that logs each container's name and holds
# each Go job until both runs are in theirs, so the names are compared while both runs are alive.
say "checks.sh: two runs at once"
C=$T/checks-runs
mkdir -p "$C/bin" "$C/repo/scripts" "$C/repo/ctdev" "$C/repo/.github/workflows"
cp "$REPO/scripts/checks.sh" "$C/repo/scripts/"
cp "$REPO/.github/workflows/ci.yml" "$C/repo/.github/workflows/"
printf '#!/usr/bin/env bash\nexit 0\n' >"$C/repo/scripts/check-guidance.sh"
chmod +x "$C/repo/scripts/check-guidance.sh"
printf 'module example.invalid/ctdev\n\ngo 1.22\n' >"$C/repo/ctdev/go.mod"
printf 'package main\n' >"$C/repo/ctdev/x.go"
git -C "$C/repo" init -q && git -C "$C/repo" add -A && git -C "$C/repo" commit -q -m "test: a tree to check"
cat >"$C/bin/docker" <<'EOF'
#!/usr/bin/env bash
case "$1" in
  info) exit 0 ;;
  run)
    name="" out="" prev=""
    for a in "$@"; do
      [ "$prev" != --name ] || name=$a
      [ "$prev" != -v ] || [[ $a != *:/out ]] || out=${a%:/out}
      prev=$a
    done
    printf '%s\n' "$name" >>"$CHECKS_DOCKER_LOG"
    if [[ $name == ctdev-checks-go-* ]]; then
      for ((i = 0; i < 400; i++)); do
        [ "$(grep -c '^ctdev-checks-go-' "$CHECKS_DOCKER_LOG")" -lt 2 ] || break
        sleep 0.05
      done
      # What the Go job leaves for the Ubuntu jobs.
      printf '#!/bin/sh\n' >"$out/ctdev" && chmod +x "$out/ctdev"
    fi ;;
  *) echo "checks docker stand-in: unexpected $*" >&2; exit 1 ;;
esac
EOF
chmod +x "$C/bin/docker"
export CHECKS_DOCKER_LOG=$C/docker.log
: >"$CHECKS_DOCKER_LOG"
PATH="$C/bin:$PATH" "$C/repo/scripts/checks.sh" -C "$C/repo" --changed ctdev/x.go >"$C/out1" 2>&1 & pid1=$!
PATH="$C/bin:$PATH" "$C/repo/scripts/checks.sh" -C "$C/repo" --changed ctdev/x.go >"$C/out2" 2>&1 & pid2=$!
wait "$pid1"; rc1=$?
wait "$pid2"; rc2=$?
cat "$C/out1" "$C/out2" "$CHECKS_DOCKER_LOG" >"$E/out"
check "both checks runs pass" test "$rc1$rc2" = 00
check "both runs were in their Go job at once" test "$(grep -c '^ctdev-checks-go-' "$CHECKS_DOCKER_LOG")" = 2
check "both runs started their Ubuntu jobs" test "$(grep -c '^ctdev-checks-ubuntu-' "$CHECKS_DOCKER_LOG")" -ge 2
check "every container has a name" test "$(grep -c . "$CHECKS_DOCKER_LOG")" = "$(wc -l <"$CHECKS_DOCKER_LOG")"
check "no two containers share a name" test -z "$(sort "$CHECKS_DOCKER_LOG" | uniq -d)"

# 10. a slug used again --------------------------------------------------------------------------
# The first goal demo leaves pull request #1 merged with head goal/demo, and the stand-in gh's
# pr view goal/demo answers with it. The second goal demo lands through its own #2.
say "a slug used again"
fresh reuse
gdir=$WT/demo/goal
succeeds "start the first goal" start demo
succeeds "slice" slice demo one
commit "$WT/demo/one" one.txt "one"
succeeds "merge" merge demo one
succeeds "land the first goal" land demo
first_head=$(cat "$E/state/pr/1/head")
succeeds "start the slug again" start demo
check "the second goal opens its own pull request" same "$(cat "$E/state/pr/2/state" 2>/dev/null)" OPEN
succeeds "slice" slice demo two
commit "$WT/demo/two" two.txt "two"
succeeds "merge" merge demo two
succeeds "status of the second goal" status demo
check "status shows the open pull request, not the merged one" out_has "pr       https://example.invalid/pull/2  OPEN draft"
: >"$E/state/gh.log"; : >"$E/state/checks.log"
succeeds "land the second goal" land demo
head=$(cat "$E/state/pr/2/head")
check "the full checks run for the second goal" logged checks "-C $gdir --full"
check "no CI run is asked for" no_gh_run
check "the second pull request is marked ready" logged gh "pr ready 2"
check "the second pull request is merged at the goal's head" logged gh "pr merge 2 --merge --match-head-commit $head"
check "the second pull request is merged" same "$(cat "$E/state/pr/2/state")" MERGED
check "the first pull request is not touched" eval "not_logged_like gh 'pr ready 1' && not_logged_like gh 'pr merge 1'"
check "the first pull request keeps its head" same "$(cat "$E/state/pr/1/head")" "$first_head"
check "goal.sh never asks for a pull request by branch" not_logged_like gh "pr view goal/"
check "the second goal's work is on main" git --git-dir="$BARE" cat-file -e main:two.txt

# 11. jq and false -------------------------------------------------------------------------------
# jq's // takes false for missing, so .isDraft // true reads a ready pull request as a draft. Here
# isDraft is false: a land that misreads it marks the pull request ready again.
say "jq and false"
check "goal.sh puts no // after isDraft" eval "! grep -nE 'isDraft[)]?[[:space:]]*//' '$GOAL'"
fresh ready
gdir=$WT/demo/goal
succeeds "start" start demo
echo false >"$E/state/pr/1/draft"
succeeds "status of a ready pull request" status demo
check "status reads isDraft false as ready" out_has "pr       https://example.invalid/pull/1  OPEN"
check "status does not call a ready pull request a draft" eval "! out_has 'OPEN draft'"
printf 'false\n' >"$E/state/pr/1/draft"
: >"$E/state/gh.log"
succeeds "land a pull request already ready" land demo
check "a ready pull request is not marked ready" not_logged_like gh "pr ready"
check "the ready pull request is merged" same "$(cat "$E/state/pr/1/state")" MERGED

# 12. main moves under a goal --------------------------------------------------------------------
say "main moves under a goal"
fresh sync
gdir=$WT/demo/goal
succeeds "start" start demo
advance_main moved.txt
succeeds "status after main moved" status demo
check "status counts the commit main is ahead by" out_has "(1 ahead of origin/main, 1 behind)"
check "status says sync brings main in" out_has "scripts/goal.sh sync demo brings main in"
advance_main moved2.txt
touch "$E/state/fetch.fail"
succeeds "status when the fetch fails" status demo
rm -f "$E/state/fetch.fail"
check "status says the counts are from the last fetch" out_has "1 behind, as of the last fetch"
printf 'stray\n' >"$gdir/stray.txt"
fails "sync with uncommitted changes in the goal worktree" "uncommitted changes" sync demo
rm -f "$gdir/stray.txt"
before=$(rev goal/demo); pushed=$(origin_rev goal/demo)
touch "$E/state/checks.fail"
fails "sync when the checks fail on what main brings" "merge was undone" sync demo
rm -f "$E/state/checks.fail"
check "failing checks leave the goal where it was" same "$(rev goal/demo)" "$before"
check "failing checks push nothing" same "$(origin_rev goal/demo)" "$pushed"
check "failing checks leave the goal worktree clean" clean_tree "$gdir"
: >"$E/state/checks.log"
succeeds "sync" sync demo
check "sync merges main into the goal" same "$(rev goal/demo^2)" "$(origin_rev main)"
check "sync's merge is named for it" same "$(git -C "$PRIMARY" log -1 --format=%s goal/demo)" "chore: merge main into goal demo"
check "sync checks only what main brings in" logged checks "-C $gdir --changed moved.txt moved2.txt"
check "sync pushes the goal" same "$(origin_rev goal/demo)" "$(rev goal/demo)"
check "sync releases the lock" lock_free demo
succeeds "status after sync" status demo
check "status reports nothing behind" out_has ", 0 behind)"
check "status does not suggest sync" eval "! out_has 'scripts/goal.sh sync'"
: >"$E/state/checks.log"
succeeds "sync with nothing new" sync demo
check "a sync with nothing new says so" out_has "the goal already has origin/main"
check "a sync with nothing new runs no checks" test ! -s "$E/state/checks.log"

say "main moves under a goal: a conflict"
succeeds "slice" slice demo readme
commit "$WT/demo/readme" README.md "goal side"
succeeds "merge" merge demo readme
advance_main README.md
pushed=$(origin_rev goal/demo)
fails "sync that conflicts" "main conflicts with the goal" sync demo
check "the conflict is left in the goal worktree" test -f "$(git -C "$gdir" rev-parse --git-path MERGE_HEAD)"
check "a conflict pushes nothing" same "$(origin_rev goal/demo)" "$pushed"
check "a conflict releases the lock" lock_free demo
fails "sync again before resolving" "merge in progress" sync demo
printf 'goal side and elsewhere\n' >"$gdir/README.md"
git -C "$gdir" add README.md && git -C "$gdir" commit -q --no-edit
: >"$E/state/checks.log"
succeeds "sync after resolving" sync demo
check "the resolution is pushed" same "$(origin_rev goal/demo)" "$(rev goal/demo)"
check "the checks ran on what the resolution brings" logged checks "-C $gdir --changed README.md"
succeeds "status after the resolved sync" status demo
check "status reports nothing behind after the resolved sync" out_has ", 0 behind)"

# 13. land run again after an interruption -------------------------------------------------------
say "land run again after an interruption"
fresh resume
gdir=$WT/demo/goal
succeeds "start" start demo
succeeds "slice" slice demo one
commit "$WT/demo/one" one.txt "one"
succeeds "merge" merge demo one
touch "$E/state/merge.interrupt"
if killed_land demo; then bad "land killed after GitHub merged exited 0"; else ok; fi
check "GitHub merged the pull request" same "$(cat "$E/state/pr/1/state")" MERGED
check "the interrupted land left the goal worktree" test -d "$gdir"
check "the interrupted land left the goal branch" has_branch goal/demo
: >"$E/state/gh.log"; : >"$E/state/checks.log"
succeeds "land again" land demo
check "land again says it only cleans up" out_has "pull request #1 already merged goal/demo"
check "land again runs no checks" test ! -s "$E/state/checks.log"
check "land again marks nothing ready" not_logged_like gh "pr ready"
check "land again merges nothing" not_logged_like gh "pr merge"
check "land again removes the goal worktree" test ! -e "$WT/demo"
check "land again removes the local goal branch" no_branch goal/demo
check "land again deletes the remote goal branch" test -z "$(origin_rev goal/demo)"
check "land again fast-forwards the primary checkout" same "$(rev main)" "$(origin_rev main)"

say "land run again when only the remote branch is left"
succeeds "start" start rest
succeeds "slice" slice rest one
commit "$WT/rest/one" rest.txt "rest"
succeeds "merge" merge rest one
touch "$E/state/merge.interrupt"
if killed_land rest; then bad "land killed after GitHub merged exited 0"; else ok; fi
git -C "$PRIMARY" worktree remove "$WT/rest/goal"
git -C "$PRIMARY" branch -q -D goal/rest
: >"$E/state/gh.log"; : >"$E/state/checks.log"
succeeds "land again with only the remote branch" land rest
check "it runs no checks" test ! -s "$E/state/checks.log"
check "it merges nothing" not_logged_like gh "pr merge"
check "it deletes the remote goal branch" test -z "$(origin_rev goal/rest)"
check "it leaves nothing under the worktree root" test ! -e "$WT"

# 14. Docker -------------------------------------------------------------------------------------
say "docker"
cat "$DOCKER_LOG" >"$E/out"
check "nothing called the real docker (or the stand-in for it)" test ! -s "$DOCKER_LOG"

say "$passed passed in $(( $(date +%s) - start_time ))s"
if [ "$fail" -ne 0 ]; then
  say "goal-test: failures above"
  exit 1
fi
