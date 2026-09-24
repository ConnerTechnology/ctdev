# Explicit tool allowlist

A repo loads only the skills, agents, and MCP servers that `.claude/settings.json` names, not
whatever happens to be installed on the machine. Skills are committed files under `.claude/skills/`
(ADR-0003), so what runs is pinned and reviewable in a diff.

## Consequences

MCP is default-deny: `allowedMcpServers` blocks any server that doesn't match an entry, including
ones added later, so only Linear gets through. Built-in servers are exempt from that allowlist, so
Claude in Chrome is blocked by name in `deniedMcpServers`, and a repo that reads web pages removes
that entry during setup.

**Plugins can't be default-denied from a repo.** `strictKnownMarketplaces` and
`blockedMarketplaces` are Managed scope only, and `enabledPlugins` is keyed by name, so a plugin
with no entry falls back to its own `defaultEnabled` and loads. `setup-skills` pins every plugin
installed at setup time, and `.claude/hooks/check-plugins` names any installed later at the next
session start. The gap is detected, not closed.

`skillOverrides` reaches personal and claude.ai-synced skills by bare name but not plugin skills,
which answer only to `enabledPlugins`. Every such skill gets an explicit value, `"on"` included, so
`check-plugins` can tell a kept skill from a new one.

## In ctdev

`allowedMcpServers` also lets `mcp.context7.com` through, and the `context7` plugin is on, so
sessions can read current documentation for the libraries ctdev uses or is weighing up. The
`gopls-lsp` plugin is on for Go code intelligence. Every other installed plugin is pinned off.
