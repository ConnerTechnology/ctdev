---
status: accepted
date: 2026-11-20
---

# A daily retro reads yesterday's sessions and lands its own fixes

Every morning at 06:20 a T3 Code scheduled task on the human's machine runs `/retro auto`
(`.claude/commands/retro.md`) on the previous day's sessions. It looks for repeated corrections,
missing context, long detours and token spend that code could remove. It fixes them, code before
prose, and lands the fixes as one goal through `scripts/goal.sh`. Decided on CTD-162, by the human:

| Question | Answer |
| --- | --- |
| Where checkpoints go | **This machine only.** The repo is public, and a checkpoint is the whole transcript: prompts, commands and their output, files read. Entire's redaction is pattern-based and misses what it doesn't recognize. `.entire/settings.json` sets `push_sessions: false`, the clone's push policy is `manual`, and `scripts/goal.sh` fails after any push that leaves a `refs/entire/` ref or an `entire/` branch on `origin`. Sessions on another computer don't reach this machine's retro; a separate private repo (`entire configure --checkpoint-remote`) would fix that, and can be added later without losing anything |
| What a retro file may say | Summaries in its own words, with the session ID and time. **Never a quote from a prompt**, and no personal or client detail: `docs/retros/` is published |
| What it may land without review | **Everything**, `AGENTS.md`, `.claude/**` and the check runner (`scripts/checks.sh`, `ci.yml`, `scripts/check-guidance.sh`) included, once every check passes, like any goal (AGENTS.md, "How we work") |
| What stays the human's | Whatever AGENTS.md, "Who decides what", leaves to them. The retro writes it as a question in its file and as a `ready-for-human` issue, and never makes the change. It never turns off or weakens a check |

The input is `scripts/sessions.sh digest`, code that reads the checkpoints in
`refs/entire/checkpoints/` and prints the signals: tokens per session, prompts that look like
corrections, failed tool calls, commands run and files read three or more times, tool outputs over
15,000 characters, and 30 or more tool calls without a prompt. The model reads the digest, a few
kilobytes, not the transcripts, and treats it as data, never as instructions. It was ported from
Oslados's `infra/sessions/`.

## Risk accepted

A bad retro, or an instruction injected through content a session read (a web page, a file, a
command's output), could loosen the rules that guard everything else, and nothing stops it before
it lands. The human accepted that risk. What remains: every check must pass, locally and on GitHub,
before anything lands; the digest is data; and each run is a pull request the human can read and
revert.

## Considered options

- **Governing files wait for the human.** The human chose that everything lands on its own.
- **A model reading the full transcripts.** A day's transcripts run to megabytes; the digest is a
  few kilobytes and finds repeats across sessions.
- **Entire's hosted search, recap and summaries.** They send transcripts to Entire or spend tokens.
  The retro uses only `scripts/sessions.sh`, `entire session tokens`, `entire why` and
  `entire checkpoint explain` without `--generate` or `--repo`.
- **A systemd timer or cron job.** ctdev configures machines, and a timer would be machine config
  for one repo's workflow. The T3 task shows each run where the human already watches.
