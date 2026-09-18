---
paths:
  - "install.sh"
  - "install.ps1"
---
# Install

`install.sh` (repo root) installs just the `ctdev` binary (downloads the latest
release and verifies it against `SHA256SUMS`; there is no source-build path).
`install.sh --doctor` instead runs `ctdev doctor` from a temp directory and
deletes it — nothing installed, no PATH change, no sudo. `install.ps1` is the
Windows equivalent. There is no all-in-one machine bootstrap — compose a machine from
individual `ctdev install <component>` and `ctdev configure <category>` calls.
See README "Fresh Machine Setup".

**Flags:** `--help`, `--verbose`, `--dry-run`, `--force`, `--version`, `--refresh-keys`
