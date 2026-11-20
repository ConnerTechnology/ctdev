# The goal loop

A goal is one change the human asked for. The session takes it to `main` on its own and asks only
about architecture (AGENTS.md, "Who decides what"). `scripts/goal.sh` runs the loop, and
`scripts/goal-test.sh` tests it against a throwaway repo. ADR-0017 records why it is shaped this way.

## The shape

- **One goal branch and one pull request.** `goal/<slug>`, cut from `origin/main`, pushed with a
  draft pull request. Start the slug with the Linear ID (`ctd-158-goal-loop`) so the issue links.
- **Slices.** `slice/<slug>/<name>`, each in its own worktree, cut from the goal branch's head.
  A slice is a local branch: it is merged into the goal branch on this machine and never pushed.
  A slice that needs another slice's work starts after that one merges.
- **One place for worktrees.** `../.worktrees/<repo>/<slug>/`, beside the primary checkout: the
  goal in `goal/`, each slice in `<name>/`. The script removes each one when it merges or lands.
- **Checks run here, not on GitHub.** `ci.yml` has no `pull_request` trigger, so the goal's pushes
  run nothing there. `land` runs the full checks on this machine and merges as soon as they pass.
  GitHub runs `ci.yml` on each push to `main` and each release tag, for the macOS job only it can
  run; `build-release` needs every job, so a macOS failure blocks the release and is fixed forward.

## The commands

Run them from the primary checkout, which stays on `main`. Running the copy inside the goal
worktree would let a merge rewrite the script while it runs. The commands that change a goal
(`slice`, `merge`, `drop`, `sync`, `land`) take its lock (`flock`), so they run one at a time on
each goal: one started while another holds it says so and waits, then looks again at what it left.

| Command | What it does |
| --- | --- |
| `start <slug> [title]` | Goal branch and worktree, an empty first commit (GitHub opens no pull request without one), push, draft pull request. A start that fails anywhere removes what it made, so it can run again |
| `slice <slug> <name>` | Slice branch and worktree from the goal's head; prints the path. Waits for a merge into the goal that is running, so it starts from what that merge leaves |
| `merge <slug> <name>` | Merges the slice into the goal worktree, runs `scripts/checks.sh --changed` on what the push adds, pushes the goal, removes the slice. Failing checks undo the merge and keep the slice; a commit made on the slice during the checks keeps it too. A slice (or goal) with an ignored file outside what Entire and the build leave is never removed: `git worktree remove` would delete it unseen |
| `drop <slug> <name> [--force]` | Removes a slice with nothing left to merge. `--force` discards its unmerged commits too; a slice with uncommitted changes is refused either way |
| `sync <slug>` | Merges `origin/main` into the goal, runs `scripts/checks.sh --changed` on what that brings in, pushes the goal. Failing checks undo the merge. Use it when `status` says the goal is behind, so slices cut next start from today's `main` |
| `status <slug>` | The goal's commits ahead of and behind `origin/main` (behind points at `sync`; a failed fetch says the counts are from the last one), whether it is pushed, its open pull request, each open slice, and what is left |
| `land <slug>` | Merges a moved `main` in, runs `scripts/checks.sh --full`, pushes, marks a draft pull request ready (GitHub won't merge a draft), merges it with `--match-head-commit` so only the checked commit lands, deletes the goal branch here and on GitHub, removes its worktree, fast-forwards the primary checkout. Failing checks touch no pull request. If GitHub refuses the merge, the pull request stays open and the goal stays as it was: fix what it says and land again. If it is interrupted after GitHub merged, running it again only finishes the cleanup |
| `path <slug> [name]` | Prints a worktree's path |

A conflict stops `merge`, `sync` or `land` with the merge left in the goal worktree. The session
resolves it there, commits, and runs the same command again.

A slug can be used again once its goal has landed. The script finds the goal's pull request by its
head branch and state, never `gh pr view <branch>`, which can answer with the earlier goal's merged
one: the open pull request is the goal's, and a merged one counts only when its head is the goal's
head.

## Who builds a slice

The session builds a slice itself or hands it to a subagent. A subagent gets the slice's worktree
path and works only there: it commits on the slice branch and never pushes, merges, or touches
another worktree. Parallel agents never share a worktree. ctdev has no ports, databases or
containers that outlive a run; `scripts/checks.sh` starts its own containers per run, so parallel
checks don't collide.

Don't give a subagent the Agent tool's `isolation: worktree`. It makes its worktree in
`.claude/worktrees/`, cut from the primary checkout's `HEAD` rather than the goal's head, and the
script neither merges nor removes it.

`/implement-spec` comes from the shared skills fork and stays generic. Here its steps are these
commands: "create a branch and a draft PR" is `start`, an implementer's worktree is a `slice`, the
merger is `merge`, and "mark the PR ready" plus the cleanup is `land`.

## Checks

`scripts/checks.sh` runs every job in `ci.yml` but the macOS one: shellcheck, the guidance check,
the goal tests, and in containers the Go tests (with the restic `ci.yml` pins), vet, the
cross-compile, the tests of `scripts/sessions/` (the tool behind `scripts/sessions.sh`), and the
Ubuntu install jobs. The PowerShell job parses `install.ps1` with `pwsh`, or, where `pwsh` is not
installed, in the official PowerShell image. It never runs ctdev on the host, because ctdev
configures the machine it runs on. `--changed` runs only the jobs the files can break; `--full`
runs them all, and is what `land` runs before it merges. The first run
takes about 10 minutes, nearly all of it the backup tests; the Go build and test caches are shared
volumes, so a run whose Go code is unchanged takes about a minute.

## Before landing

A fresh-context reviewer reads the goal pull request's diff (`gh pr diff`) against the goal's Linear
issue, not the author's summary: `/code-review main` on the goal branch. Fix what it finds in a
slice, then land.

## Entire

Entire records a checkpoint (prompt, transcript, files touched) for each commit with agent work,
linked by an `Entire-Checkpoint` trailer. `entire checkpoint list`, `entire why` and
`scripts/sessions.sh search` read them on this machine (`entire search` sends data to Entire).
This repo is public, so checkpoints stay on this machine (ADR-0019), and the daily retro reads
them here:

- `.entire/settings.json` sets `push_sessions: false` and the `git-refs` backend for every clone and
  worktree. The settings file `entire enable --local` writes is not in a new worktree; this one is.
- `entire configure --checkpoint-push-policy manual` is set once per clone, as a second lock.
- `scripts/goal.sh` pushes with `--no-verify`, which skips Entire's pre-push hook (the only thing
  that pushes checkpoints), and fails after any push if `origin` has a `refs/entire/` ref or an
  `entire/` branch (the legacy backend's `entire/checkpoints/v1`), or if it can't tell.
- After the release push, `git ls-remote origin 'refs/entire/*' 'refs/heads/entire/*'` prints
  nothing, and `entire checkpoint list --outgoing` says `git push` sends none of them.

On a new clone: `ctdev install entire` (ADR-0018), run
`entire configure --checkpoint-push-policy manual`, then `entire enable --agent claude-code`.
The mirror, `entire/unmirrored/` branches and Trails are not used (ADR-0017).
