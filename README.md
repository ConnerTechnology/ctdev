# dotfiles

Modular dotfiles for macOS and Linux. Managed via the `ctdev` CLI.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/install.sh | bash
```

## Getting Started

After installing, run the full system setup:

```bash
ctdev setup                     # Configure OS defaults, drivers, services
ctdev install zsh git gh        # Install components you need
ctdev configure git             # Set your git name and email
```

Use `--dry-run` on any command to preview changes before applying.

## Commands

```bash
ctdev install <component...>    # Install specific components
ctdev uninstall <component...>  # Remove specific components
ctdev update [-y]               # Update system packages and components
ctdev info                      # Show system information
ctdev configure git             # Configure git user
ctdev gpu info                  # Show GPU hardware info and signing status
ctdev gpu setup                 # Configure MOK signing for NVIDIA drivers
ctdev setup                     # Set up and configure your OS
ctdev setup --show              # Show current system configuration
ctdev cleanup                   # Run all cleanup tasks
```

Run `ctdev install --help` to see all available components.

**Flags:** `--help`, `--dry-run`, `--verbose`, `--force`, `--version`

## DevContainers

Add to your VS Code `settings.json`:

```json
{
  "dotfiles.repository": "https://github.com/ConnerTechnology/dotfiles.git",
  "dotfiles.targetPath": "~/dotfiles",
  "dotfiles.installCommand": "./devcontainer.sh"
}
```

## Platform Support

- **macOS** - Homebrew
- **Ubuntu/Debian/Linux Mint** - apt
- **Fedora/RHEL** - dnf
- **Arch** - pacman

## Uninstall

```bash
ctdev uninstall <component...>   # Remove specific components
rm ~/.local/bin/ctdev            # Remove the ctdev binary
```

## License

MIT
