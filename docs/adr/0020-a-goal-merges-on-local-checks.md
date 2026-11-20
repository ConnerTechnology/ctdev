---
status: accepted
date: 2026-11-20
---

# A goal merges when the local checks pass, without waiting for GitHub

`scripts/goal.sh land` runs the full checks on this machine (`scripts/checks.sh --full`: every
Linux job in containers, and the PowerShell parse in a container), pushes, and merges the goal's
pull request with `--match-head-commit`. GitHub runs no CI on pull requests. It still runs `ci.yml`
on every push to `main` and on each release tag, which is where the macOS job runs, the one job
this machine can't. The release build needs every test job, so a macOS failure stops the release
and is fixed forward on `main`. Decided on CTD-162 by the human: "We should not be waiting on
GitHub to merge a PR." This replaces ADR-0017's "GitHub's checks run once, when the goal is
landed"; the rest of ADR-0017 stands.

Measured on CTD-162: the full local checks take about 11 minutes, nearly all of it the Go tests
with the restic backup tests. Waiting for GitHub's run added about 24 minutes to every land.

## Considered options

- **Wait for GitHub's run before merging (ADR-0017).** It catches a macOS-only failure before
  `main` instead of after, at the cost of 24 minutes per goal. Those failures are rare, and the
  release still can't ship one.
- **Post each local result as a commit status, as Oslados does.** Nothing on `main` requires a
  status (ADR-0017 rejected a ruleset), so the statuses would only be decoration.
