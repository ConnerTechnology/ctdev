# Profiles

A machine profile is a declarative TOML file: the components a machine should
have, and the `configure` categories it should have applied, at recommended
values. `ctdev apply <profile>` makes the machine match it; `ctdev diff
<profile>` reports where it no longer does.

```bash
ctdev apply                     # list the profiles this binary knows
ctdev apply dev-workstation     # plan → confirm → install + batch-configure → next steps
ctdev diff dev-workstation      # what has drifted since
```

## The built-in profiles

Four profiles are **embedded in the binary**, so a machine with nothing but
`ctdev` on it can apply one without cloning this repo:

| Profile | What it is |
| --- | --- |
| `dev-workstation` | Development workstation (shell, runtimes, containers, CLI tools) |
| `pihole-node` | Pi-hole DNS node behind Caddy — see the [recipe](node-recipes/pihole-node.md) |
| `ai-node` | AI/MCP services node behind Tailscale — see the [recipe](node-recipes/ai-node.md) |
| `family-desktop` | Family desktop (browser, auto security updates, remotely manageable) |

## Your own profiles

Drop a `<name>.toml` in `~/.config/ctdev/profiles/`. Local files are merged in
and win on a name conflict, so you can add a profile or override a built-in one
under the same name.

## What apply does, and what it leaves alone

`apply` shows the plan and asks before it does anything. It then installs the
listed components — resolving their dependencies first — batch-configures the
listed categories, and prints the profile's `notes`, which are its next-steps
runbook.

Two things it deliberately does not do. **Interactive wizards are never run by
`apply`**: restic and Caddy ask for secrets, so they stay a separate step you
run yourself. And the **`gpu` category is rejected in a profile**, because MOK
signing cannot be answered non-interactively.

`diff` exits non-zero when the machine has drifted, which is what makes it
usable as a cron check.

Every flag is in [Commands](commands.md), and `ctdev apply --help` prints the
same.
