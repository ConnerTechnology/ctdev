# Getting started

Two starting points: a machine with nothing on it, and a machine you already
work on. Both are below; neither needs a clone of this repo.

## Fresh machine setup

Install the `ctdev` binary, then either apply a **machine profile** (the fast
path) or compose the machine by hand from components and `configure` categories.

```bash
# 1. install ctdev
curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/ctdev/main/install.sh | bash

# 2a. the fast path: apply a profile (built in — no repo clone needed)
ctdev apply                      # list profiles: dev-workstation, pihole-node, ai-node, family-desktop
ctdev apply dev-workstation      # plan → confirm → install + batch-configure → next steps
ctdev diff dev-workstation       # later: check the machine hasn't drifted (cron-able)
# add your own: ~/.config/ctdev/profiles/<name>.toml

# 2b. or by hand: install the components you want (each runs its configure step afterward)
ctdev install zsh git gh node go docker tailscale vscode claude-code tmux

# 3. apply the system settings you want
ctdev configure ssh --batch      # SSH server + key-based auth hardening
ctdev configure sleep --batch    # never suspend (always-on box)
ctdev configure locale --batch   # en_US.UTF-8 (for Mosh)
ctdev configure linger --batch   # keep user services alive without a login
ctdev configure tunnel --batch   # VS Code tunnel (optional)
# ctdev configure ufw --batch    # firewall — skip on a DNS/proxy host (Pi-hole/Caddy)
```

`ctdev install <x>` pulls in declared dependencies first and runs `<x>`'s
configure step afterward; `ctdev configure <x>` configures without installing.
Everything is idempotent and safe to re-run.

**Manual steps**

- Add your client's SSH public key: `echo 'ssh-ed25519 ...' >> ~/.ssh/authorized_keys`, then re-run `ctdev configure ssh --batch` (password auth is disabled only once a key is present)
- Authenticate the VS Code tunnel once: `code tunnel user login`
- `gh auth login` · `ctdev configure git` · (optional) `sudo tailscale up`
- Verify everything: `ctdev verify`

## Day one on an existing machine

```bash
ctdev configure                 # Walk through all system configuration categories
ctdev install zsh git gh        # Install components you need
ctdev configure git             # Set your git name, email, and signing key
```

Use `--dry-run` on any command to preview changes before applying.

## Where to go next

- [Profiles](profiles.md) — the declarative form of the block above
- [Components](components/README.md) — what `ctdev install` can put on a machine
- [Configure](configure.md) — the system settings categories
- [Commands](commands.md) — every command and flag
