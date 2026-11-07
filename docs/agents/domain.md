# Domain docs

How the engineering skills should consume this repo's domain documentation when exploring the
codebase. Single-context: one glossary, one ADR directory.

## Before exploring, read these

- **`CONTEXT.md`** at the repo root — the glossary. `/domain-modeling` adds a term when one is
  resolved.
- **`docs/adr/`** — the decisions, one per file. A new ADR follows `docs/adr/template.md`, which
  adds a date to the skill's format and makes the status required. A superseded ADR is
  history: the one that replaced it is what holds. Numbering starts at 0002; there is no 0001.

## Use the glossary's vocabulary

When your output names a domain concept — an issue title, a refactor proposal, a test name — use
the term as `CONTEXT.md` defines it. Where the glossary is silent, use the code's words: component,
configure category, check, finding.

If the concept you need is not in the glossary, that is a signal: either you are inventing language
the project does not use, or there is a real gap to note for `/domain-modeling`.

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it rather than silently overriding:

> _Contradicts ADR-0002, but worth reopening because…_
