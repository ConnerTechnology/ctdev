#!/usr/bin/env bash
# Run a goal from start to main: one goal branch and one pull request, built in slices.
#
# A goal is one change the human asked for. It gets a goal branch, goal/<slug>, cut from the base
# branch and pushed with a draft pull request. The work is done in slices: local branches,
# slice/<slug>/<name>, each in its own worktree, cut from the goal branch's head. A slice is merged
# into the goal branch here, never through GitHub, so slices never leave this machine. When the goal
# is done, land runs the full checks, marks the pull request ready so GitHub's checks run once, and
# merges it when they pass. See docs/agents/goal-loop.md.
#
# Every worktree lives under one root, ../.worktrees/<repo>/<slug>/, next to the primary checkout:
# the goal in goal/, each slice in <name>/. The script removes what it creates.
#
# Commands that change a goal (slice, merge, drop, sync, land) take its lock (flock), so they run
# one at a time on each goal; one that finds the lock held says so and waits for it.
#
# Usage:
#   scripts/goal.sh start <slug> [title]    goal branch, its worktree, and a draft pull request
#   scripts/goal.sh slice <slug> <name>     a slice worktree, cut from the goal branch's head
#   scripts/goal.sh merge <slug> <name>     check the slice, merge it into the goal, push, clean up
#   scripts/goal.sh drop  <slug> <name> [--force]
#                                           remove a slice; --force discards its unmerged commits
#   scripts/goal.sh sync  <slug>            merge the base into the goal, check what it brings, push
#   scripts/goal.sh status <slug>           ahead and behind the base, the slices, what is left
#   scripts/goal.sh land <slug>             full checks, then GitHub's checks, then merge and clean
#                                           up; run again after an interruption, it finishes
#   scripts/goal.sh path <slug> [name]      print a worktree's path
#
# Settings (environment): GOAL_REMOTE (origin), GOAL_BASE (main), GOAL_WORKTREES (the root above),
# GOAL_CHECKS (the goal worktree's scripts/checks.sh), GOAL_POLL (30 seconds between CI polls),
# GOAL_CI_TIMEOUT (5400 seconds), GOAL_SETTLE (10 seconds between land's push and marking the
# pull request ready).
set -eEuo pipefail
# set -e alone exits without a word; name the command that stopped the script.
trap 'printf "goal: stopped at line %s: %s\n" "$LINENO" "$BASH_COMMAND" >&2' ERR

usage() { sed -n '/^# Usage:/,/^# Settings/p' "$0" | sed '$d; s/^# \{0,1\}//' >&2; exit 2; }
die() { printf 'goal: %s\n' "$*" >&2; exit 1; }
say() { printf '==> %s\n' "$*" >&2; }

remote=${GOAL_REMOTE:-origin}
base=${GOAL_BASE:-main}
poll=${GOAL_POLL:-30}
ci_timeout=${GOAL_CI_TIMEOUT:-5400}
settle=${GOAL_SETTLE:-10}
for n in "$poll" "$ci_timeout" "$settle"; do
  [[ $n =~ ^[0-9]+$ ]] || { printf 'goal: GOAL_POLL, GOAL_CI_TIMEOUT and GOAL_SETTLE are whole seconds\n' >&2; exit 2; }
done

# The primary checkout, found from any worktree of the repo, including a goal's or a slice's.
primary=$(git worktree list --porcelain 2>/dev/null | sed -n '1s/^worktree //p')
[ -n "$primary" ] || die "not inside a git repository"
root=${GOAL_WORKTREES:-$(dirname "$primary")/.worktrees/$(basename "$primary")}
g() { git -C "$primary" "$@"; }
# gh finds the repository from the current directory.
cd "$primary"

valid() { # valid <what> <value>: lowercase words joined by - or ., so it is safe in a ref and a path
  if [[ ! $2 =~ ^[a-z0-9][a-z0-9.-]*$ ]] || [[ $2 == *..* ]]; then
    die "$1 '$2': use lowercase letters, digits, - and ."
  fi
}

