---
status: accepted
date: 2026-10-22
---

# Claude Code setup is local in every repo, and ctdev does only the deterministic part

Nothing ctdev does to set up Claude Code in a repo lands in a file the repo commits, in our repos
or anyone else's. `ctdev repo setup` installs git, gh and Claude Code, adds the `connertechnology`
marketplace and installs its plugin at local scope (the repo's `.claude/settings.local.json`, kept
out of git through `.git/info/exclude`), then starts Claude Code on `/connertechnology:setup`.
`ctdev configure linear` and `ctdev configure jira` add their MCP servers with
`claude mcp add-json --scope local`. Decided with Thomas on 2026-10-22 in CTD-109, alongside
CON-146, so the engineering library works in a repo Thomas doesn't own (a job on Jira), and works
the same when others contribute to ours: each contributor runs `ctdev repo setup` themselves.

ctdev does only what has one right answer: a tool is installed or not, a credential works or not, a
server is there or not. Which skills, plugins, MCP servers and settings a repo uses are
preferences. They live in the engineering repo, and its setup skill writes them, so ctdev has no
per-repo Claude settings and no `ctdev repo diff`.

## Considered options

- **A `--personal` flag for repos we don't own, committed setup in ours.** Two paths to keep
  working, and a committed `enabledPlugins` doesn't install the plugin on another machine anyway.
- **The marketplace in `~/.claude/settings.json`**, its default scope. That file is ctdev's
  baseline (ADR-0009), and the drift review would offer to remove the marketplace.

## Consequences

The plugin is pinned to a tag in ctdev's code, and a third-party marketplace doesn't auto-update,
so the library moves to a new version only when a ctdev release moves the pin and
`ctdev repo setup` runs again. The marketplace clone uses gh's github.com login for that one
command, through `GIT_CONFIG_*` variables, without touching `~/.gitconfig`.

Claude Code keeps one clone per marketplace name on a machine, and adding it again with a new
source repoints that clone for every repo, while each repo's `.claude/settings.local.json` still
records the source it was set up with. After a pin moves, run `ctdev repo setup` again in each
repo, so none still names the old tag.
