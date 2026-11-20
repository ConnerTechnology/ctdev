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
# Usage:
#   scripts/goal.sh start <slug> [title]   goal branch, its worktree, and a draft pull request
#   scripts/goal.sh slice <slug> <name>    a slice worktree, cut from the goal branch's head
#   scripts/goal.sh merge <slug> <name>    check the slice, merge it into the goal, push, clean up
#   scripts/goal.sh drop  <slug> <name>    remove a slice that has nothing left to merge
#   scripts/goal.sh status <slug>          the goal's slices, their state, and what is left
#   scripts/goal.sh land <slug>            full checks, then GitHub's checks, then merge and clean up
#   scripts/goal.sh path <slug> [name]     print a worktree's path
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
clean() { [ -z "$(git -C "$1" status --porcelain)" ]; }

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
  refs=$(g ls-remote "$remote" 'refs/entire/*' 'refs/heads/entire/*')
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
  g worktree add --quiet --no-track -b "$branch" "$dir" "$remote/$base"
  # GitHub opens no pull request for a branch with no commits beyond its base.
  git -C "$dir" commit --quiet --allow-empty -m "chore: start goal $slug" -m "Goal $slug: the first commit, so its pull request can open."
  # A goal without its pull request can't be landed or started again, so a start that fails after
  # the push takes the branch and worktree back down.
  if ! git -C "$dir" push --quiet --no-verify "$remote" "refs/heads/$branch:refs/heads/$branch" ||
     ! entire_refs_absent ||
     ! gh pr create --draft --base "$base" --head "$branch" --title "${title:-goal: $slug}" \
       --body "Goal \`$slug\`. Built in slices with \`scripts/goal.sh\`; the description is written before it lands." >&2; then
    say "start failed: removing goal $slug"
    g push --quiet --no-verify "$remote" --delete "$branch" 2>/dev/null || true
    g worktree remove --force "$dir"
    g branch --quiet -D "$branch"
    rmdir "$root/$slug" 2>/dev/null || true
    die "goal $slug was not started"
  fi
  say "goal $slug started in $dir"
  printf '%s\n' "$dir"
}

cmd_slice() {
  local slug=$1 name=$2 branch dir
  valid slug "$slug"; valid slice "$name"
  [ "$name" != goal ] || die "a slice can't be named 'goal': that is the goal's own worktree"
  need_goal "$slug"
  # A merge commits before its checks run, so a slice cut now could start from a merge they undo.
  [ ! -d "$root/$slug/.merge.lock" ] || die "a merge into goal '$slug' is running: cut the slice when it is done"
  branch=$(slice_branch "$slug" "$name"); dir=$(slice_dir "$slug" "$name")
  has_branch "$branch" && die "slice $branch already exists"
  [ -e "$dir" ] && die "$dir already exists"
  g worktree add --quiet -b "$branch" "$dir" "$(goal_branch "$slug")"
  say "slice $name of goal $slug at $(g rev-parse --short "$branch") in $dir"
  printf '%s\n' "$dir"
}

remove_slice() { # remove_slice <slug> <name>: the worktree, then the branch
  local dir branch
  dir=$(slice_dir "$1" "$2"); branch=$(slice_branch "$1" "$2")
  [ ! -d "$dir" ] || g worktree remove "$dir"
  has_branch "$branch" && g branch --quiet -D "$branch"
  g worktree prune
}

# One merge or land into a goal at a time: two would race on the goal worktree.
lock() {
  lock_dir="$root/$1/.merge.lock"
  mkdir "$lock_dir" 2>/dev/null || die "another merge into goal '$1' is running (or left $lock_dir behind)"
  trap unlock EXIT
}
unlock() { [ -z "${lock_dir:-}" ] || rmdir "$lock_dir" 2>/dev/null || true; lock_dir=""; }

