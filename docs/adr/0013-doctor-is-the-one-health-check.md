---
status: accepted
date: 2026-10-15
---

# Doctor is the one health check

ctdev has no `status` and no `verify` command. `doctor` is the health check and `info` is the
inventory. `status` was removed in CTD-86 (PR #55, merged 2026-10-13, released in v13.0.0) and
`verify` in CTD-88 (Thomas's direction 2026-10-15, PR #58, merged the same day). This ADR is dated
by the second, which completed the decision.

CTD-86 gives the reason: three commands overlapped, Thomas rarely used `status`, and having all
three was confusing. `status`'s health lines were doctor's own checks with the network ones
filtered out, and on a dev Mac it listed other projects' containers under the same label as the
ctdev stacks. CTD-88 calls removing `verify` part of narrowing ctdev's command set, the same way
`status` went.

## Consequences

Doctor took two of `verify`'s checks, and both appear only for what ctdev installed: each installed
tool is on PATH, with its version, and the services ctdev depends on (ssh, tailscaled, docker) are
running. A machine ctdev does not manage shows none of these rows (ADR-0011).

Some checks are no longer anywhere, by decision:

- From `status` (CTD-86): backup freshness, because the feature didn't work well and backup is
  being rethought in CTD-87; and up/down for the ctdev stacks. Doctor still flags crashed,
  restart-looping and unhealthy containers, but not one that stopped cleanly.
- From `verify` (CTD-88): homelab containers running, sleep masked, and the shell and dotfiles
  setup. The restic binary and timer are still checked by `ctdev backup test`. The CHANGELOG lists
  the full set.
