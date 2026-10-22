# Where things go

Every fact in this repo has one home, picked by who needs it and when. The code is the source of
truth (ADR-0010): a home that restates what the code does is a copy that goes stale. When a fact
could live in two places, it goes in the one nearer the code, and the other points at it.

The Anthropic pages each row rests on are linked in [Sources](#sources). Where Anthropic says
nothing, the row says so.

## The table

| Home | What goes there | Rests on |
| --- | --- | --- |
| Code comment | The why, next to the line that acts on it: a trap, a vendor quirk, a number that would otherwise look arbitrary. The how is the code itself. | [7]; no Anthropic page says when a comment beats a doc |
| Commit body | A line or two under the subject: the Linear ID and why the change was made. | [2], [3], [9] |
| Pull request | What changed across the commits, how it was checked, and what the reviewer should look at. | Anthropic says nothing about what a PR body holds; [7] lists PR conventions as something CLAUDE.md can carry |
| Linear issue | The work: what was asked, the plan, acceptance, the QA walk, and the discussion. See `issue-tracker.md`. | [4] (reach the tracker through MCP); Anthropic says nothing about what an issue holds |
| ADR (`docs/adr/`) | A decision that is hard to reverse, surprising without context, and a real trade-off, product or setup alike. Status and date in the header (`docs/adr/template.md`); a reversed one is marked superseded, never deleted. | Anthropic says nothing; Nygard's practice [8] |
| Rule (`.claude/rules/`) | What no single file can say about one area: a convention across files, a trap, an outside fact. Path-scoped, so it drops out at `/compact` until a matching file is touched again. | [1], [5] |
| Skill (`.claude/skills/`) | A procedure, or reference needed only sometimes. | [1], [6] |
| `AGENTS.md` | Orientation and the conventions that bind everywhere. Loaded every session, so held to a byte ceiling. | [1], [7] |
| `CONTEXT.md` | The vocabulary: one term, its meaning, and the words to avoid. Nothing about implementation. | `/domain-modeling` |
| `docs/` | The human-facing manual, one page per capability, plus `TROUBLESHOOTING.md` at the root. | [7] (link to docs, don't inline them) |
| `CHANGELOG.md` | What shipped in each version, for the people who run it. | Anthropic says nothing |

A fact that changes often, or that a session can find by reading one file or running one command,
gets no home of its own: it is already in the code, the config, or `--help`.

## Tracing a line back to its reason

1. `git log -L <start>,<end>:<file>` (or `git blame`) names the commit that wrote the line.
2. The commit body names the Linear ID and the why. Older commits may not have a body.
3. `gh pr list --search <sha> --state merged` finds the pull request. Its branch name or body
   usually names the Linear ID, but some PRs name none, and there the trail ends. That gap is why
   commit bodies now cite the ID.
4. `get_issue` with that ID, then `list_comments`, gives the ask, the plan and the discussion.

If the reason is a lasting decision rather than one change, it is an ADR, and the issue links it.

## Running the prompt audit

`/doctor prompt-audit` runs on the bundled `/claude-api` skill [1], and this repo's
`.claude/settings.json` sets `disableBundledSkills`, so in a plain session here it refuses. A key
passed with `--settings` overrides the same key in the project's settings for that session only
[10], [11], so start the session in the repo with the skills turned back on:

```sh
claude --settings '{"disableBundledSkills":false}'
```

Then run `/doctor prompt-audit`. A session started outside the repo would load the audit too, but
it would also load the user's `~/.claude/CLAUDE.md`, which ADR-0007 keeps out of sessions here.

## Sources

Retrieved 2026-10-18.

1. [Claude Code: How Claude remembers your project](https://code.claude.com/docs/en/memory): what CLAUDE.md and AGENTS.md should hold, path-scoped rules, and `/doctor prompt-audit`.
2. [Claude prompting best practices: state management](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices#state-management-best-practices): "Use git for state tracking".
3. [Effective harnesses for long-running agents](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents) (2025-11-26): commit progress with descriptive messages; a new session reads the git log.
4. [Claude Code best practices: connect MCP servers](https://code.claude.com/docs/en/best-practices#connect-mcp-servers): MCP lets Claude work from an issue tracker.
5. [Claude Code: Context window, what survives compaction](https://code.claude.com/docs/en/context-window#what-survives-compaction): path-scoped rules are summarized away.
6. [Claude Code: Skills](https://code.claude.com/docs/en/skills): a skill for a procedure, or a CLAUDE.md section that has grown into one.
7. [Claude Code best practices: write an effective CLAUDE.md](https://code.claude.com/docs/en/best-practices#write-an-effective-claude-md): leave out "Anything Claude can figure out by reading code", and link to detailed docs rather than inline them.
8. [Michael Nygard, Documenting Architecture Decisions](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions) (2011): not Anthropic; short records with a status, kept in the repo.
9. [Claude Code best practices: provide specific context](https://code.claude.com/docs/en/best-practices#provide-specific-context-in-your-prompts): to learn why code looks the way it does, have Claude read its git history.
10. [Claude Code: CLI reference](https://code.claude.com/docs/en/cli-reference#cli-flags): `--settings` overrides the same keys in your `settings.json` files for that session.
11. [Claude Code: Settings precedence](https://code.claude.com/docs/en/settings#settings-precedence): the command line sits above every settings file except managed settings.
