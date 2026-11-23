---
status: accepted
date: 2026-11-23
---

# ctdev doesn't set up Claude Code in a repo or connect it to Linear

ctdev no longer has `ctdev repo setup`, `ctdev configure linear` or the hidden `ctdev linear`
commands. Thomas decided on 2026-11-23 in CTD-169 to remove them. This supersedes ADR-0008
(Linear app credentials per computer) and ADR-0011 (Claude Code setup is local in every repo).

A repo's Claude Code reaches Linear through the `linear` server in its `.mcp.json`, signed in
with Linear's own OAuth (`claude mcp login linear`). ctdev stores no Linear credentials.

## Consequences

- Sessions act in Linear as the person who signed in, not as a per-computer app, so their
  comments and status changes no longer notify that person the way a teammate's would.
- A repo set up by an earlier ctdev keeps a local-scope `linear` server in `~/.claude.json` whose
  `headersHelper` runs `ctdev linear mcp-headers`, which now fails. Remove it with
  `claude mcp remove linear --scope local` in that repo. The credentials in
  `~/.config/ctdev/linear/` and the token cache in `~/.cache/ctdev/linear/` can be deleted.
- The `connertechnology` plugin is installed with Claude Code's own `claude plugin` commands.
