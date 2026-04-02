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

## Communication

- Be concise and direct
- Skip unnecessary preamble
- When uncertain, ask rather than assume
- Don't be lazy. If you don't know something, look it up or ask me.
