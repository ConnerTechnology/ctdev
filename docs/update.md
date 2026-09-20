# Update

`ctdev update` checks for updates across the machine — system packages, the
components ctdev installed, the runtimes they manage, and the Docker stacks it
brought up — and installs the ones you pick.

```bash
ctdev update                    # scan, pick, install
ctdev update -y                 # install everything found, no prompt
ctdev update --check            # list what is available, install nothing
ctdev update --refresh-keys     # refresh APT GPG keys first
```

The scan refreshes the package index (APT or Homebrew) first, so what it lists
is current; `--no-refresh` skips that when you have just done it yourself.
`--check` is read-only — it never refreshes keys, even if you also pass
`--refresh-keys`, because a key refresh writes to the machine.

`--refresh-keys` exists for the failure that looks like a broken repository: an
APT signing key that expired or rotated, leaving `apt update` complaining about
a signature. Naming components after it narrows which keys are refreshed
(`ctdev update --refresh-keys vscode`); the names do not filter the update scan
itself.

Root is used only for the updates that need it: the system package manager, the
Go toolchain (it lives in `/usr/local/go`), and rebuilding a Docker stack.
Bun, nodenv and rbenv all stay inside `$HOME` and are updated as you.

See [Commands](commands.md) for every flag, and `ctdev update --help` for the
same list on the machine.
