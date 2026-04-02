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
ctdev configure git             # Set your git name, email, and signing key
```

Use `--dry-run` on any command to preview changes before applying.

## Commands

```bash
ctdev install <component...>    # Install specific components
ctdev uninstall <component...>  # Remove specific components
ctdev update [-y]               # Update system packages and components
ctdev update --check            # List available updates without installing
ctdev update --refresh-keys     # Refresh APT GPG keys before updating
ctdev info                      # Show system information
ctdev configure git             # Configure git user and SSH signing key
ctdev configure aws             # Configure AWS profile
ctdev gpu info                  # Show GPU hardware info and signing status
ctdev gpu setup                 # Configure MOK signing for NVIDIA drivers
ctdev setup                     # Set up and configure your OS
ctdev setup --show              # Show current system configuration
ctdev setup --reset             # Reset system configuration to defaults
ctdev cleanup                   # Run all cleanup tasks
```

Run `ctdev install` (no args) for an interactive component picker.

**Flags:** `--help`, `--dry-run`, `--verbose`, `--force`, `--version`

## Components

35 components across CLI tools, desktop apps, runtimes, security, infrastructure, and system utilities. Run `ctdev install` to browse interactively.

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
curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/uninstall.sh | bash
```

## License

MIT