goal_branch() { printf 'goal/%s' "$1"; }
slice_branch() { printf 'slice/%s/%s' "$1" "$2"; }
goal_dir() { printf '%s/%s/goal' "$root" "$1"; }
slice_dir() { printf '%s/%s/%s' "$root" "$1" "$2"; }
has_branch() { g show-ref --verify --quiet "refs/heads/$1"; }
on_remote() { [ -n "$(g ls-remote "$remote" "refs/heads/$1")" ]; }
clean() { [ -z "$(git -C "$1" status --porcelain)" ]; }

# git worktree remove deletes ignored files without a word, and status doesn't show them, so a file
# written under a .gitignore pattern (a new .claude/ path, say) would vanish with its worktree. What
# Entire, Claude Code and a build leave behind is disposable; anything else stops the removal.
disposable='^(\.entire/|\.claude/scheduled_tasks\.lock$|\.playwright-mcp/|ctdev/ctdev$|ctdev/dist/)'
kept_files() { # kept_files <dir>: the ignored files in a worktree that removing it would lose
  git -C "$1" ls-files --others --ignored --exclude-standard --directory | grep -Ev "$disposable" || true
}
removable() { # removable <dir> <what to do after>: die, naming them, if removal would lose ignored files
  local kept
  kept=$(kept_files "$1")
  [ -z "$kept" ] || die "$1 has ignored files its removal would delete:
$kept
Commit them (git add -f, or unignore the path), move them out, or delete them, then $2"
}

need_goal() {
  has_branch "$(goal_branch "$1")" || die "no goal '$1' (start it with: scripts/goal.sh start $1)"
  [ -d "$(goal_dir "$1")" ] || die "goal '$1' has no worktree at $(goal_dir "$1")"
}

# Checkpoints hold session transcripts and the repo may be public, so they stay on this machine.
# Entire is set not to push them, and its pre-push hook is the only thing that would: the script's
# pushes skip it (--no-verify) and name only the goal branch. This then makes sure, after every push,
# that no checkpoint is on the remote under either of Entire's backends (refs/entire/ refs, or the
# legacy entire/checkpoints/v1 branch).
entire_refs_absent() {
  local refs
  # Called under || and !, where set -e is off: an unreachable remote must not read as clean.
  refs=$(g ls-remote "$remote" 'refs/entire/*' 'refs/heads/entire/*') || {
    printf 'goal: could not list the refs on %s, so it is unknown whether transcripts are there\n' "$remote" >&2
    return 1
  }
  [ -z "$refs" ] && return 0
  printf 'goal: Entire refs are on %s, so session transcripts may be published:\n%s\n' "$remote" "$refs" >&2
  printf 'Delete them (git push %s --delete <ref>) and find what pushed them before going on.\n' "$remote" >&2
  return 1
}
no_entire_refs() { entire_refs_absent || exit 1; }

push_goal() { # push_goal <slug>: push the goal branch, and only it
  git -C "$(goal_dir "$1")" push --quiet --no-verify "$remote" \
    "refs/heads/$(goal_branch "$1"):refs/heads/$(goal_branch "$1")"
  no_entire_refs
}

checks_cmd() { # checks_cmd <slug>: the checks command, or die before anything has changed
  local cmd=${GOAL_CHECKS:-$(goal_dir "$1")/scripts/checks.sh}
  [ -x "$cmd" ] || command -v "$cmd" >/dev/null || die "no checks command at $cmd (set GOAL_CHECKS)"
  printf '%s' "$cmd"
}

checks() { # checks <slug> <args...>: returns the checks' status, so the caller can undo a merge
  local cmd
  cmd=$(checks_cmd "$1") || return 1
  shift
  "$cmd" "$@"
}

