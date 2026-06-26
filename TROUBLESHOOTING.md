# Troubleshooting

## ctdev not found

```bash
curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/install.sh | bash
export PATH="$HOME/.local/bin:$PATH"
```

## Permission denied

ctdev uses sudo automatically for operations that need root. If prompted for sudo fails, ensure your user is in the sudo group.

## Component shows as not installed

ctdev detects installed components via command existence and path checks. To force a re-deploy of configs:

```bash
ctdev install <component>         # Re-runs config deployment
ctdev install <component> --force # Full reinstall including package
```

## Upgrading from bash-based ctdev (pre-v9)

The v9.0 Go rewrite replaces symlinks with embedded config files. If you see dangling symlinks after upgrading:

```bash
ctdev install zsh tmux ghostty claude-code git   # Re-deploys all configs
```

This replaces old symlinks (pointing to deleted `components/` directory) with regular files containing the embedded configs.

## Uninstalling

```bash
ctdev uninstall <component...>   # Remove specific components
curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/uninstall.sh | bash
```

Or manually: `rm ~/.local/bin/ctdev`

## macOS

**Xcode popup:** Run `xcode-select --install` manually and wait for it to complete.

**Homebrew not found:** Add to shell profile:
```bash
eval "$(/opt/homebrew/bin/brew shellenv)"  # Apple Silicon
eval "$(/usr/local/bin/brew shellenv)"     # Intel
```

**"Operation not permitted" for defaults:** Grant Terminal full disk access in System Preferences > Security & Privacy.

## Linux

**Expired APT GPG key (EXPKEYSIG):** Re-download signing keys:
```bash
ctdev update --refresh-keys
```

**Fonts not showing:** Run `fc-cache -fv` and restart terminal.

**Package manager not detected:** Run `ctdev info` to see what was detected.

### Desktop Freezes (NVIDIA + Dual GPU)

Repeated system freezes on Linux Mint with Ryzen 7000 series (Raphael iGPU + discrete NVIDIA). The `amdgpu` driver loads for the unused iGPU and fails on suspend/resume cycles.

**1. Disable integrated GPU in BIOS** (recommended)

Disable the Ryzen iGPU in BIOS/UEFI. Verify with:
```bash
lsmod | grep amdgpu   # Should return nothing
```

If BIOS toggle is unavailable, blacklist the module:
```bash
echo "blacklist amdgpu" | sudo tee /etc/modprobe.d/blacklist-amdgpu.conf
sudo update-initramfs -u
sudo reboot
```

**2. NVIDIA suspend stability** (automated by `ctdev configure gpu` / `ctdev configure boot`)

`ctdev configure` handles these on NVIDIA systems:
- NVIDIA suspend/resume/hibernate systemd services (`ctdev configure gpu`)
- GRUB kernel parameters for video memory preservation (`ctdev configure boot`)

To check current status: `ctdev configure gpu --show`

To re-run GPU signing setup: `ctdev configure gpu`

**Monitoring:**
```bash
sudo journalctl -b -1 -p err                      # Errors from previous boot
sudo nvme smart-log /dev/nvme0n1 | grep unsafe     # Unsafe shutdown count
```

## Zsh

**Oh My Zsh not loading:** Check `ls -la ~/.zshrc` — should be a regular file (not a symlink). Re-run `ctdev install zsh`.

**Pure prompt missing:** Delete `~/.zsh/pure` and reinstall: `ctdev install zsh --force`.

**Configs not updating:** `ctdev install zsh` always re-deploys .zshrc, aliases, exports, completions, and path configs even when oh-my-zsh is already installed.

**Cancel a hung install/update:** Ctrl-C terminates the currently running subprocess (apt-get, dkms, large downloads, etc.) thanks to ctx plumbing through the shell-out layer.

## Node/Ruby

**Version manager not found:** These are configured in `~/.zsh/path.zsh` (deployed by `ctdev install zsh`). Restart your shell after installing.

**Build failing:** Install dependencies first:
```bash
# macOS
brew install openssl readline libyaml

# Ubuntu/Debian
sudo apt install build-essential libssl-dev libyaml-dev zlib1g-dev libffi-dev
```

## Debugging

```bash
ctdev --dry-run install zsh      # Preview without changes
ctdev --verbose install zsh      # More output
ctdev info                       # System diagnostics
```

## Still stuck?

Open an issue with `ctdev info` output and the failing command.
