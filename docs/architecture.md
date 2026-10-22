# Architecture

## Directory structure

```
ctdev/                 Go module root
  cmd/                 Cobra command handlers
  claudecli/           Drives the claude CLI for a repo: local-scope MCP servers, the plugin, the hand-off
  cleanup/             Disk-reclaim task catalog (Linux + macOS), scan/run
  diagnose/            ctdev doctor: check catalog, correlation engine, vendor integrations
  component/           Component registry, installers, and embedded config files
    configs/           Config files deployed by installers (go:embed)
  gitrepo/             Finding the git repo a command runs in, and its .git/info/exclude
  gpu/                 GPU/NVIDIA signing management
  jira/                Jira API tokens per site, and the Atlassian MCP server entry
  linear/              Linear app credentials per workspace, app tokens, the linear MCP server entry
  platform/            OS/arch detection
  secretfile/          Owner-only KEY=VALUE credential files, written atomically
  setup/               System settings (Linux dconf/GRUB, macOS defaults)
    configs/           Setup config files (go:embed)
  state/               Install markers and XDG state
  sysutil/             System utilities (packages, downloads, deploy, diff, exec)
  tui/                 Bubble Tea UI models
```
