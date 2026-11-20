#!/usr/bin/env bash
# The tests for scripts/goal.sh.
#
# goal.sh moves branches, pushes, removes worktrees and merges pull requests, so a mistake in it
# loses work or ships it early. Each test runs it against a throwaway repo: a bare repo as origin,
# a clone as the primary checkout, and worktrees under the same temp dir. A stand-in gh, first on
# PATH, logs its arguments and answers from state files the tests write; it merges a pull request
# by merging into the bare repo, so landing and its cleanup run for real. A stand-in checks command
# logs its arguments and fails on demand. Nothing touches GitHub, the host or the real checks.
#
# Checks:
#   1. start: the goal branch is cut from origin's main, gets its worktree, is pushed, and has a
#      draft pull request; bad slugs and an existing goal are refused
#   2. slice: the slice branch is cut from the goal's head into its own worktree; the name goal
#      and a duplicate are refused
#   3. merge: merges, checks the changed files, pushes and cleans up; refuses uncommitted work;
#      failing checks undo the merge; a conflict is left to resolve and a re-run finishes it
#   4. drop: refuses a slice with commits not in the goal, removes a clean one
#   5. status: lists open slices and what is left, or says the goal is ready to land
#   6. land: refuses open slices, runs the full checks, marks the pull request ready, ignores an
#      all-skipped draft run, stops on a failed run or check, and on success merges exactly the
#      goal's head, removes the goal and fast-forwards the primary checkout's main
#   7. a refs/entire/ ref on origin makes the next push fail loudly
#
# Usage: scripts/goal-test.sh   (exit 1 on any failure, all failures listed)

set -uo pipefail

GOAL=$(cd "$(dirname "$0")" && pwd)/goal.sh
[ -x "$GOAL" ] || { echo "no executable $GOAL" >&2; exit 1; }

T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT

# Keep the user's git config, hooks and environment out of it.
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME="Example Tester" GIT_AUTHOR_EMAIL=test@example.invalid
export GIT_COMMITTER_NAME="Example Tester" GIT_COMMITTER_EMAIL=test@example.invalid
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES
unset GOAL_REMOTE GOAL_BASE
export GOAL_POLL=0 GOAL_SETTLE=0 GOAL_CI_TIMEOUT=5
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
esac
exec "$REAL_GIT" "$@"
EOF
chmod +x "$T/bin/git"

# The stand-in gh. Every call is logged; a form goal.sh does not use is an error, so a new gh call
# in goal.sh needs a stub answer here too.
cat >"$T/bin/gh" <<'EOF'
#!/usr/bin/env bash
set -uo pipefail
s=$GH_STATE
printf '%s\n' "$*" >>"$s/gh.log"
json="" jq=""
args=("$@")
for ((i = 0; i < ${#args[@]}; i++)); do
  case "${args[i]}" in
    --json) json=${args[i+1]} ;;
    --jq) jq=${args[i+1]} ;;
    --head) head=${args[i+1]} ;;
    --base) base=${args[i+1]} ;;
    --match-head-commit) match=${args[i+1]} ;;
  esac
