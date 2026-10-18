---
status: accepted
date: 2026-10-18
---

# The code is the source of truth, and git is the trail to its reason

What ctdev does is what its code says. The reason for a line lives in a comment next to it when
the code acts on that reason, and otherwise in the trail git keeps: the commit body names the
Linear ticket and says why, the pull request collects the commits, and the ticket holds the ask
and the discussion. Docs and rules don't restate how the code works. A rule holds only what no
single file can say: a convention across files, a trap, an outside fact. ADRs record product
decisions as well as agent setup, each with a status and a date. `AGENTS.md` stays orientation.
`docs/agents/where-things-go.md` is the table of homes. Decided in CTD-105.

The rules had grown into manuals for their components, which repeated the code and drifted from
it, and a path-scoped rule drops out at `/compact` until a matching file is touched again, so a
fact kept only there goes missing mid-session. Anthropic's guidance says to leave out of
CLAUDE.md anything Claude can learn by reading the code, to use git for state, and to read git
history to learn why code looks the way it does. It says nothing about ADRs; keeping them with a
status, and marking a reversed one superseded rather than deleting it, is Michael Nygard's
practice. The sources are listed in `docs/agents/where-things-go.md`.

## Considered options

Keeping component manuals in the rules was the status quo. It costs nothing to write but loads a
copy of the code into context and has to be kept in step by hand. CTD-92 already went the other
way: it dropped `.claude/rules/aws.md` and moved each fact into a comment where the code acts on
it.

## Consequences

Commit bodies cite the Linear ID and the why. `/doctor prompt-audit` runs at each release, beside
`scripts/check-guidance.sh` in CI. CTD-106 trims the rules to what the code can't say, and
CTD-107 gives the existing ADRs a status and a date and writes ADRs for ctdev's main product
decisions. ADRs from here on use `docs/adr/template.md`.
