# Codebase Cleanup & Config Colocation — Design Spec

## Overview

Clean up the codebase after the Go rewrite: rename files, fix bugs, colocate config files with Go code using `go:embed`, replace symlinks with copy-with-backup, delete dead bash code, and update documentation.

## 1. File Renames

Drop `install_` prefix from all 31 files in `ctdev/component/`:

| Old | New |
|-----|-----|
| `install_1password.go` | `onepassword.go` |
| `install_age.go` | `age.go` |
| `install_bun.go` | `bun.go` |
| `install_chatgpt.go` | `chatgpt.go` |
| `install_chrome.go` | `chrome.go` |
| `install_claude_code.go` | `claude_code.go` |
| `install_claude_desktop.go` | `claude_desktop.go` |
| `install_cleanmymac.go` | `cleanmymac.go` |
| `install_codex.go` | `codex.go` |
| `install_dbeaver.go` | `dbeaver.go` |
| `install_docker.go` | `docker.go` |
| `install_doctl.go` | `doctl.go` |
| `install_earlyoom.go` | `earlyoom.go` |
| `install_fonts.go` | `fonts.go` |
| `install_gh.go` | `gh.go` |
| `install_ghostty.go` | `ghostty.go` |
| `install_git.go` | `git.go` |
| `install_git_spice.go` | `git_spice.go` |
| `install_helm.go` | `helm.go` |
| `install_kubectl.go` | `kubectl.go` |
| `install_linear.go` | `linear.go` |
| `install_logi_options.go` | `logi_options.go` |
| `install_node.go` | `node.go` |
| `install_ruby.go` | `ruby.go` |
| `install_slack.go` | `slack.go` |
| `install_solaar.go` | `solaar.go` |
| `install_sops.go` | `sops.go` |
| `install_tailscale.go` | `tailscale.go` |
| `install_terraform.go` | `terraform.go` |
| `install_vscode.go` | `vscode.go` |
| `install_zsh.go` | `zsh.go` |

No code changes — pure renames. All function names stay the same.

## 2. Fix gpu.sh Reference Bug

`cmd/gpu.go` references `cmds/gpu-setup.sh` in 4 places. The actual file is `cmds/gpu.sh`. Fix all 4 references to `cmds/gpu.sh`.

## 3. Config Colocation with go:embed

Move config/template files to live alongside the Go code that deploys them:

```
ctdev/component/configs/ghostty/config
ctdev/component/configs/zsh/.zshrc
ctdev/component/configs/zsh/aliases.zsh
ctdev/component/configs/zsh/exports.zsh
ctdev/component/configs/zsh/exports.local.zsh
ctdev/component/configs/zsh/path.zsh
ctdev/component/configs/zsh/completions/_ctdev
ctdev/component/configs/git/.gitconfig
ctdev/component/configs/claude-code/CLAUDE.md
ctdev/component/configs/claude-code/settings.json
ctdev/component/configs/claude-code/settings.local.json
ctdev/setup/configs/xbindkeys/.xbindkeysrc
ctdev/setup/configs/xbindkeys/xbindkeys.desktop
ctdev/setup/configs/wireplumber/51-ldac-hq.conf
```

Create embed variables in the relevant packages:

In `ctdev/component/configs.go`:
```go
package component

import "embed"

//go:embed configs
var Configs embed.FS
```

In `ctdev/setup/configs.go`:
```go
package setup

import "embed"

//go:embed configs
var Configs embed.FS
```

Update component installers (ghostty, zsh, git, claude-code) to read from `Configs` instead of `findDotfilesRoot()`. Delete `ctdev/component/dotfiles.go` (the `findDotfilesRoot()` helper becomes unnecessary for config deployment).

Note: `cmd/info.go` has a separate `dotfilesRoot()` function used to locate `cmds/setup.sh` and `cmds/gpu.sh`. That function stays — it's in a different package and serves a different purpose (finding the repo root for bash delegation, not for config files).

## 4. Copy-with-Backup Instead of Symlinks

### New function in `ctdev/sysutil/deploy.go`:

```go
func DeployFile(content []byte, dest string) error
```

Behavior:
1. Read existing dest file (if any)
2. If content matches: skip (idempotent), print "already up to date"
3. If dest exists and differs: rename to `<dest>.<YYYY-MM-DDTHH-MM-SS>.bak`
4. Write content to dest
5. Create parent directories as needed

Also add:
```go
func DeployFileFromFS(fs embed.FS, srcPath, dest string) error
```

This reads from the embedded FS and calls `DeployFile`.

### Callers to update:

Replace all `sysutil.SafeSymlink(src, dest)` calls in component installers with `sysutil.DeployFileFromFS(component.Configs, "configs/zsh/.zshrc", "~/.zshrc")`.

Keep `sysutil.SafeSymlink` in the codebase for now (it may be used elsewhere), but the component installers should all use `DeployFile`.

## 5. Delete Dead Bash Code

### Delete entirely:
- `cmds/cleanup.sh`
- `cmds/configure.sh`
- `cmds/info.sh`
- `cmds/install.sh`
- `cmds/uninstall.sh`
- `cmds/update.sh`
- `lib/cli.sh`
- `lib/components.sh`
- `lib/keys.sh`
- `lib/packages.sh`
- `lib/github.sh`
- `lib/logging.sh`
- `lib/platform.sh`
- All `components/*/install.sh` and `components/*/uninstall.sh`
- Empty component directories (those with no remaining config files)

### Keep (still referenced):
- `cmds/setup.sh` — called by `cmd/setup.go` for `--reset` and macOS
- `cmds/gpu.sh` — called by `cmd/gpu.go` for GPU setup/recover
- `lib/utils.sh` — sourced by `cmds/setup.sh`
- `lib/gpu.sh` — sourced by `cmds/gpu.sh`
- Component directories that contain config files (after configs are moved to `ctdev/`, these directories can also be deleted)

### After config migration:
Once configs are embedded in the Go binary, the `components/` directories with only config files can also be deleted. The `findDotfilesRoot()` helper and `component/dotfiles.go` can be deleted.

The `config/` directory at repo root can be deleted after its contents are moved to `ctdev/setup/configs/`.

## 6. Update Documentation

### CLAUDE.md updates:
- Remove bash utility references (`log_info`, `detect_os`, `safe_symlink`, etc.)
- Remove "Adding a new component" bash template
- Add Go component template
- Update directory structure to reflect Go-native layout
- Remove references to `lib/`, `cmds/` (except the 2 kept files)

### README.md updates:
- Reflect that ctdev is a native Go binary
- Update installation instructions
- Update development instructions

## Out of Scope

- Porting `cmds/gpu.sh` and `cmds/setup.sh` to Go (follow-up task)
- Release workflow
- New features
