---
status: accepted
date: 2026-11-19
---

# A goal lands through one pull request, built in local slice worktrees

The human gives a goal and the session takes it to `main`, asking only about architecture. Each goal
gets one goal branch and one pull request. The work is split into slices, local branches in their own
worktrees that merge into the goal branch on this machine. GitHub's checks run once, when the goal is
landed. `scripts/goal.sh` runs the loop (`docs/agents/goal-loop.md`). This was decided on CTD-158,
which was the first goal built this way.

Measured on 2026-11-19, before deciding: a full GitHub run takes about 25 minutes. The Linux jobs
run locally in containers in about 35 seconds without the restic backup tests and about 10 minutes
with them. A fetch from GitHub takes about a second.

## Considered options

- **A pull request per slice.** Each would cost a GitHub round trip and a full run. Slices merge
  locally instead, and only the goal branch is pushed.
- **Checks only on GitHub, on every push.** Drafts now skip every job (`ci.yml`). Slices are checked
  locally on what they touched, and the goal is checked in full both locally and then once on GitHub.
  The Windows and macOS jobs can only run on GitHub.
- **A ruleset on `main` that requires the checks.** Rejected. AGENTS.md has the release commit
  pushed straight to `main`, so a ruleset would need an admin bypass. Every session runs as an
  admin, so the ruleset would add friction and enforce nothing. `land` refuses to merge unless the
  run that marking the pull request ready started has passed on that exact commit, and it merges with
  `--match-head-commit`.
- **Entire's GitHub mirror, `entire/unmirrored/` slice branches, and Trails.** Rejected. Slices are
  worktrees of one clone, so they never touch a remote, and a mirror would cut maybe a second off
  each goal push. It would also need a signup and a third-party GitHub App with write access to a
  public repo. Unmirrored branches aren't backed up. A Trail would repeat the goal pull request,
  and its Checks gate reads the same GitHub checks.
- **Pushing checkpoints to a separate private repo.** Not chosen for now. Checkpoints stay on this
  machine (`push_sessions: false`, a manual push policy, and a guard in `scripts/goal.sh`). A
  transcript can never reach this public repo that way, and nothing new has to be created. If
  sessions need to be shared across machines, a private checkpoint repo is the next step.

## Consequences

- Entire captures inside linked worktrees: its git hooks live in the shared `.git/hooks`, and its
  project settings are committed. That was tested in a sandbox, including a session in the primary
  checkout that commits into a slice worktree. `entire enable --local` alone would have left new
  worktrees unconfigured, and they would fall back to the legacy branch backend.
- The worktrees older sessions left in `.claude/worktrees/` were not touched. Three of them hold
  uncommitted changes and one holds a commit that was never pushed, so removing them is the human's
  decision.
