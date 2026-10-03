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
Drift is shown, not silently reverted. The install writes a missing file and never touches one
that differs; after the progress screen, a review runs whenever the install (or `apply`) included
`claude-code` and it is installed, even if another component in the run failed (and as
`ctdev configure claude-code`). For each owned file that differs, or is a
symlink, it prints a diff and asks before it backs the file up with a dated name and replaces it.
A symlink is removed and the file it pointed to is left alone. With no terminal the file is left
and reported; `--force` replaces without asking; `--dry-run` shows the diff and writes nothing.
Every question comes before any write. A setting worth keeping goes into the baseline in this repo.

`~/.claude/settings.local.json` is backed up and removed by that review: Claude Code reads
`settings.local.json` only inside a project, so one at the root looked real and did nothing.

ccstatusline is part of the `claude-code` component, not a component of its own: it does nothing
without Claude Code, and the baseline's `statusLine` is what turns it on. It no longer edits
`~/.claude/settings.json` (which CTD-72 had it do), so the file has one writer.
