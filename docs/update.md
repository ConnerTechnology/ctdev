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

ctdev updates itself by running the new release's own `install.sh` — fetched
from that release's tag, never from `main` — with the running binary's
directory as `INSTALL_DIR`, so the binary is replaced where it already lives.
A system install in `/usr/local/bin` stays there rather than gaining a second
copy in `~/.local/bin`.

Components installed outside a package manager are checked against their
vendor's latest stable release: `helm`, `kubectl`, `terraform`, the T3 Code CLI
(`t3code`) and, on Linux, the T3 Code desktop app (`t3code-desktop`). The T3
Code CLI updates through its own `t3 update`, which also restarts its background
service if you installed one. The Linux desktop app updates by installing the
newer release's `.deb`, checked against its published sha512. On macOS the T3
Code app updates itself, so it isn't listed.

Root is used only for the updates that need it: the system package manager, the
Go toolchain (it lives in `/usr/local/go`), `helm`, `kubectl` and `terraform`
(they live in `/usr/local/bin`), the T3 Code `.deb`, rebuilding a Docker stack,
and replacing a ctdev binary in a directory you cannot write to.
Bun, nodenv, rbenv, pyenv and the T3 Code CLI all stay inside `$HOME` and are
updated as you.

Python updates stay within the minor version you are on: 3.14.8 is offered
3.14.9, never 3.15. A new minor breaks every installed package and virtualenv,
so moving to one is `pyenv install 3.15.0 && pyenv global 3.15.0`, done when
you choose.

See [Commands](commands.md) for every flag, and `ctdev update --help` for the
same list on the machine.
