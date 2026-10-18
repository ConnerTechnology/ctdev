---
status: accepted
date: 2026-09-15
---

# Doctor is read-only, and `--root` is opt-in

`ctdev doctor` assumes nothing about the machine: it is built for devices ctdev did not set up.
Every check is read-only, and root is never required: a check that needs it reports Skipped and
says so. `ctdev doctor --root` asks for a sudo password once, before any facts are gathered, so the
root-only checks can run. It is off by default.

Doctor was added read-only in v12.14.0 (commit `cb4545ca`, 2026-08-13), as "a health report for
machines ctdev doesn't manage", for "the laptop someone hands you". `--root` was added in v12.25.0
(commit `29ce2bfc`, 2026-09-15), which is this ADR's date. Neither change has a Linear issue or a
pull request; they predate both. The reasons here come from the CHANGELOG entries for those two
versions and from `.claude/rules/doctor.md`. Never requiring root had hardened into never asking,
so SMART health, `ufw status`, container log sizes and the authoritative `sshd -T` read were
skipped on machines whose owner would gladly have typed a password. `--root` stays opt-in because
doctor is pointed at machines we do not manage, and asking a stranger for their password uninvited
is the behavior the package says it does not have. The record gives no reason for making every
check read-only beyond doctor being meant for machines ctdev does not manage.

## Consequences

A root-only skip never advises `sudo ctdev doctor`: ctdev installs to `~/.local/bin`, which sudo's
`secure_path` leaves out on stock Debian and Mint, so that command fails. The skip names
`ctdev doctor --root` instead.

Vendor integrations under `--deep` are read-only by construction: the provider interface has no
method that changes anything.
