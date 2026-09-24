# Issue tracker: Linear

Issues, specs and design docs for this repo live in **Linear**, team **CTDev** (key `CTD`), a
sub-team of Conner Technology. Use the Linear MCP tools (`mcp__linear__*`) for every operation;
GitHub Issues are not used. Issues moved from `CON` on 2026-09-24; their old IDs still resolve.

Sessions reach Linear as the **Claude Code app**, not as Thomas: `.mcp.json` gives the `linear`
server a `headersHelper`, `scripts/linear-app.sh --mcp-headers`, which trades the app's client ID
and secret from `~/.secrets` for a token. The app's comments and status changes notify Thomas the
way a teammate's would. The app sees only teams it can reach. `scripts/linear-app.sh graphql`
sends one request as the app, for what the MCP has no tool for.

## Conventions

- **Every ticket that changes the repo carries one kind label**: `bug` or `enhancement`. Wayfinder
  and other planning tickets carry their `wayfinder:*` label instead.
- **Create an issue**: `save_issue` with `team: "CTDev"`, the `project`, the kind label, and
  `blockedBy` / `blocks` for the edges. Titles say the outcome, not the task.
- **Read an issue**: `get_issue` with relations, then `list_comments` for the history.
- **List issues**: `list_issues` with `team: "CTDev"`, plus `project`, `state` or `label`.
- **States**: Backlog → Todo → In Progress → In Review (the pull request is open) → **Done**
  (merged to `main`; comment the merge commit hash). Canceled and Duplicate close without work.
- **`/implement` and builders never close a ticket.** The coordinator moves it to Done after
  reviewing the diff and the evidence, and Thomas merges.
- **A dispatched ticket is reviewed by the coordinator before merge.** A builder doesn't review
  its own work, so the review judges the diff and the evidence rather than the builder's report.
  The builder follows `/implement` minus the review and commits on its worktree branch; the
  coordinator runs `/code-review main` on that branch in a fresh context, with the ticket as the
  spec. When Thomas types `/implement` himself, the skill's own order (review before commit) holds.
- **A builder runs unattended, so it's told not to stop early.** Its prompt ends with the
  instruction under "Keeping implementers working" in `.claude/skills/implement-spec/SKILL.md`.
  If its run ends with acceptance criteria still open and no blocker named, the coordinator sends
  it one message naming them, at most twice, and then raises it with Thomas.
- **A QA plan is a checklist in the ticket description.** One checkbox per thing to verify; under
  it the exact actions as numbered sub-steps, then a **See:** line. Thomas ticks the box when he
  saw it; an item that needs a reading kept says **Paste:** and he adds it as a comment. An
  unticked box with a note under it is a finding, and becomes a linked issue. The coordinator
  reads the boxes, not the chat, to know the walk is done.
- **Who takes it next** is a label: `ready-for-agent` or `ready-for-human` (see `triage-labels.md`).
- **Branch names**: the one Linear gives the issue (`gitBranchName`), so the GitHub integration
  can move it.
- Text is American English, written like a person wrote it.

## Pull requests as a triage surface

**PRs as a request surface: no.** Pull requests here are opened by the coordinator for its own
branches. The repo is public, but outside contributions are not solicited.

## When a skill says "publish to the issue tracker"

- A **spec or design doc** is a **Linear document on the project** (`save_document`), not a file
  in the repo.
- A **ticket** is a Linear issue in that project, with native blocking links.

## When a skill says "fetch the relevant ticket"

`get_issue` with the `CTD-123` identifier, then `list_comments`.

## Wayfinding operations

Used by `/wayfinder`. The **map** is a single issue with **child** issues as tickets.

- **Map**: one issue holding the Notes / Decisions-so-far / Fog body, titled `Map: <destination>`.
- **Child ticket**: a sub-issue of the map (`parentId`), in the same project.
- **Blocking**: Linear's native relations — `blockedBy` / `blocks` on `save_issue`. A ticket is
  unblocked when every blocker is Done or Canceled.
- **Frontier query**: `list_issues` with `parentId` set to the map, open states only; drop any with
  an open blocker or an assignee; first in map order wins.
- **Claim**: `save_issue` with `assignee: "me"` and `state: "In Progress"` — the session's first write.
- **Resolve**: `save_comment` with the answer, move the ticket to Done, then append the decision to
  the map's Decisions-so-far.
