# Repo instructions only

Sessions read this repo's `AGENTS.md` (through `CLAUDE.md`, ADR-0002) and nothing from the user
level. `claudeMdExcludes` skips the user's `~/.claude/CLAUDE.md`, so how work is done here is
written down here and doesn't shift when someone's personal setup changes.

## Consequences

The **Project instructions** switch in `/config` would be the obvious tool, but Claude Code ignores
it in project and local settings, so `claudeMdExcludes` is the only lever a repo has. Patterns match
absolute paths, and the docs define symlink matching only for rules files. The template excludes
`**/.claude/CLAUDE.md`; when the user file is a symlink, `setup-skills` adds its resolved target as
well. The same pattern would skip a project-level `.claude/CLAUDE.md`, so instructions stay in the
root `AGENTS.md`.

Anything a global instruction used to enforce needs a home in the repo. Commit and PR attribution
live in `attribution` in `.claude/settings.json`, because a setting enforces it.

## In ctdev

`claudeMdExcludes` also skips the parent `ConnerTechnology/CLAUDE.md`, which loads for every repo
under that directory. What ctdev needed from it (no real family or client data, never invent a fact)
is in `AGENTS.md`'s Guardrails.
