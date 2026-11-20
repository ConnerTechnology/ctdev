# Retros

What went wrong in our sessions and what changed so it doesn't happen again. Two kinds (ADR-0019):

| Kind | File | When | How it lands |
| --- | --- | --- | --- |
| Daily | `<YYYY-MM-DD>-daily.md` | Every day at 06:20 on the human's machine (T3 Code scheduled task, `/retro auto`), on the previous day's sessions | Nobody: it lands through `scripts/goal.sh land` when every check passes |
| With the human | `<YYYY-MM-DD>-<topic>.md` | When the human runs `/retro` at the end of a session | Through the goal loop, like any goal |

## The daily loop

1. **Capture.** Entire records each session as a checkpoint next to its commit, in
   `refs/entire/checkpoints/` of this clone, from any worktree. Checkpoints never leave this
   machine. Sessions that never commit aren't recorded.
2. **Digest.** `scripts/sessions.sh digest` turns a day's checkpoints into signals: tokens per
   session, prompts that look like corrections, failed tool calls, repeated commands and reads,
   large tool outputs, long stretches without a prompt. Code, not a model.
3. **Retro.** `/retro auto` checks the previous daily retro's fixes (worked or repeated), picks at
   most five problems with evidence, and fixes each, code first.
4. **Land.** One goal and one pull request, landed when the checks pass, then released.

## Past sessions, any time

| Need | Command |
| --- | --- |
| Where earlier sessions dealt with something | `scripts/sessions.sh search <term>...` |
| The prompt and session behind a line | `entire why <file>:<line>` |
| What a commit's session did | `entire checkpoint explain <sha> --short` (`--full` for the transcript) |
| Where a session's tokens went | `entire session tokens <session-id>` |

All local. Only these: other `entire` commands (`search`, `activity`, `recap`, `api`, `dispatch`,
`checkpoint explain --generate` or `--repo`) send data to Entire or spend tokens on summaries.

## Rules

- This directory is public. Summarize; never quote a prompt. No personal or client details.
- Prefer code over prose. A rule that can be a check becomes a check.
- A problem that comes back after a fix means the fix was wrong; the next fix is stronger.
