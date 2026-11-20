---
status: accepted
date: 2026-11-20
---

# ctdev doesn't use Entire, and there is no daily retro

ctdev no longer installs, updates or reports the Entire CLI, and this repo no longer records its
sessions with Entire or runs a retro on them. The human decided this on CTD-165. It replaces
ADR-0018 (ctdev owns Entire's updates) and ADR-0019 (a daily retro lands its own fixes).

What went with it: the `entire` component, its `ctdev update` scan and doctor row, Entire's agent
hooks and `.entire/` settings, `scripts/goal.sh`'s check for checkpoint refs on `origin`, the
sessions tool behind the retro, `/retro`, and `docs/retros/`.

## Consequences

- A machine that has Entire keeps it. ctdev stops updating it, and Entire's own update check stays
  off where the component turned it off (`no_auto_update` in Entire's `config.json`).
- A clone that ran `entire enable` still has Entire's git hooks in `.git/hooks`, and checkpoints
  under `refs/entire/`. `entire disable --uninstall` removes the hooks. The checkpoints are local
  refs: nothing pushes them, and deleting them is the human's call.
- ADR-0017's notes on Entire in linked worktrees and on keeping checkpoints local are history.
