# ctdev

[![CI](https://github.com/ConnerTechnology/ctdev/actions/workflows/ci.yml/badge.svg)](https://github.com/ConnerTechnology/ctdev/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/ConnerTechnology/ctdev)](https://github.com/ConnerTechnology/ctdev/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

Conner Technology's internal tool for managing and maintaining devices and the
networks they sit on.

It runs on our own devices today and on clients' devices later. The repository
is public so that a device can install from it, not as an invitation: there is
no support, and nothing here is a roadmap commitment to anyone outside Conner
Technology.

## Fresh Machine Setup

```bash
curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/ctdev/main/install.sh | bash
ctdev install                    # pick the components this machine should have
ctdev configure                  # walk through the system settings
```

`ctdev --help` lists every command and flag.

## What ctdev does

**Components.** A component is one installable thing — a CLI tool, a desktop
app, a runtime, a container stack. `ctdev install` resolves its dependencies,
installs it and runs its configuration step, and is safe to re-run.

**Configure.** System settings, grouped into categories: the SSH server and its
hardening, suspend, automatic updates, the GPU driver, your git identity.
Interactive by default, or `--batch` for the recommended values.

**Update.** One scan across system packages, installed components, the runtimes
they manage and the container stacks ctdev brought up, then you pick what to
install.

**Backups.** `ctdev backup` shows whether the folder you're in is backed up,
sets it up with a destination, and backs it up whole with restic.

**Doctor.** A read-only health report for a machine and the network it sits
on — hardware, OS, DNS, routing, the path out — with a plain-English verdict for
each finding. It assumes nothing about the machine, so it works on one ctdev did
not set up, and it can run from a temporary directory without installing
anything.

ctdev's network job is two things. It **diagnoses any network**, including the
unmanaged devices it cannot be installed on, reading a UniFi, Synology or
Proxmox API when given a read-only key. And it **builds the machines that serve
a network** — a DNS node, a reverse proxy, a monitoring stack. It does not
configure an unmanaged device: it will not log into a router, a switch or an
access point to change a setting.

## What it does to a device

ctdev runs as you and escalates with `sudo` only for the steps that need it: a
package manager, a systemd unit, a file under `/etc` or `/usr/local`. Its own
files live in `~/.local/bin`, `~/.config/ctdev` and `~/.local/state/ctdev`, and
the uninstaller removes them, along with the scheduled-backup timer. It keeps
`~/.config/ctdev/backup`, this computer's backup keys, unless you pass `--purge`.

**No secret is ever stored in this repository.** Every secret is entered at its
`configure` step and written only to the device that needs it, owner-readable
only. Lose the device and you re-enter them from your password manager.

`ctdev doctor` changes nothing, sends no telemetry, and needs no root; a check
that would need it is skipped and says so.

**Releases ship a checksum file and are not signed.** `install.sh` verifies a
download against the release's `SHA256SUMS` and fails closed if it cannot, which
proves the file is the one the release page lists — not who built it.

## More

- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) — symptoms and fixes on a live machine
- [CHANGELOG.md](CHANGELOG.md) — what shipped in each version

## License

MIT — see [LICENSE](LICENSE).
