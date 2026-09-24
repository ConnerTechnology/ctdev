# ctdev documentation

The pages below are in the same order as the README's sections. Every one of
them is plain Markdown with relative links, readable on GitHub as it stands.

## Setting a machine up

- [Getting started](getting-started.md) — a fresh machine, and day one on one you already have
- [Install](install.md) — installing the binary, verifying it, DevContainers, platform support, uninstalling

## What ctdev does

- [Profiles](profiles.md) — declarative machine profiles: `apply` and `diff`
- [Components](components/README.md) — everything `ctdev install` can put on a machine
- [Configure](configure.md) — system settings, by category
- [Update](update.md) — packages, components and stacks
- [Backups](backups.md) — restic snapshots, and what gets backed up
- [Doctor](doctor.md) — diagnosing a machine and the network it sits on

## Node recipes

- [Pi-hole / homelab node](node-recipes/pihole-node.md) — Pi-hole behind Caddy, with a wildcard cert
- [AI / MCP node](node-recipes/ai-node.md) — tailnet-only MCP servers

## Trust

- [Security](security.md) — privilege, file locations, secrets, diagnostics, release verification

## Where ctdev is going

- [Roadmap](roadmap/README.md) — one page per idea, with no dates on purpose

## Reference

- [Commands](commands.md) — every command and flag, with the examples `--help` leaves out
- [Architecture](architecture.md) — the directory map of the Go module
- [Adding a component](adding-a-component.md) — the template, and the rules a new component follows

At the repository root: [RECOVERY.md](../RECOVERY.md) for disaster recovery,
[TROUBLESHOOTING.md](../TROUBLESHOOTING.md) for symptoms on a live machine,
[CHANGELOG.md](../CHANGELOG.md) for what shipped when, and [CONTEXT.md](../CONTEXT.md)
for the words this documentation uses.
