---
paths:
  - "ctdev/profile/**"
---
# Profiles

Machine profiles are declarative TOML files (components + configure categories
applied at recommended values). Built-ins — `pihole-node`, `ai-node`, `dev-workstation`,
`family-desktop` — are **embedded in the binary** (`ctdev/profile/profiles/`),
so a fresh machine can `ctdev apply pihole-node` with nothing but the installed
binary. Local files in `~/.config/ctdev/profiles/<name>.toml` add profiles or
override built-ins by name. `apply` shows the plan and confirms, installs
(deps resolve), batch-configures, then prints the profile's `notes` (its
next-steps runbook — interactive wizards like restic/caddy are never run by
apply, and the `gpu` category is rejected in profiles because MOK signing is
interactive). `diff` exits non-zero on drift, so it works as a cron check.