cmd_start() {
  local slug=$1 title=${2:-} branch dir
  valid slug "$slug"
  branch=$(goal_branch "$slug"); dir=$(goal_dir "$slug")
  has_branch "$branch" && die "branch $branch already exists"
  [ -e "$dir" ] && die "$dir already exists"
  g fetch --quiet "$remote" "$base"
  [ -z "$(g ls-remote --heads "$remote" "$branch")" ] || die "$branch already exists on $remote"
  mkdir -p "$(dirname "$dir")"
  # A goal without its pull request can't be landed or started again, so a start that fails takes
  # down what it made. The branch was checked absent here and on the remote above, so it is ours.
  # GitHub opens no pull request for a branch with no commits beyond its base: hence the commit.
  if ! g worktree add --quiet --no-track -b "$branch" "$dir" "$remote/$base" ||
     ! git -C "$dir" commit --quiet --allow-empty -m "chore: start goal $slug" \
       -m "Goal $slug: the first commit, so its pull request can open." ||
     ! git -C "$dir" push --quiet --no-verify "$remote" "refs/heads/$branch:refs/heads/$branch" ||
     ! entire_refs_absent ||
     ! gh pr create --draft --base "$base" --head "$branch" --title "${title:-goal: $slug}" \
       --body "Goal \`$slug\`. Built in slices with \`scripts/goal.sh\`; the description is written before it lands." >&2; then
    say "start failed: removing goal $slug"
    [ -z "$(g ls-remote --heads "$remote" "$branch" 2>/dev/null)" ] ||
      g push --quiet --no-verify "$remote" --delete "$branch" || true
    [ ! -d "$dir" ] || g worktree remove --force "$dir" || true
    ! has_branch "$branch" || g branch --quiet -D "$branch" || true
    g worktree prune
    rmdir "$root/$slug" 2>/dev/null || true
    die "goal $slug was not started"
  fi
  say "goal $slug started in $dir"
  printf '%s\n' "$dir"
}

# One command that changes a goal at a time: two would race on the goal worktree or its slices. The
# lock is held on fd 9 until the script exits; a second command waits for it and then looks again at
# what the first left. ERR is not raised inside the || below, so waiting is not an error.
lock() { # lock <slug>
  local f="$root/$1/.lock"
  command -v flock >/dev/null || die "flock is needed for the goal lock (util-linux; on macOS: brew install flock)"
  mkdir -p "$root/$1"
  exec 9>"$f"
  if ! flock -n 9; then
    say "waiting for another command on goal '$1' to finish ($f)"
    flock 9
  fi
}
# unlock <slug>: land's last step. The lock file goes with the goal's directory; a command that was
# waiting on it finds the goal gone.
unlock() { rm -f "$root/$1/.lock"; exec 9>&-; }

# A merge or sync that stopped on a conflict leaves it in the goal worktree for the session.
goal_worktree_ready() { # goal_worktree_ready <slug> <command to run again>
  local gdir
  gdir=$(goal_dir "$1")
  if [ -f "$(git -C "$gdir" rev-parse --git-path MERGE_HEAD)" ]; then
    die "the goal worktree has a merge in progress: resolve it in $gdir, commit, then run $2 again"
  fi
  clean "$gdir" || die "the goal worktree $gdir has uncommitted changes"
}

cmd_slice() {
  local slug=$1 name=$2 branch dir
  valid slug "$slug"; valid slice "$name"
  [ "$name" != goal ] || die "a slice can't be named 'goal': that is the goal's own worktree"
  need_goal "$slug"
  # A merge commits before its checks run, so a slice cut during one could start from a merge they
  # undo: wait for it.
  lock "$slug"
  need_goal "$slug"
  branch=$(slice_branch "$slug" "$name"); dir=$(slice_dir "$slug" "$name")
  has_branch "$branch" && die "slice $branch already exists"
  [ -e "$dir" ] && die "$dir already exists"
  g worktree add --quiet -b "$branch" "$dir" "$(goal_branch "$slug")"
  say "slice $name of goal $slug at $(g rev-parse --short "$branch") in $dir"
  printf '%s\n' "$dir"
}