done
unhandled() { echo "gh stub: unhandled: $*" >&2; exit 64; }
pr() { # pr <number or branch>: the pull request's state dir
  local d
  for d in "$s"/pr/*/; do
    [ -d "$d" ] || continue
    if [ "$1" = "$(basename "$d")" ] || [ "$1" = "$(cat "$d/branch")" ]; then
      printf '%s' "${d%/}"; return 0
    fi
  done
  echo "no pull requests found for $1" >&2; exit 1
}
case "$1 $2" in
  "pr create")
    [ ! -f "$s/prcreate.fail" ] || { echo "pull request create failed" >&2; exit 1; }
    n=$(( $(find "$s/pr" -mindepth 1 -maxdepth 1 | wc -l) + 1 ))
    mkdir -p "$s/pr/$n"
    echo "$head" >"$s/pr/$n/branch"; echo "$base" >"$s/pr/$n/base"; echo OPEN >"$s/pr/$n/state"
    if [[ " $* " == *" --draft "* ]]; then echo true; else echo false; fi >"$s/pr/$n/draft"
    echo "https://example.invalid/pull/$n" ;;
  "pr view")
    d=$(pr "$3"); n=$(basename "$d")
    case "$json" in
      url,isDraft,state)
        draft=""; [ "$(cat "$d/draft")" = true ] && draft=" draft"
        printf 'https://example.invalid/pull/%s  %s%s\n' "$n" "$(cat "$d/state")" "$draft" ;;
      number) echo "$n" ;;
      isDraft) cat "$d/draft" ;;
      state) cat "$d/state" ;;
      headRefOid,statusCheckRollup)
        want=$(printf '%s' "$jq" | sed -n 's/.*\.headRefOid != "\([0-9a-f]*\)".*/\1/p')
        [ -n "$want" ] || unhandled "$@"
        if [ "$(git --git-dir="$GH_BARE" rev-parse "refs/heads/$(cat "$d/branch")")" != "$want" ]; then
          echo "the pull request's head moved"
        elif [ -f "$s/rollup.json" ]; then
          # GitHub's shape, through goal.sh's own filter; HEAD_SHA stands for the pull request's head
          sed "s/HEAD_SHA/$want/" "$s/rollup.json" | jq -r "$jq"
        elif [ -f "$s/rollup_bad" ]; then
          cat "$s/rollup_bad"
        fi ;;
      *) unhandled "$@" ;;
    esac ;;
  "pr ready")
    d=$(pr "$3")
    if [[ " $* " == *" --undo "* ]]; then echo true; else echo false; fi >"$d/draft" ;;
  "pr merge")
    [ ! -f "$s/merge.fail" ] || { echo "merge refused: a review is required" >&2; exit 1; }
    d=$(pr "$3"); branch=$(cat "$d/branch")
    [[ " $* " == *" --merge "* ]] || unhandled "$@"
    tip=$(git --git-dir="$GH_BARE" rev-parse "refs/heads/$branch")
    [ "${match:-}" = "$tip" ] || { echo "head is $tip, not ${match:-unset}" >&2; exit 1; }
    w=$s/merge-work
    git clone -q "$GH_BARE" "$w" &&
      git -C "$w" merge -q --no-ff -m "Merge pull request #$(basename "$d")" "origin/$branch" &&
      git -C "$w" push -q origin "HEAD:refs/heads/$(cat "$d/base")" || exit 1
    rm -rf "$w"
    echo MERGED >"$d/state" ;;
  "run list")
    # $s/runlist.fail makes this one call fail, as a dropped connection would.
    if [ -f "$s/runlist.fail" ]; then rm -f "$s/runlist.fail"; echo "connection reset" >&2; exit 1; fi
    # $s/runs: one "id status conclusion createdAt" per line. After answering, runs.next (if any)
    # becomes the answer to the next poll.
    since=$(printf '%s' "$jq" | sed -n 's/.*\.createdAt >= "\([^"]*\)".*/\1/p')
    [ -n "$since" ] || unhandled "$@"
    if [ -f "$s/runs" ]; then
      while read -r id status conclusion created; do
        [ -n "$id" ] || continue
        [[ $created < $since ]] || printf '%s %s %s\n' "$id" "$status" "$conclusion"
      done <"$s/runs" | sort -k1,1nr
    fi
    [ ! -f "$s/runs.next" ] || mv "$s/runs.next" "$s/runs" ;;
  "run cancel")
    [ -n "${3:-}" ] || unhandled "$@" ;;
  "run view")
    [ "$json" = jobs ] || unhandled "$@"
    [ -f "$s/jobs/$3" ] || { echo "no run $3" >&2; exit 1; }
    grep -cvx skipped "$s/jobs/$3" || true ;;
  *) unhandled "$@" ;;
esac
EOF

cat >"$T/bin/checks" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$GH_STATE/checks.log"
[ ! -x "$GH_STATE/checks.hook" ] || "$GH_STATE/checks.hook"
[ ! -e "$GH_STATE/checks.fail" ]
EOF
chmod +x "$T/bin/gh" "$T/bin/checks"

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
  mkdir -p "$E/state/pr" "$E/state/jobs"
  : >"$E/out"; : >"$E/state/gh.log"; : >"$E/state/checks.log"
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
count() { grep -cF -- "$2" "$E/state/$1.log"; }          # count <gh|checks> <text>
out_has() { grep -qF -- "$1" "$E/out"; }
clean_tree() { [ -z "$(git -C "$1" status --porcelain)" ]; }
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

