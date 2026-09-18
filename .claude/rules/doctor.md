---
paths:
  - "ctdev/diagnose/**"
  - "ctdev/cmd/doctor*.go"
---
# ctdev doctor

`ctdev doctor` is the one command that assumes nothing about the machine — it is
built for diagnosing hardware you did not set up. Every check is read-only, root
is never required (checks needing it report Skipped and say so), and no data
leaves the machine beyond the diagnostic probes themselves.

- **`--root` is how root-only checks run, and it is opt-in.** Never *requiring*
  root hardened into never *asking*, which silently cost SMART health, ufw
  status, container log sizes and the authoritative `sshd -T` read on machines
  where the operator would gladly have typed a password. `--root` calls
  `ensureSudo` in `runDoctor` **before** `GatherFacts`, which settles
  `Facts.Root` once for the whole run — never inside `GatherFacts`, because
  `ctdev status` shares it and must stay silent. Default off: doctor is pointed
  at machines we do not manage, and demanding a stranger's password uninvited is
  the behavior the package doc promises it does not have.
- **A skip never says "re-run with sudo".** ctdev installs to `~/.local/bin`,
  which sudo's `secure_path` excludes on stock Debian/Mint, so `sudo ctdev
  doctor` dies with "command not found" — advice the operator cannot follow.
  `needsRootSkip` is the single place that wording lives, and it names
  `ctdev doctor --root`. A check that root cannot unlock (CPU temperature off
  Linux) must not mention root at all.
- **Checks** live in `ctdev/diagnose/` as a catalog of struct literals with
  closures, built as a function of `platform.Info` + `Facts` — the same shape as
  `cleanup.Task`. Gate at construction time so a wired machine has no Wi-Fi rows
  rather than a column of "n/a".
- **`dns.dnssec` and `dns.roots`** (`dnssec.go`) speak DNS on the wire by hand
  because `net.Resolver` hides the response flags and both verdicts *are* flags.
  DNSSEC is judged by the bogus name (`dnssec-failed.org` must SERVFAIL), never
  by the AD bit — stubs in the path (systemd-resolved, MagicDNS) strip AD but pass
  SERVFAIL through. Root reach runs only when Pi-hole's upstream is on loopback
  (Unbound) and asks a root server non-recursively: only a real root sets AA; a
  transparent ISP proxy cannot, and that is the failure mode that broke DNSSEC in
  the Spencer's Desk Pi-hole write-up ctpi01 was compared against on 2026-09-07.
- **`Check.Network`** marks a check that does network I/O; `ctdev status` reuses
  the same catalog filtered to `!Network && !Deep`, which is what keeps its
  "no network calls" contract honest. **`Check.Deep`** marks slow or third-party
  probes that only run under `--deep`.
- **`Diagnose(results, facts) []Finding`** is the correlation engine: a pure
  function that turns combinations into verdicts. Network rules are layered and
  first-match-wins, because a network fault has one root cause and everything
  downstream is noise. Hardware verdicts accumulate.
- **Vendor integrations** (`integration_*.go`) are read-only *by construction* —
  the `Provider` interface has no action method. They live in package `diagnose`
  rather than a subpackage to avoid an import cycle.
- **Credentials** come from flags, environment (`CTDEV_UNIFI_API_KEY`,
  `CTDEV_SYNOLOGY_PASSWORD`, `CTDEV_PROXMOX_SECRET`, …), or a one-shot prompt
  held in memory. **Nothing is ever written to the machine being diagnosed.**
  They are only ever sent to private addresses, and never appear in a report —
  `visibleData` drops secret-looking keys at the render boundary.
