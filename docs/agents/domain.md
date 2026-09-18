# Domain docs

How the engineering skills should consume this repo's domain documentation when exploring the
codebase. Single-context: one glossary, one ADR directory.

## Before exploring, read these

- **`CONTEXT.md`** at the repo root — the glossary. It does not exist until the first term is
  settled; if it is missing, **proceed silently**. `/domain-modeling` creates it when a term
  actually gets resolved.
- **`docs/decisions/`** — the ADRs (`ADR-0001-...md`). This repo's ADR home — not the plugin's
  default ADR folder. Created with the first ADR; if it is missing, proceed silently.

## Use the glossary's vocabulary

When your output names a domain concept — an issue title, a refactor proposal, a test name — use
the term as `CONTEXT.md` defines it. Until it exists, the live vocabulary is the code's: component,
profile, configure category, check, finding.

If the concept you need is not in the glossary, that is a signal: either you are inventing language
the project does not use, or there is a real gap to note for `/domain-modeling`.

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it rather than silently overriding:

> _Contradicts ADR-0002, but worth reopening because…_
