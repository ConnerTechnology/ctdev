# Issue tracker: Linear

Work for this repo is tracked in Linear, team **<TEAM NAME> (`<TEAM KEY>`)**, project
**<PROJECT NAME> (`<PROJECT ID>`)**. Use the `linear` MCP server's tools for every operation.

## Conventions

- **Create an issue**: `save_issue` with `team: "<TEAM KEY>"`, `project: "<PROJECT ID>"`, a
  `title`, and a Markdown `description`.
- **Read an issue**: `get_issue` with the identifier (e.g. `<TEAM KEY>-12`), then `list_comments`
  for its thread.
- **List issues**: `list_issues` with `project: "<PROJECT ID>"`, filtered by `state`, `label`, or
  `assignee` as needed.
- **Comment on an issue**: `save_comment` with `issueId` and `body`.
- **Apply / remove labels**: `save_issue` with `id` and `addLabels` / `removeLabels`.
- **Close**: `save_issue` with `id` and `state: "Done"` (or `"Canceled"`), after a closing comment.
- **Project status update**: `save_status_update` with `type: "project"` and `project:
  "<PROJECT ID>"`: what moved, what's blocked, what's next.

## Pull requests as a triage surface

**PRs as a request surface: no.** PRs aren't triaged here; review happens against the Linear issue.

## When a skill says "publish to the issue tracker"

Create a Linear issue in `<TEAM KEY>`, in project `<PROJECT ID>`.

## When a skill says "fetch the relevant ticket"

`get_issue` with its identifier, plus `list_comments`.

## Wayfinding operations

Used by `/wayfinder`. The **map** is a single issue with **child** issues as tickets.

- **Map**: an issue labelled `wayfinder:map`, holding the Notes / Decisions-so-far / Fog body.
- **Child ticket**: a sub-issue of the map (`save_issue` with `parentId: "<map identifier>"`),
  labelled `wayfinder:<type>` (`research`/`prototype`/`grilling`/`task`).
- **Blocking**: Linear's native relations, `save_issue` with `blockedBy: ["<identifier>", ...]`. A
  ticket is unblocked when every blocker is in a completed or canceled state.
- **Frontier query**: `list_issues` for the map's open children; drop any with an open blocker or an
  assignee; first in the map's order wins.
- **Claim**: `save_issue` with `assignee: "me"`, the session's first write.
- **Resolve**: `save_comment` with the answer, `save_issue` with `state: "Done"`, then append a
  context pointer (gist + link) to the map's Decisions-so-far.
