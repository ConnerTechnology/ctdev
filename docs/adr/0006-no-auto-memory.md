# No auto memory

Claude Code's auto memory is off (`autoMemoryEnabled: false`). Decisions are captured where they can
be reviewed and acted on: the repo's Linear project for work, `CONTEXT.md` for vocabulary, and
`docs/adr/` for decisions. A memory directory nobody reads back drifts out of step with the code.

Project settings outrank user settings, so turning memory on globally with `/memory` doesn't turn it
back on here.
