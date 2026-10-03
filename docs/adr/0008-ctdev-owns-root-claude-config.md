# ctdev owns the root Claude config

`~/.claude/settings.json` and `~/.claude/CLAUDE.md` are one baseline, kept in
`ctdev/component/configs/claude-code/` and identical on every computer. Customization for a
project lives in that project's own `.claude/settings.json` and `CLAUDE.md`. Nothing outside the
AI repo knows the AI repo exists: no environment variable, memory directory or permission rule at
the root points at it.

This reverses the earlier split in the AI repo ("ctdev owns the machine, that repo owns Claude"),
where the AI repo's setup merged its settings into the root file and linked the root `CLAUDE.md`
into itself. Two tools writing the same files left neither one in charge of what a session got.

## Consequences

Claude Code writes to the root file itself (`/model`, `/effort`, `/config`), so the file drifts.
Drift is shown, not silently reverted: after `ctdev install claude-code`, the configure step
prints a diff for each owned file that differs (or is a symlink) and asks before it backs the
file up with a dated name and replaces it. With no terminal the file is left alone and reported;
`--force` replaces without asking. A setting worth keeping goes into the baseline in this repo.

`~/.claude/settings.local.json` is backed up and removed on install: Claude Code reads
`settings.local.json` only inside a project, so one at the root looked real and did nothing.

The baseline's `statusLine` runs ccstatusline, so `claude-code` depends on `ccstatusline`.
