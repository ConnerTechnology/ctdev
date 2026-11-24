# Getting started

Two starting points: a machine with nothing on it, and a machine you already
work on. Both are below; neither needs a clone of this repo.

## Fresh machine setup

Install the `ctdev` binary, then choose the components and `configure`
categories this machine should have.
For a development machine, the [dev machine recipe](node-recipes/dev-machine.md)
lists the base set to start from.

```bash
# 1. install ctdev
curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/ctdev/main/install.sh | bash

# 2. install the components you want (each runs its configure step afterward)
ctdev install                    # pick them from a list
ctdev install zsh git gh nodenv go docker tailscale vscode claude-code tmux   # or name them

# 3. apply the system settings you want
ctdev configure ssh --batch      # SSH server, key-only login
ctdev configure power --batch    # never suspend (always-on box)
```

`ctdev install <x>` pulls in declared dependencies first and runs `<x>`'s
configure step afterward; `ctdev configure <x>` configures without installing.
Everything is idempotent and safe to re-run.

**Manual steps**

- Add your client's SSH public key: `echo 'ssh-ed25519 ...' >> ~/.ssh/authorized_keys`, then re-run `ctdev configure ssh --batch` (password auth is disabled only once a key is present)
- `gh auth login` · `ctdev configure git` · (optional) `sudo tailscale up`

## Day one on an existing machine

```bash
ctdev configure                 # Walk through all system configuration categories
ctdev install zsh git gh        # Install components you need
ctdev configure git             # Set your git name, email, and signing key
```

Use `--dry-run` on any command to preview changes before applying.

## Where to go next

- [Components](components/README.md) — what `ctdev install` can put on a machine
- [Configure](configure.md) — the system settings categories
- [Commands](commands.md) — every command and flag