mkdir "$WT/demo/.merge.lock"
fails "slicing while a merge into the goal runs" "a merge into goal 'demo' is running" slice demo late
rmdir "$WT/demo/.merge.lock"
check "a slice refused during a merge makes no branch" no_branch slice/demo/late

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
check "the merge lock is released" test ! -e "$WT/demo/.merge.lock"

succeeds "slice for uncommitted work" slice demo dirty
printf 'unsaved\n' >"$WT/demo/dirty/README.md"
before=$(rev goal/demo)
fails "merging a slice with uncommitted changes" "uncommitted changes" merge demo dirty
check "an uncommitted slice is not merged" same "$(rev goal/demo)" "$before"
check "an uncommitted slice is kept" test -d "$WT/demo/dirty"
git -C "$WT/demo/dirty" checkout -q -- README.md
succeeds "dropping the clean slice" drop demo dirty

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
check "failing checks release the merge lock" test ! -e "$WT/demo/.merge.lock"
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
check "a conflict releases the merge lock" test ! -e "$WT/demo/.merge.lock"
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

touch "$E/state/checks.fail"
fails "landing when the full checks fail" "full checks failed" land demo
rm -f "$E/state/checks.fail"
check "the full checks ran on the goal worktree" logged checks "-C $gdir --full"
check "failing full checks leave the pull request a draft" test "$(count gh 'pr ready')" = 0

head=$(rev goal/demo)
printf '10 completed failure 2099-01-01T00:00:00Z\n' >"$E/state/runs"
printf 'success\nfailure\n' >"$E/state/jobs/10"
fails "landing when CI fails" "CI run 10 on $head ended 'failure'" land demo
check "land marks the pull request ready" logged gh "pr ready 1"
check "land waits for the run on the goal's head" logged_like gh "run list --workflow ci.yml --commit $head --event pull_request"
check "a failed CI run merges nothing" test "$(count gh 'pr merge')" = 0
check "a failed CI run keeps the goal worktree" test -d "$gdir"
check "a failed CI run puts the pull request back to draft" logged gh "pr ready 1 --undo"
check "the pull request is a draft again" same "$(cat "$E/state/pr/1/draft")" true

# A pull request someone marked ready by hand: land takes the run that exists, however old.
echo false >"$E/state/pr/1/draft"
: >"$E/state/gh.log"
printf '11 completed success 2000-01-01T00:00:00Z\n' >"$E/state/runs"
printf 'success\n' >"$E/state/jobs/11"
printf 'Example Check\n' >"$E/state/rollup_bad"
fails "landing when a check on the pull request failed" "not every check passed on #1: Example Check" land demo
rm -f "$E/state/rollup_bad"
check "an already-ready pull request is not marked ready again" test "$(count gh 'pr ready 1')" = "$(count gh 'pr ready 1 --undo')"
check "a failed check merges nothing" test "$(count gh 'pr merge')" = 0
check "a failed check puts the pull request back to draft" same "$(cat "$E/state/pr/1/draft")" true

# Rollups shaped like GitHub's, through goal.sh's own jq filter: a commit status has a context and a
# state but no name, and a check still running has no conclusion yet.
printf '12 completed success 2099-01-01T00:00:00Z\n' >"$E/state/runs"
printf 'success\n' >"$E/state/jobs/12"
printf '{"headRefOid":"%s","statusCheckRollup":[{"name":"Go Tests","conclusion":"SUCCESS"},{"name":"Lint","conclusion":"SKIPPED"},{"context":"example/status","state":"FAILURE"}]}\n' \
  "$head" >"$E/state/rollup.json"
fails "landing with a failing commit status" "not every check passed on #1: example/status" land demo
printf '{"headRefOid":"%s","statusCheckRollup":[{"name":"Go Tests","status":"IN_PROGRESS","conclusion":""}]}\n' \
  "$head" >"$E/state/rollup.json"
