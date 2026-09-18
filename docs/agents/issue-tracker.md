# Issue tracker: Linear

Issues, specs and design docs for this repo live in **Linear**, team **Conner Technology** (key
`CON`). Use the Linear MCP tools (`mcp__linear__*`) for every operation; GitHub Issues are not used.

## Conventions

- **Every issue from this repo carries the `ctdev` label** and one kind: `Bug`, `Feature` or
  `Improvement`. The team is shared with other work, so the label is how this repo's issues are found.
- **Create an issue**: `save_issue` with `team: "Conner Technology"`, the `project`, the labels
  above, and `blockedBy` / `blocks` for the edges. Titles say the outcome, not the task.
- **Read an issue**: `get_issue` with relations, then `list_comments` for the history.
- **List issues**: `list_issues` filtered by `label: "ctdev"` plus `project` or `state`.
- **States**: Backlog → Todo → In Progress → In Review (the pull request is open) → **Done**
  (merged to `main`; comment the merge commit hash). Canceled and Duplicate close without work.
- **`/implement` and builders never close a ticket.** The coordinator moves it to Done after
  reviewing the diff and the evidence, and Thomas merges.
- **A dispatched ticket is reviewed by the coordinator before merge.** A builder is a sub-agent
  and cannot spawn the two sub-agents `/code-review` needs. It follows `/implement` minus the
  review and commits on its worktree branch; the coordinator runs `/code-review main` on that
  branch in a fresh context, with the ticket as the spec. When Thomas types `/implement` himself,
  the skill's own order (review before commit) holds.
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

`get_issue` with the `CON-123` identifier, then `list_comments`.

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
