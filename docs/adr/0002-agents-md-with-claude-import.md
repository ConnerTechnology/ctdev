# AGENTS.md, imported by CLAUDE.md

A repo's instructions live in `AGENTS.md`, and `CLAUDE.md` is the single line `@AGENTS.md`. Codex,
Cursor, and Copilot read `AGENTS.md` directly. Claude Code reads it on its own only when no
`CLAUDE.md` exists, and not in every session, so the import makes sure Claude reads it every time.
`setup-skills` writes to `AGENTS.md`.

## Considered options

A symlink from `CLAUDE.md` to `AGENTS.md` was rejected: Claude's Edit and Write tools refuse to write
through a symlink, and a committed symlink checks out as a one-line text file on Windows.