cmd_merge() {
  local slug=$1 name=$2 branch dir gdir merged_now=0 files f
  valid slug "$slug"; valid slice "$name"
  need_goal "$slug"
  branch=$(slice_branch "$slug" "$name"); dir=$(slice_dir "$slug" "$name"); gdir=$(goal_dir "$slug")
  has_branch "$branch" || die "no slice '$name' in goal '$slug'"
  [ ! -d "$dir" ] || clean "$dir" || die "slice $name has uncommitted changes in $dir: commit or drop them"
  checks_cmd "$slug" >/dev/null
  lock "$slug"
  if [ -f "$(git -C "$gdir" rev-parse --git-path MERGE_HEAD)" ]; then
    die "the goal worktree has a merge in progress: resolve it in $gdir, commit, then run this again"
  fi
  clean "$gdir" || die "the goal worktree $gdir has uncommitted changes"
  if ! git -C "$gdir" merge-base --is-ancestor "$branch" HEAD; then
    say "merging slice $name into $(goal_branch "$slug")"
    if ! git -C "$gdir" merge --quiet --no-ff -m "chore: merge slice $name" \
      -m "Goal $slug: slice $name, checked and merged by scripts/goal.sh." "$branch"; then
      die "slice $name conflicts with the goal. Resolve it in $gdir and commit, then run this again"
    fi
    merged_now=1
  fi
  # What this push will add to the goal: the slice, plus any conflict resolution.
  g fetch --quiet "$remote" "$(goal_branch "$slug")"
  files=()
  # --no-renames lists a moved file under its old path too, so moving it out of ctdev/ still runs Go.
  while IFS= read -r f; do files+=("$f"); done < <(git -C "$gdir" diff --name-only --no-renames \
    "$remote/$(goal_branch "$slug")" HEAD)
  if [ ${#files[@]} -gt 0 ] && ! checks "$slug" -C "$gdir" --changed "${files[@]}"; then
    if [ "$merged_now" = 1 ]; then
      git -C "$gdir" reset --quiet --hard ORIG_HEAD
      die "checks failed, so the merge was undone. Fix slice $name in $dir and run this again"
    fi
    die "checks failed on the goal branch. Fix it in $gdir and run this again"
  fi
  push_goal "$slug"
  # The checks can take minutes; a commit made on the slice meanwhile is not in the goal, and
  # deleting the branch would lose it.
  git -C "$gdir" merge-base --is-ancestor "$branch" HEAD ||
    die "slice $name has commits made after it was merged: run this again to merge them"
  remove_slice "$slug" "$name"
  say "slice $name merged into $(goal_branch "$slug") and pushed; its worktree and branch are gone"
}

cmd_drop() {
  local slug=$1 name=$2 branch dir ahead
  valid slug "$slug"; valid slice "$name"
  branch=$(slice_branch "$slug" "$name"); dir=$(slice_dir "$slug" "$name")
  has_branch "$branch" || die "no slice '$name' in goal '$slug'"
  [ ! -d "$dir" ] || clean "$dir" || die "slice $name has uncommitted changes in $dir"
  ahead=$(g rev-list --count "$(goal_branch "$slug")..$branch")
  [ "$ahead" = 0 ] || die "slice $name has $ahead commit(s) not in the goal: merge it, or delete it by hand"
  remove_slice "$slug" "$name"
  say "slice $name dropped"
}

cmd_status() {
  local slug=$1 branch gdir ref name ahead dirty state left=0 local_head remote_head pr
  valid slug "$slug"
  need_goal "$slug"
  branch=$(goal_branch "$slug"); gdir=$(goal_dir "$slug")
  g fetch --quiet "$remote" "$branch" "$base" 2>/dev/null || true
  local_head=$(g rev-parse "$branch")
  remote_head=$(g rev-parse --verify --quiet "$remote/$branch" || true)
  printf 'goal     %s  (%s commits beyond %s/%s)\n' "$branch" \
    "$(g rev-list --count "$remote/$base..$branch")" "$remote" "$base"
  printf 'worktree %s%s\n' "$gdir" "$(clean "$gdir" || printf '  (uncommitted changes)')"
  if [ "$local_head" = "$remote_head" ]; then
    printf 'pushed   yes\n'
  else
    printf 'pushed   no: the goal has commits not on %s\n' "$remote"; left=1
  fi
  pr=$(gh pr view "$branch" --json url,isDraft,state --jq '"\(.url)  \(.state)\(if .isDraft then " draft" else "" end)"' 2>/dev/null || echo "none found")
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
  local slug=$1 branch gdir head since slices number state rollup_bad
  valid slug "$slug"
  need_goal "$slug"
  branch=$(goal_branch "$slug"); gdir=$(goal_dir "$slug")
  slices=$(g for-each-ref --format='%(refname:short)' "refs/heads/slice/$slug/")
  [ -z "$slices" ] || die "slices are still open, merge or drop them first:
$slices"
  clean "$gdir" || die "the goal worktree $gdir has uncommitted changes"
  checks_cmd "$slug" >/dev/null
  lock "$slug"
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
  number=$(gh pr view "$branch" --json number --jq .number) || die "no pull request for $branch"
  # A push to the draft starts a run that skips everything. Let it be created first, so the run
  # marking the pull request ready starts can be told apart by its start time.
  sleep "${GOAL_SETTLE:-10}"
  since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  if [ "$(gh pr view "$number" --json isDraft --jq .isDraft)" = true ]; then
    gh pr ready "$number" >&2
  else
    # Already ready: nothing new will start, so the run for this commit is whichever exists.
    since=1970-01-01T00:00:00Z
  fi
  say "waiting for GitHub's checks on pull request #$number"
  wait_ci "$head" "$since" || not_landed "$number" "GitHub's checks did not pass"
  # Every check on the head, not only ci.yml's: a check run has a name and a conclusion, a commit
  # status a context and a state, and anything not finished and green blocks the merge.
  rollup_bad=$(gh pr view "$number" --json headRefOid,statusCheckRollup --jq \
    "if .headRefOid != \"$head\" then \"the pull request's head moved\" else
     ([.statusCheckRollup[] | select((.conclusion // .state // \"PENDING\") as \$c |
       [\"SUCCESS\",\"SKIPPED\",\"NEUTRAL\"] | index(\$c) | not) |
       (.name // .context // \"a check with no name\")] | join(\", \")) end") ||
    not_landed "$number" "could not read the checks"
  [ -z "$rollup_bad" ] || not_landed "$number" "not every check passed on #$number: $rollup_bad"
  gh pr merge "$number" --merge --match-head-commit "$head" >&2
  state=$(gh pr view "$number" --json state --jq .state)
  [ "$state" = MERGED ] || die "pull request #$number is $state, not merged"
  # GitHub deletes a merged branch only when the repository is set to; don't rely on it.
  if [ -n "$(g ls-remote --heads "$remote" "$branch")" ]; then
    g push --quiet --no-verify "$remote" --delete "$branch" || true
  fi
  g worktree remove "$gdir"
  g branch --quiet -D "$branch"
  g worktree prune
  unlock
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
    drop) [ $# -eq 2 ] || usage; cmd_drop "$@" ;;
    status) [ $# -eq 1 ] || usage; cmd_status "$@" ;;
    land) [ $# -eq 1 ] || usage; cmd_land "$@" ;;
    path) [ $# -le 2 ] || usage; cmd_path "$@" ;;
    *) usage ;;
  esac
}

# Bash reads a script as it runs it. With every line above a definition, the file is read whole
# before main runs, so a merge that rewrites this file mid-run can't change what runs.
main "$@"; exit