fails "landing with a check still running" "not every check passed on #1: Go Tests" land demo
rm -f "$E/state/rollup.json"
check "a failing status or a running check merges nothing" test "$(count gh 'pr merge')" = 0

: >"$E/state/runs"
GOAL_CI_TIMEOUT=1 fails "landing when no CI run appears" "no passing CI run on $head after 1s" land demo
check "a CI timeout merges nothing" test "$(count gh 'pr merge')" = 0

printf '30 in_progress none 2099-01-01T00:00:00Z\n' >"$E/state/runs"
GOAL_CI_TIMEOUT=0 fails "landing when the run is still going at the deadline" "no passing CI run on $head" land demo
check "a timed-out land cancels the run still going" logged gh "run cancel 30"
check "a timed-out land puts the pull request back to draft" same "$(cat "$E/state/pr/1/draft")" true

printf '31 completed success 2099-01-01T00:00:00Z\n' >"$E/state/runs"
printf 'success\n' >"$E/state/jobs/31"
touch "$E/state/merge.fail"
fails "landing when GitHub refuses the merge" "GitHub refused the merge" land demo
rm -f "$E/state/merge.fail"
check "a refused merge puts the pull request back to draft" same "$(cat "$E/state/pr/1/draft")" true
check "a refused merge keeps the goal worktree" test -d "$gdir"

GOAL_POLL=soon fails "a GOAL_POLL that is not whole seconds" "whole seconds" status demo

# Success: main moved meanwhile, the pull request is a draft again, the first poll sees only the
# draft run (all jobs skipped) and an old failure from before land, the next sees the real run.
git clone -q "$BARE" "$E/elsewhere"
commit "$E/elsewhere" .github/workflows/ci.yml "name: CI"
git -C "$E/elsewhere" push -q origin main
rm -rf "$E/elsewhere"
moved=$(origin_rev main)
# What a retried land sees: the failed job of an earlier run on this commit is still in the rollup.
printf '{"headRefOid":"HEAD_SHA","statusCheckRollup":[{"name":"Go Tests","workflowName":"CI","conclusion":"FAILURE"},{"name":"Go Tests","workflowName":"CI","conclusion":"SUCCESS"},{"context":"example/status","state":"SUCCESS"}]}\n' \
  >"$E/state/rollup.json"
echo true >"$E/state/pr/1/draft"
printf '20 completed skipped 2099-01-01T00:00:00Z\n5 completed failure 2000-01-01T00:00:00Z\n' >"$E/state/runs"
printf '20 completed skipped 2099-01-01T00:00:00Z\n21 completed success 2099-01-01T00:00:01Z\n' >"$E/state/runs.next"
printf 'skipped\nskipped\n' >"$E/state/jobs/20"
printf 'success\n' >"$E/state/jobs/21"
: >"$E/state/gh.log"; : >"$E/state/checks.log"
touch "$E/state/runlist.fail"
succeeds "land" land demo
check "land tries again when gh run list fails once" out_has "gh run list failed; trying again"
rm -f "$E/state/rollup.json"
landed=$(sed -n 's/^pr merge 1 --merge --match-head-commit //p' "$E/state/gh.log")
check "land merges the main that moved into the goal" same "$(git --git-dir="$BARE" rev-parse "$landed^2")" "$moved"
check "land deletes the remote goal branch" test -z "$(origin_rev goal/demo)"
check "land runs the full checks after merging main" logged checks "-C $gdir --full"
check "land marks the draft ready" logged gh "pr ready 1"
check "land looks at the draft run and skips it" logged gh "run view 20 --json jobs --jq [.jobs[] | select(.conclusion != \"skipped\")] | length"
check "land ignores runs from before it marked the pull request ready" not_logged_like gh "run view 5 "
check "land merges exactly the goal's head" logged gh "pr merge 1 --merge --match-head-commit $landed"
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

say "$passed passed in $(( $(date +%s) - start_time ))s"
if [ "$fail" -ne 0 ]; then
  say "goal-test: failures above"
  exit 1
fi
