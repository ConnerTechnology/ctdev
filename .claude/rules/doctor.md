---
paths:
  - "ctdev/diagnose/**"
  - "ctdev/cmd/doctor*.go"
---
# ctdev doctor

Why doctor is read-only and `--root` is opt-in:
`docs/adr/0012-doctor-is-read-only-and-root-is-opt-in.md`. No data leaves the
machine beyond the diagnostic probes themselves.

- **`--root`:** the prompt belongs to `runDoctor`, before `GatherFacts` settles
  `Facts.Root` — never inside package `diagnose`, so callers that only want facts
  never hit a password prompt.
- **A skip never says "re-run with sudo".** ctdev installs to `~/.local/bin`,
  which sudo's `secure_path` excludes on stock Debian/Mint, so `sudo ctdev
  doctor` dies with "command not found" — advice the operator cannot follow.
  Every root skip goes through `needsRootSkip`, which names `ctdev doctor --root`.
  A check that root cannot unlock (CPU temperature off Linux) must not mention
  root at all.
- **What ctdev installed reaches `diagnose` only through `Facts.Components`**, which
  `runDoctor` fills; `diagnose` never imports `component`, so on an unmanaged
  machine the tool and service rows simply don't exist.
- **DNSSEC is judged by the bogus name** (`dnssec-failed.org` must SERVFAIL), never
  by the AD bit — stubs in the path (systemd-resolved, MagicDNS) strip AD but pass
  SERVFAIL through. Root reach asks a root server non-recursively because only a
  real root sets AA; a transparent ISP proxy cannot, and that is the failure mode
  that broke DNSSEC in the Spencer's Desk Pi-hole write-up pi-01 was compared
  against on 2026-09-07.
- **A vendor integration's credentials** come only from flags, the environment, or
  an in-memory prompt. Nothing is ever written to the machine being diagnosed, a
  credential is sent only to a private address (`IsPrivate`), and none appears in a
  report.
