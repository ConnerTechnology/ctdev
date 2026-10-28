---
status: accepted
date: 2026-10-15
---

# No machine profiles: the person at the computer chooses what to install

ctdev has no machine profiles. `ctdev apply`, `ctdev diff`, the built-in profiles
(`pihole-node`, `ai-node`, `dev-workstation`, `family-desktop`) and local profiles in
`~/.config/ctdev/profiles/` are gone. `ctdev install` with no arguments opens a picker over the
registry, and that is how a machine's components are chosen. Decided by Thomas on 2026-10-15 in
CTD-71, and merged the same day in PR #57.

Thomas's direction in CTD-71: ctdev should be able to install and set things up, and the person
at the computer chooses what. The issue gives no further reason for dropping profiles.

## Considered options

CTD-71 settled what each part of a profile became:

- **Configure step:** nothing replaces it. `install` already configures each component it
  installs, and `ctdev configure` covers everything else.
- **Drift check:** `ctdev diff` is dropped, and nothing replaces it. The issue gives no reason.
- **Picker memory**, so that re-running the picker becomes the new `apply`: out of scope, and
  would need its own ticket.

## Consequences

The node recipes in `docs/node-recipes/` carry what only the profiles held, as explicit
`install` and `configure` steps. `ctdev update` deletes leftover local profiles and the recorded
applied profile, and `--dry-run` only reports them, so this is one-way for those files.
