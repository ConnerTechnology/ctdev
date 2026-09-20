# Install

`install.sh` installs just the `ctdev` binary: it downloads the latest release
for this platform, verifies it against the release's `SHA256SUMS`, and moves it
into place. There is no source-build path and nothing else is installed. It
never sits on a password prompt: the one privileged thing it does — clearing a
stale `ctdev` out of `/usr/local/bin` that an older install left there — uses
`sudo -n` and warns instead of asking if that is not already allowed.

## Install (ctdev only)

```bash
curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/ctdev/main/install.sh | bash
```

The binary lands in `~/.local/bin/ctdev`. Set `INSTALL_DIR` to put it somewhere
else. If the install directory is not on your `PATH`, the script says so and
prints the line to add to your shell profile.

On Windows, `install.ps1` is the equivalent. It installs to
`%LOCALAPPDATA%\Programs\ctdev` and adds that to the user `PATH`; Windows runs
`ctdev doctor` and nothing else.

```powershell
irm https://raw.githubusercontent.com/ConnerTechnology/ctdev/main/install.ps1 | iex
```

## Verifying a download

The install script verifies what it downloads for you: it fetches `SHA256SUMS`
from the same release and compares. It fails closed — a missing sums file, a
missing entry for this platform, or no `sha256sum` on the machine aborts the
install rather than proceeding unverified. `CTDEV_SKIP_VERIFY=1` turns that off
and exists for releases that predate checksums.

To check a binary you downloaded by hand, fetch `SHA256SUMS` from the same
release page and run `sha256sum -c`.

A checksum proves the download was not corrupted in transit. It does not prove
who built the binary: releases are not signed today. See
[Security](security.md).

## DevContainers

Add to your VS Code `settings.json`:

```json
{
  "dotfiles.repository": "https://github.com/ConnerTechnology/ctdev.git",
  "dotfiles.targetPath": "~/ctdev",
  "dotfiles.installCommand": "./devcontainer.sh"
}
```

VS Code calls these settings "dotfiles" — that is the name of its hook, not a
description of this repo.

## Platform Support

- **Ubuntu/Debian/Linux Mint** - apt (primary target)
- **macOS** - Homebrew
- **Windows** - `ctdev doctor` only; every other command refuses up front

Other package managers (dnf, pacman) are not supported; components on those
systems report as skipped.

## Uninstall

```bash
ctdev uninstall <component...>   # Remove specific components
curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/ctdev/main/uninstall.sh | bash
```

The second line removes the `ctdev` binary itself, along with its config, state
and cache directories (`~/.config/ctdev`, `~/.local/state/ctdev`,
`~/.cache/ctdev`). Components you installed with it stay where they are; remove
those first if you want them gone.