remove_slice() { # remove_slice <slug> <name> <tip>: the worktree, then the branch if it is still at <tip>
  local dir branch
  dir=$(slice_dir "$1" "$2"); branch=$(slice_branch "$1" "$2")
  [ ! -d "$dir" ] || removable "$dir" "run this again"
  [ ! -d "$dir" ] || g worktree remove "$dir"
  g worktree prune
  # Only at the tip that was checked: a commit made since is not lost with the branch.
  g update-ref -d "refs/heads/$branch" "$3" ||
    die "slice $2 gained a commit while it was being removed: its branch is kept, run this again"
}

# check_push <slug> <1 if a merge was just made> <why, undoing it> <why, otherwise>: run the checks on
# what the goal worktree's head adds to the pushed goal, then push. Failing checks undo a merge just
# made, so the goal is as it was.
check_push() {
  local gdir files f
  gdir=$(goal_dir "$1")
  # The caller fetched the goal branch before merging, so a failed fetch never strands a merge.
  files=()
  # --no-renames lists a moved file under its old path too, so moving it out of ctdev/ still runs Go.
  while IFS= read -r f; do files+=("$f"); done < <(git -C "$gdir" diff --name-only --no-renames \
    "$remote/$(goal_branch "$1")" HEAD)
  if [ ${#files[@]} -gt 0 ] && ! checks "$1" -C "$gdir" --changed "${files[@]}"; then
    if [ "$2" = 1 ]; then
      git -C "$gdir" reset --quiet --hard ORIG_HEAD
      die "$3"
    fi
    die "$4"
  fi
  push_goal "$1"
}

cmd_merge() {
  local slug=$1 name=$2 branch dir gdir merged_now=0 tip
  valid slug "$slug"; valid slice "$name"
  need_goal "$slug"
  branch=$(slice_branch "$slug" "$name"); dir=$(slice_dir "$slug" "$name"); gdir=$(goal_dir "$slug")
  has_branch "$branch" || die "no slice '$name' in goal '$slug'"
  [ ! -d "$dir" ] || clean "$dir" || die "slice $name has uncommitted changes in $dir: commit or drop them"
  [ ! -d "$dir" ] || removable "$dir" "merge again"
  checks_cmd "$slug" >/dev/null
  lock "$slug"
  need_goal "$slug"
  has_branch "$branch" || die "no slice '$name' in goal '$slug'"
  goal_worktree_ready "$slug" merge
  # Fetched before merging: a fetch that fails after it would leave an unchecked merge in the goal.
  g fetch --quiet "$remote" "$(goal_branch "$slug")" || die "could not fetch $(goal_branch "$slug") from $remote"
  if ! git -C "$gdir" merge-base --is-ancestor "$branch" HEAD; then
    say "merging slice $name into $(goal_branch "$slug")"
    if ! git -C "$gdir" merge --quiet --no-ff -m "chore: merge slice $name" \
      -m "Goal $slug: slice $name, checked and merged by scripts/goal.sh." "$branch"; then
      die "slice $name conflicts with the goal. Resolve it in $gdir and commit, then run this again"
    fi
    merged_now=1
  fi
  # What this push will add to the goal: the slice, plus any conflict resolution.
  check_push "$slug" "$merged_now" \
    "checks failed, so the merge was undone. Fix slice $name in $dir and run this again" \
    "checks failed on the goal branch. Fix it in $gdir and run this again"
  # The checks can take minutes; a commit made on the slice meanwhile is not in the goal, and
  # deleting the branch would lose it.
  tip=$(g rev-parse "$branch")
  git -C "$gdir" merge-base --is-ancestor "$tip" HEAD ||
    die "slice $name has commits made after it was merged: run this again to merge them"
  remove_slice "$slug" "$name" "$tip"
  say "slice $name merged into $(goal_branch "$slug") and pushed; its worktree and branch are gone"
}

# --force discards the slice's commits, never uncommitted work: a worktree with changes is refused
# either way, so what was never committed can't be lost by a flag.
cmd_drop() {
  local slug=$1 name=$2 force=${3:-} branch dir ahead tip
  valid slug "$slug"; valid slice "$name"
  need_goal "$slug"
  branch=$(slice_branch "$slug" "$name"); dir=$(slice_dir "$slug" "$name")
  has_branch "$branch" || die "no slice '$name' in goal '$slug'"
  lock "$slug"
  has_branch "$branch" || die "no slice '$name' in goal '$slug'"
  [ ! -d "$dir" ] || clean "$dir" ||
    die "slice $name has uncommitted changes in $dir: commit or discard them first (--force drops only commits)"
  tip=$(g rev-parse "$branch")
  ahead=$(g rev-list --count "$(goal_branch "$slug")..$tip")
  [ "$ahead" = 0 ] || [ "$force" = --force ] ||
    die "slice $name has $ahead commit(s) not in the goal: merge it, or drop it with --force to discard them"
  remove_slice "$slug" "$name" "$tip"
  if [ "$ahead" = 0 ]; then say "slice $name dropped"; else say "slice $name dropped; its $ahead unmerged commit(s) are discarded"; fi
}

# Brings what reached the base into the goal between merges, so the slices cut next start from it
# and a conflict with it comes up now rather than at land.
cmd_sync() {
  local slug=$1 gdir merged_now=0
  valid slug "$slug"
  need_goal "$slug"
  gdir=$(goal_dir "$slug")
  checks_cmd "$slug" >/dev/null
  lock "$slug"
  need_goal "$slug"
  goal_worktree_ready "$slug" sync
  g fetch --quiet "$remote" "$base" "$(goal_branch "$slug")" || die "could not fetch from $remote"
  if git -C "$gdir" merge-base --is-ancestor "$remote/$base" HEAD; then
    say "the goal already has $remote/$base"
  else
    say "merging $remote/$base into $(goal_branch "$slug")"
    git -C "$gdir" merge --quiet --no-ff -m "chore: merge $base into goal $slug" \
      -m "Goal $slug: brings in what reached $base, checked by scripts/goal.sh sync." "$remote/$base" ||
      die "$base conflicts with the goal. Resolve it in $gdir and commit, then run sync again"
    merged_now=1
  fi
  check_push "$slug" "$merged_now" \
    "checks failed on what $base brings in, so the merge was undone. Merge $remote/$base in $gdir by hand, fix what breaks, commit, then run sync again" \
    "checks failed on the goal branch. Fix it in $gdir and run sync again"
  say "goal $slug has $remote/$base and is pushed"
}

# The goal's pull requests, found by head branch and state. A slug used before has an old merged
# pull request with the same head branch, and gh pr view <branch> may pick that one, so the goal's
# own is the open one, or a merged one only when its head is the goal's head.
open_pr() { # open_pr <branch>: "<number> <isDraft> <url>", or nothing; fails when gh does
  gh pr list --head "$1" --state open --json number,isDraft,headRefOid,url,state \
    --jq '.[:1][] | "\(.number) \(.isDraft) \(.url)"'
}
merged_pr() { # merged_pr <branch> <sha>: "<number> <url>" of the merged pull request whose head is <sha>
  gh pr list --head "$1" --state merged --json number,headRefOid,url,state \
    --jq "[.[] | select(.headRefOid == \"$2\")][:1][] | \"\(.number) \(.url)\""
}

cmd_status() {
  local slug=$1 branch gdir ref name ahead behind counts stale dirty state left=0 local_head remote_head pr
  local number draft url
  valid slug "$slug"
  need_goal "$slug"
  branch=$(goal_branch "$slug"); gdir=$(goal_dir "$slug")
  local_head=$(g rev-parse "$branch")
  stale=""
  g fetch --quiet "$remote" "$branch" "$base" 2>/dev/null ||
    stale=", as of the last fetch: fetching from $remote failed"
  counts=$(g rev-list --left-right --count "$remote/$base...$branch")
  read -r behind ahead <<<"$counts"
  printf 'goal     %s  (%s ahead of %s/%s, %s behind%s)\n' "$branch" "$ahead" "$remote" "$base" "$behind" "$stale"
  if [ "$behind" -gt 0 ]; then
    printf 'behind   %s/%s has moved on: scripts/goal.sh sync %s brings %s in\n' "$remote" "$base" "$slug" "$base"
  fi
  printf 'worktree %s%s\n' "$gdir" "$(clean "$gdir" || printf '  (uncommitted changes)')"
  remote_head=$(g rev-parse --verify --quiet "$remote/$branch" || true)
  if [ "$local_head" = "$remote_head" ]; then
    printf 'pushed   yes\n'
  else
    printf 'pushed   no: the goal has commits not on %s\n' "$remote"; left=1
  fi
  if ! pr=$(open_pr "$branch" 2>/dev/null); then
    pr="unknown: could not list $branch's pull requests"
  elif [ -z "$pr" ]; then
    pr="none open for $branch"
  else
    read -r number draft url <<<"$pr"
    pr="$url  OPEN$([ "$draft" != true ] || printf ' draft')"
  fi
  printf 'pr       %s\n' "$pr"
  printf 'slices\n'
  while read -r ref; do
    [ -n "$ref" ] || continue
    name=${ref#refs/heads/slice/"$slug"/}
    ahead=$(g rev-list --count "$branch..${ref#refs/heads/}")
    dirty=""
    if [ -d "$(slice_dir "$slug" "$name")" ]; then
      clean "$(slice_dir "$slug" "$name")" || dirty=", uncommitted changes"
    else
      dirty=", no worktree"
    fi
    if [ "$ahead" = 0 ] && [ -z "$dirty" ]; then state="nothing to merge yet"; else state="$ahead commit(s) to merge$dirty"; fi
    printf '  %-20s %s\n' "$name" "$state"
    left=1
  done < <(g for-each-ref --format='%(refname)' "refs/heads/slice/$slug/")
  if [ "$left" = 0 ]; then
    printf 'left     nothing: ready to land (scripts/goal.sh land %s)\n' "$slug"
  else
    printf 'left     merge or drop every slice, push, then land\n'
  fi
}

# Wait for the CI run GitHub starts when the pull request is marked ready, on exactly this commit.
# Draft pull requests run nothing (ci.yml skips every job), so a run whose jobs were all skipped is
# a draft run and does not count. Returns 1, having said why, when the run fails or never comes.
wait_ci() { # wait_ci <sha> <since>
  local sha=$1 since=$2 deadline runs id status conclusion real
  deadline=$(( $(date +%s) + ci_timeout ))
  while :; do
    runs=$(gh run list --workflow ci.yml --commit "$sha" --event pull_request --limit 20 \
      --json databaseId,status,conclusion,createdAt \
      --jq "[.[] | select(.createdAt >= \"$since\")] | sort_by(.databaseId) | reverse | .[] | \"\(.databaseId) \(.status) \(.conclusion)\"") ||
      { say "gh run list failed; trying again"; runs=""; }
    while read -r id status conclusion; do
      [ -n "$id" ] || continue
      if [ "$status" != completed ]; then
        break
      fi
      real=$(gh run view "$id" --json jobs --jq '[.jobs[] | select(.conclusion != "skipped")] | length') ||
        { say "gh run view $id failed; trying again"; break; }
      [ "$real" -gt 0 ] || continue
      if [ "$conclusion" = success ]; then
        say "CI run $id passed on $sha"
        return 0
      fi
      say "CI run $id on $sha ended '$conclusion'"
      return 1
    done <<<"$runs"
    if [ "$(date +%s)" -ge "$deadline" ]; then
      say "no passing CI run on $sha after ${ci_timeout}s"
      # A run still going would keep spending runner time on a goal that is going back to draft.
      while read -r id status conclusion; do
        [ -z "$id" ] || [ "$status" = completed ] || gh run cancel "$id" >&2 || true
      done <<<"$runs"
      return 1
    fi
    sleep "$poll"
  done
}

# A goal that failed to land goes back to draft, so the pushes that fix it run no checks.
not_landed() { # not_landed <number> <why>
  gh pr ready "$1" --undo >&2 || true
  die "$2. Pull request #$1 is a draft again: fix it on the goal branch and land again"
}

cmd_land() {
  local slug=$1 branch gdir head since slices number merged draft url state rollup_bad ci_name pr
  valid slug "$slug"
  branch=$(goal_branch "$slug"); gdir=$(goal_dir "$slug")
  has_branch "$branch" || [ -d "$gdir" ] || on_remote "$branch" ||
    die "no goal '$slug' (start it with: scripts/goal.sh start $slug)"
  [ ! -d "$gdir" ] || clean "$gdir" || die "the goal worktree $gdir has uncommitted changes"
  lock "$slug"
  slices=$(g for-each-ref --format='%(refname:short)' "refs/heads/slice/$slug/")
  [ -z "$slices" ] || die "slices are still open, merge or drop them first:
$slices"
  # The goal's head: the local branch, or what an interrupted land left of it on the remote.
  if has_branch "$branch"; then
    head=$(g rev-parse "$branch")
  else
    head=$(g ls-remote "$remote" "refs/heads/$branch" | cut -f1)
  fi
  [ -n "$head" ] || die "goal '$slug' has no branch here or on $remote"
  # A land interrupted after GitHub merged only has its cleanup left. The merged pull request has to
  # have the goal's own head: one from an earlier goal with this slug has another.
  merged=$(merged_pr "$branch" "$head") || die "could not list the pull requests for $branch"
  if [ -n "$merged" ]; then
    read -r number url <<<"$merged"
    say "pull request #$number already merged $branch at $(g rev-parse --short "$head"): finishing the cleanup"
  else
    need_goal "$slug"
    # Again under the lock: what changed while land waited would be checked but not pushed.
    goal_worktree_ready "$slug" land
    removable "$gdir" "land again"
    checks_cmd "$slug" >/dev/null
    pr=$(open_pr "$branch") || die "could not list the pull requests for $branch"
    [ -n "$pr" ] || die "no open pull request for $branch (a merged or closed one from an earlier goal doesn't count)"
    read -r number draft url <<<"$pr"
    # The checks have to pass on what lands, so bring in whatever reached the base meanwhile.
    g fetch --quiet "$remote" "$base"
    if ! git -C "$gdir" merge-base --is-ancestor "$remote/$base" HEAD; then
      say "merging $remote/$base into the goal"
      git -C "$gdir" merge --quiet --no-ff -m "chore: merge $base into goal $slug" \
        -m "Goal $slug: brings in what reached $base, so the checks run on what lands." "$remote/$base" ||
        die "$base conflicts with the goal. Resolve it in $gdir and commit, then land again"
    fi
    say "full checks on $(git -C "$gdir" rev-parse --short HEAD)"
    checks "$slug" -C "$gdir" --full || die "the full checks failed: fix them on the goal branch and land again"
    push_goal "$slug"
    head=$(git -C "$gdir" rev-parse HEAD)
    # A push to the draft starts a run that skips everything. Let it be created first, so the run
    # marking the pull request ready starts can be told apart by its start time.
    sleep "$settle"
    since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    if [ "$draft" = true ]; then
      gh pr ready "$number" >&2
    elif [ "$draft" = false ]; then
      # Already ready: nothing new will start, so the run for this commit is whichever exists.
      since=1970-01-01T00:00:00Z
    else
      die "could not tell whether pull request #$number is a draft (isDraft: '$draft')"
    fi
    say "waiting for GitHub's checks on pull request #$number"
    wait_ci "$head" "$since" || not_landed "$number" "GitHub's checks did not pass"
    # Every other check on the head: a check run has a name and a conclusion, a commit status a
    # context and a state, and anything not finished and green blocks the merge. ci.yml's own jobs
    # are left out: wait_ci judged exactly the run this land started, and an earlier run's failed
    # jobs on the same commit stay in the rollup and would block a retried land for good. None of
    # these fields is a boolean, so // falls through only on a missing one.
    ci_name=""
    if [ -f "$gdir/.github/workflows/ci.yml" ]; then
      ci_name=$(sed -n 's/^name: *//p' "$gdir/.github/workflows/ci.yml" | head -n1 | tr -d "\"'")
    fi
    rollup_bad=$(gh pr view "$number" --json headRefOid,statusCheckRollup --jq \
      "if .headRefOid != \"$head\" then \"the pull request's head moved\" else
       ([.statusCheckRollup[] | select(\"$ci_name\" == \"\" or .workflowName != \"$ci_name\") |
         select((.conclusion // .state // \"PENDING\") as \$c |
         [\"SUCCESS\",\"SKIPPED\",\"NEUTRAL\"] | index(\$c) | not) |
         (.name // .context // \"a check with no name\")] | join(\", \")) end") ||
      not_landed "$number" "could not read the checks"
    [ -z "$rollup_bad" ] || not_landed "$number" "not every check passed on #$number: $rollup_bad"
    gh pr merge "$number" --merge --match-head-commit "$head" >&2 || not_landed "$number" "GitHub refused the merge"
    state=$(gh pr view "$number" --json state --jq .state) || state=unknown
    [ "$state" = MERGED ] || not_landed "$number" "pull request #$number is $state, not merged"
  fi
  # Each step skips what an interrupted land already did. The local branch goes last: while it is
  # there, a land run again finds the goal's head without the remote.
  [ ! -d "$gdir" ] || removable "$gdir" "land again to finish the cleanup"
  [ ! -d "$gdir" ] || g worktree remove "$gdir"
  g worktree prune
  # GitHub deletes a merged branch only when the repository is set to; don't rely on it.
  if on_remote "$branch"; then
    g push --quiet --no-verify "$remote" --delete "$branch" || true
  fi
  ! has_branch "$branch" || g branch --quiet -D "$branch"
  unlock "$slug"
  rmdir "$root/$slug" 2>/dev/null || true
  rmdir "$root" 2>/dev/null || true
  g fetch --quiet --prune "$remote"
  # Bring the primary checkout up to date when it sits on the base with nothing uncommitted.
  if [ "$(g rev-parse --abbrev-ref HEAD)" = "$base" ] && clean "$primary"; then
    g merge --quiet --ff-only "$remote/$base" || true
  fi
  no_entire_refs
  say "goal $slug landed on $base: pull request #$number"
}

cmd_path() {
  valid slug "$1"
  if [ $# -gt 1 ]; then valid slice "$2"; slice_dir "$1" "$2"; else goal_dir "$1"; fi
  echo
}

main() {
  [ $# -ge 2 ] || usage
  local cmd=$1; shift
  case "$cmd" in
    start) [ $# -le 2 ] || usage; cmd_start "$@" ;;
    slice) [ $# -eq 2 ] || usage; cmd_slice "$@" ;;
    merge) [ $# -eq 2 ] || usage; cmd_merge "$@" ;;
    drop) [ $# -eq 2 ] || { [ $# -eq 3 ] && [ "$3" = --force ]; } || usage; cmd_drop "$@" ;;
    sync) [ $# -eq 1 ] || usage; cmd_sync "$@" ;;
    status) [ $# -eq 1 ] || usage; cmd_status "$@" ;;
    land) [ $# -eq 1 ] || usage; cmd_land "$@" ;;
    path) [ $# -le 2 ] || usage; cmd_path "$@" ;;
    *) usage ;;
  esac
}

# Bash reads a script as it runs it. With every line above a definition, the file is read whole
# before main runs, so a merge that rewrites this file mid-run can't change what runs.
main "$@"; exit
