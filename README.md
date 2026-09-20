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
ctdev apply dev-workstation      # plan → confirm → install + batch-configure → next steps
ctdev diff dev-workstation       # later: check the machine hasn't drifted (cron-able)
```

Both paths — a profile, or components and settings picked by hand — are in
[Getting started](docs/getting-started.md). Installing the binary on its own,
verifying it, and removing it are in [Install](docs/install.md).

## What ctdev does

**[Profiles](docs/profiles.md).** A machine profile is a TOML file saying which
components a machine should have and which settings should be applied. `ctdev
apply` makes the machine match it; `ctdev diff` says where it has drifted. Four
profiles are built into the binary, so a machine with nothing else on it can
apply one.

**[Components](docs/components/README.md).** A component is one installable
thing — a CLI tool, a desktop app, a runtime, a container stack. `ctdev install`
resolves its dependencies, installs it and runs its configuration step, and is
safe to re-run.

**[Configure](docs/configure.md).** System settings, grouped into categories:
the SSH server and its hardening, the firewall, suspend, locale, automatic
updates, the GPU driver, your git identity. Interactive by default, or
`--batch` for the recommended values.

**[Update](docs/update.md).** One scan across system packages, installed
components, the runtimes they manage and the container stacks ctdev brought up,
then you pick what to install.

**[Backups](docs/backups.md).** Encrypted restic snapshots on a daily timer, to
any backend restic supports. Opt-in: you choose what to include in a local web
UI, and nothing is snapshotted until you do.

**[Doctor](docs/doctor.md).** A read-only health report for a machine and the
network it sits on — hardware, OS, DNS, routing, the path out — with a
plain-English verdict for each finding. It assumes nothing about the machine,
so it works on one ctdev did not set up, and it can run from a temporary
directory without installing anything.

ctdev's network job is two things. It **diagnoses any network**, including the
unmanaged devices it cannot be installed on, reading a UniFi, Synology or
Proxmox API when given a read-only key. And it **builds the machines that serve
a network** — a DNS node, a reverse proxy, a monitoring stack. It does not
configure an unmanaged device: it will not log into a router, a switch or an
access point to change a setting.

## Node recipes

- [Pi-hole / homelab node](docs/node-recipes/pihole-node.md) — Pi-hole behind a
  Caddy reverse proxy with a Let's Encrypt wildcard cert, nothing exposed to the
  internet
- [AI / MCP node](docs/node-recipes/ai-node.md) — an always-on machine holding
  the credentials the laptops should not, reachable only over Tailscale

## What it does to a device

ctdev runs as you and escalates with `sudo` only for the steps that need it: a
package manager, a systemd unit, a file under `/etc` or `/usr/local`. Its own
files live in `~/.local/bin`, `~/.config/ctdev` and `~/.local/state/ctdev`, and
the uninstaller removes them.

**No secret is ever stored in this repository.** Every secret is entered at its
`configure` step and written only to the device that needs it, owner-readable
only. Lose the device and you re-enter them from your password manager.

`ctdev doctor` changes nothing, sends no telemetry, and needs no root; a check
that would need it is skipped and says so.

**Releases ship a checksum file and are not signed.** `install.sh` verifies a
download against the release's `SHA256SUMS` and fails closed if it cannot, which
proves the file is the one the release page lists — not who built it.

The full account is in [Security](docs/security.md).

## Roadmap

Ideas, not commitments. Each page says what the idea is and why ctdev wants it;
there are no dates, on purpose.

- [The fleet](docs/roadmap/fleet.md) — an agent on each managed device,
  reporting to a dashboard and taking commands from it
- [The phone app](docs/roadmap/phone-app.md) — so a phone can be a managed device
- [UniFi network setup](docs/roadmap/unifi-network-setup.md) — ctdev sets a
  UniFi network up to a standard, not just reads it
- [AI capabilities](docs/roadmap/ai-capabilities.md) — ctdev reasons about
  situations it was not programmed for, without a new release
- [The docs site](docs/roadmap/docs-site.md) — this repository's Markdown,
  published as a website

## Documentation

- [Documentation index](docs/README.md) — every page, in this order
- [RECOVERY.md](RECOVERY.md) — restoring a machine from a restic backup
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) — symptoms and fixes on a live machine
- [CHANGELOG.md](CHANGELOG.md) — what shipped in each version

## License

MIT — see [LICENSE](LICENSE).
