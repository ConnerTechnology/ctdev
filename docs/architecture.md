# Architecture

## Directory structure

```
ctdev/                 Go module root
  cmd/                 Cobra command handlers
  cleanup/             Disk-reclaim task catalog (Linux + macOS), scan/run
  diagnose/            ctdev doctor: check catalog, correlation engine, vendor integrations
  component/           Component registry, installers, and embedded config files
    configs/           Config files deployed by installers (go:embed)
  gpu/                 GPU/NVIDIA signing management
  platform/            OS/arch detection
  profile/             Machine profiles (embedded TOML + ~/.config/ctdev/profiles)
  setup/               System settings (Linux dconf/GRUB, macOS defaults)
    configs/           Setup config files (go:embed)
  state/               Install markers and XDG state
  sysutil/             System utilities (packages, downloads, deploy, exec)
  tui/                 Bubble Tea UI models
```
