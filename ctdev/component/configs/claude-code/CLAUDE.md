# User Preferences

## Git Commits

- Use 1-line commit messages only (no body, no footer)
- No "Co-Authored-By" or "Created with" lines
- Use conventional commits format: `type: description`
- No subject line beyond the type prefix

## Web Searches

- IMPORTANT: Always use the current year from system context when searching
- Prefer recent/up-to-date sources over older documentation

## Code Style

- Prefer simple, readable code over clever solutions
- Avoid unnecessary abstractions
- Use modern language features when appropriate
- DRY principle: Don't Repeat Yourself
- SOLID principles where applicable
- Use the humanizer skill to write comments that explain the "why" behind complex code
- Don't write unnecessary comments for self-explanatory code

### Go Specific

- Use `inst` as the receiver name instead of single letters (e.g., `func (inst *Handler)` not `func (h *Handler)`)
- Don't use sleeps for timing in tests. Use channels and waitgroups instead to synchronize goroutines.

## UI and Design

- Building or reshaping a UI: search **Mobbin** (`mcp__mobbin__*`) for how shipped apps
  actually solved it before designing from scratch. Especially for flows with established
  conventions — onboarding, auth, paywalls, settings, empty states, checkout.
- Mobbin is reference, not a template. Take the pattern, not the pixels.
- Pair it with the `frontend-design` skill: Mobbin supplies precedent, the skill supplies
  aesthetic direction.

## How we work

- I delegate to the lead. The lead interviews me, writes the spec and tickets with me, and reviews
  every diff in a fresh context against the plan: the diff and the evidence, not the agent's
  report. It asks me only when unsure or when the decision is mine.
- Fable stays with the lead; delegated work runs on Opus unless I say otherwise.
- Progress lives where I watch it, not in the transcript: the tracker is updated as work
  happens, and the initiative gets an end-of-day update like a note to a boss.
- A decision made in conversation reaches the file the next session will read — the glossary, an
  ADR, an agent file — in the same session. I won't find the gap myself.

## Communication

- Be concise and direct
- American English everywhere: color, organized, behavior, center
- Skip unnecessary preamble
- When uncertain, ask rather than assume
- One question or decision per message, with a proposed default — inside `/grilling` too, where
  the skill would ask a numbered round; audits and reviews are walked one item at a time.
  Approving a list of already-scoped work in one go is different.
- I can't be duplicated; agents can. Work that needs me comes one item at a time — "what next"
  names a single item and what it unblocks, the full list only when asked. Work agents can do
  without me runs in parallel and doesn't wait on that queue.
- Don't be lazy. If you don't know something, look it up or ask me.
- Before a change, say what it does to the work that comes after it on the board, and shape it so
  that work doesn't have to undo it.
- Anything I have to do myself comes as numbered steps, one action each: where to go, what to
  click, what to enter, verified against the vendor's current UI. When a step changes, restate the
  whole sequence from step 1.
