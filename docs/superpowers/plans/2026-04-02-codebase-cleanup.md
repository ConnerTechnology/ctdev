# Codebase Cleanup & Config Colocation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Clean up the codebase after the Go rewrite — rename files, colocate configs with `go:embed`, replace symlinks with copy-with-backup, delete dead bash code, update docs.

**Architecture:** Config files move into `ctdev/` alongside the Go code that deploys them, embedded at build time with `go:embed`. A new `sysutil.DeployFile` replaces `SafeSymlink` for all config deployment. Dead bash scripts and empty directories are deleted.

**Tech Stack:** Go 1.24+, `embed` package, Cobra, Bubble Tea v2

**Spec:** `docs/superpowers/specs/2026-04-02-codebase-cleanup-design.md`

---

## Task 1: Rename component files (drop `install_` prefix)

**Files:** Rename 31 files in `ctdev/component/`

- [ ] **Step 1: Rename all files**

```bash
cd ctdev/component
git mv install_1password.go onepassword.go
git mv install_age.go age.go
git mv install_bun.go bun.go
git mv install_chatgpt.go chatgpt.go
git mv install_chrome.go chrome.go
git mv install_claude_code.go claude_code.go
git mv install_claude_desktop.go claude_desktop.go
git mv install_cleanmymac.go cleanmymac.go
git mv install_codex.go codex.go
git mv install_dbeaver.go dbeaver.go
git mv install_docker.go docker.go
git mv install_doctl.go doctl.go
git mv install_earlyoom.go earlyoom.go
git mv install_fonts.go fonts.go
git mv install_gh.go gh.go
git mv install_ghostty.go ghostty.go
git mv install_git.go git.go
git mv install_git_spice.go git_spice.go
git mv install_helm.go helm.go
git mv install_kubectl.go kubectl.go
git mv install_linear.go linear.go
git mv install_logi_options.go logi_options.go
git mv install_node.go node.go
git mv install_ruby.go ruby.go
git mv install_slack.go slack.go
git mv install_solaar.go solaar.go
git mv install_sops.go sops.go
git mv install_tailscale.go tailscale.go
git mv install_terraform.go terraform.go
git mv install_vscode.go vscode.go
git mv install_zsh.go zsh.go
```

- [ ] **Step 2: Build and test**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev
go build ./... && go test ./...
```

Expected: All pass (no code changes, just renames)

- [ ] **Step 3: Commit**

```bash
git commit -m "refactor: drop install_ prefix from component files"
```

---

## Task 2: Fix gpu.sh reference bug

**Files:**
- Modify: `ctdev/cmd/gpu.go`

- [ ] **Step 1: Fix all 4 references**

In `ctdev/cmd/gpu.go`, replace all occurrences of `gpu-setup.sh` with `gpu.sh`:

- Line 87: `"  [dry-run] bash cmds/gpu-setup.sh --recover"` → `"  [dry-run] bash cmds/gpu.sh --recover"`
- Line 90: `"%s/cmds/gpu-setup.sh"` → `"%s/cmds/gpu.sh"`
- Line 96: `"  [dry-run] bash cmds/gpu-setup.sh"` → `"  [dry-run] bash cmds/gpu.sh"`
- Line 99: `"%s/cmds/gpu-setup.sh"` → `"%s/cmds/gpu.sh"`

- [ ] **Step 2: Build**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add ctdev/cmd/gpu.go
git commit -m "fix: correct gpu.sh script path reference"
```

---

## Task 3: Create DeployFile with backup support

**Files:**
- Create: `ctdev/sysutil/deploy.go`
- Create: `ctdev/sysutil/deploy_test.go`

- [ ] **Step 1: Write tests**

Create `ctdev/sysutil/deploy_test.go`:

```go
package sysutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeployFileCreatesNew(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "test.conf")

	err := DeployFile([]byte("new content"), dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(dest)
	if string(got) != "new content" {
		t.Errorf("expected 'new content', got %q", string(got))
	}

	// No backup should exist
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file, got %d", len(entries))
	}
}

func TestDeployFileSkipsIdentical(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "test.conf")
	os.WriteFile(dest, []byte("same"), 0644)

	err := DeployFile([]byte("same"), dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file (no backup), got %d", len(entries))
	}
}

func TestDeployFileBackupsDifferent(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "test.conf")
	os.WriteFile(dest, []byte("old content"), 0644)

	err := DeployFile([]byte("new content"), dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(dest)
	if string(got) != "new content" {
		t.Errorf("expected 'new content', got %q", string(got))
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatalf("expected 2 files (original + backup), got %d", len(entries))
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".bak") {
			backed, _ := os.ReadFile(filepath.Join(dir, e.Name()))
			if string(backed) != "old content" {
				t.Errorf("backup should have old content, got %q", string(backed))
			}
			return
		}
	}
	t.Error("no .bak file found")
}

func TestDeployFileCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "sub", "dir", "test.conf")

	err := DeployFile([]byte("content"), dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(dest)
	if string(got) != "content" {
		t.Errorf("expected 'content', got %q", string(got))
	}
}
```

- [ ] **Step 2: Run tests — expect failure**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev
go test ./sysutil/ -run TestDeployFile -v
```

Expected: FAIL — `DeployFile` undefined

- [ ] **Step 3: Implement DeployFile**

Create `ctdev/sysutil/deploy.go`:

```go
package sysutil

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DeployFile writes content to dest, backing up any differing existing file.
// If dest already has identical content, it's a no-op.
// Backup format: <filename>.<YYYY-MM-DDTHH-MM-SS>.bak
func DeployFile(content []byte, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("create parent dirs for %s: %w", dest, err)
	}

	existing, err := os.ReadFile(dest)
	if err == nil {
		if bytes.Equal(existing, content) {
			return nil
		}
		stamp := time.Now().Format("2006-01-02T15-04-05")
		backup := fmt.Sprintf("%s.%s.bak", dest, stamp)
		if err := os.Rename(dest, backup); err != nil {
			return fmt.Errorf("backup %s: %w", dest, err)
		}
	}

	return os.WriteFile(dest, content, 0644)
}

// DeployFileFromFS reads a file from an embedded FS and deploys it to dest.
func DeployFileFromFS(fs embed.FS, srcPath, dest string) error {
	content, err := fs.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read embedded %s: %w", srcPath, err)
	}
	return DeployFile(content, dest)
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev
go test ./sysutil/ -run TestDeployFile -v
```

- [ ] **Step 5: Commit**

```bash
git add ctdev/sysutil/deploy.go ctdev/sysutil/deploy_test.go
git commit -m "feat: add DeployFile with backup support"
```

---

## Task 4: Move config files and set up go:embed

**Files:**
- Create: `ctdev/component/configs/` directory tree with config files
- Create: `ctdev/component/configs.go`
- Create: `ctdev/setup/configs/` directory tree with config files
- Create: `ctdev/setup/configs.go`

- [ ] **Step 1: Copy component config files into ctdev/component/configs/**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles

mkdir -p ctdev/component/configs/ghostty
cp components/ghostty/config ctdev/component/configs/ghostty/config

mkdir -p ctdev/component/configs/zsh/completions
cp components/zsh/.zshrc ctdev/component/configs/zsh/.zshrc
cp components/zsh/aliases.zsh ctdev/component/configs/zsh/aliases.zsh
cp components/zsh/exports.zsh ctdev/component/configs/zsh/exports.zsh
cp components/zsh/exports.local.zsh ctdev/component/configs/zsh/exports.local.zsh
cp components/zsh/path.zsh ctdev/component/configs/zsh/path.zsh
cp components/zsh/completions/_ctdev ctdev/component/configs/zsh/completions/_ctdev

mkdir -p ctdev/component/configs/git
cp components/git/.gitconfig ctdev/component/configs/git/.gitconfig

mkdir -p ctdev/component/configs/claude-code
cp components/claude-code/CLAUDE.md ctdev/component/configs/claude-code/CLAUDE.md
cp components/claude-code/settings.json ctdev/component/configs/claude-code/settings.json
cp components/claude-code/settings.local.json ctdev/component/configs/claude-code/settings.local.json

mkdir -p ctdev/component/configs/tmux
cp components/tmux/.tmux.conf ctdev/component/configs/tmux/.tmux.conf
```

- [ ] **Step 2: Copy setup config files into ctdev/setup/configs/**

```bash
mkdir -p ctdev/setup/configs/xbindkeys
cp config/linux/xbindkeys/.xbindkeysrc ctdev/setup/configs/xbindkeys/.xbindkeysrc
cp config/linux/xbindkeys/xbindkeys.desktop ctdev/setup/configs/xbindkeys/xbindkeys.desktop

mkdir -p ctdev/setup/configs/wireplumber
cp config/linux/wireplumber/51-ldac-hq.conf ctdev/setup/configs/wireplumber/51-ldac-hq.conf
```

- [ ] **Step 3: Create embed files**

Create `ctdev/component/configs.go`:

```go
package component

import "embed"

//go:embed configs
var Configs embed.FS
```

Create `ctdev/setup/configs.go`:

```go
package setup

import "embed"

//go:embed configs
var Configs embed.FS
```

- [ ] **Step 4: Build to verify embedding works**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add ctdev/component/configs/ ctdev/component/configs.go ctdev/setup/configs/ ctdev/setup/configs.go
git commit -m "feat: colocate config files with go:embed"
```

---

## Task 5: Update component installers to use embedded configs

**Files:**
- Modify: `ctdev/component/ghostty.go`
- Modify: `ctdev/component/zsh.go`
- Modify: `ctdev/component/git.go`
- Modify: `ctdev/component/claude_code.go`
- Delete: `ctdev/component/dotfiles.go`

- [ ] **Step 1: Rewrite ghostty config deployment**

In `ctdev/component/ghostty.go`, replace the `symlinkGhosttyConfig` function:

```go
func deployGhosttyConfig(o sysutil.Opts) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dst := filepath.Join(home, ".config", "ghostty", "config")

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] deploy ghostty config → %s\n", dst)
		return nil
	}
	return sysutil.DeployFileFromFS(Configs, "configs/ghostty/config", dst)
}
```

Update the caller in `ghosttyInstall` to call `deployGhosttyConfig` instead of `symlinkGhosttyConfig`. Remove the `findDotfilesRoot` import/usage.

- [ ] **Step 2: Rewrite claude-code config deployment**

In `ctdev/component/claude_code.go`, replace `symlinkClaudeCodeConfigs`:

```go
func deployClaudeCodeConfigs(o sysutil.Opts) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configDir := filepath.Join(home, ".claude")

	files := []struct{ src, dst string }{
		{"configs/claude-code/CLAUDE.md", filepath.Join(configDir, "CLAUDE.md")},
		{"configs/claude-code/settings.json", filepath.Join(configDir, "settings.json")},
		{"configs/claude-code/settings.local.json", filepath.Join(configDir, "settings.local.json")},
	}

	for _, f := range files {
		if o.DryRun {
			fmt.Fprintf(o.Stdout, "[dry-run] deploy %s → %s\n", f.src, f.dst)
			continue
		}
		if err := sysutil.DeployFileFromFS(Configs, f.src, f.dst); err != nil {
			return fmt.Errorf("deploy %s: %w", filepath.Base(f.src), err)
		}
	}
	return nil
}
```

- [ ] **Step 3: Rewrite git config deployment**

In `ctdev/component/git.go`, replace the manual copy logic in `gitInstall`:

```go
func gitInstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

	if !sysutil.CommandExists("git") {
		fmt.Fprintln(opts.Stdout, "Installing git...")
		if err := sysutil.InstallPackage(o, "git"); err != nil {
			return err
		}
	}

	fmt.Fprintln(opts.Stdout, "Installing git configuration...")

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dst := filepath.Join(home, ".gitconfig")

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] deploy .gitconfig → %s\n", dst)
		return nil
	}
	if err := sysutil.DeployFileFromFS(Configs, "configs/git/.gitconfig", dst); err != nil {
		return err
	}

	fmt.Fprintln(opts.Stdout, "Git configuration installed")
	return nil
}
```

Remove the `"io"` import since it's no longer needed.

- [ ] **Step 4: Rewrite zsh config deployment**

In `ctdev/component/zsh.go`, replace the symlink section. The `symlinkOrDryRun` helper function should become `deployOrDryRun`:

```go
func deployOrDryRun(o sysutil.Opts, srcEmbed, dst string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] deploy %s → %s\n", srcEmbed, dst)
		return nil
	}
	return sysutil.DeployFileFromFS(Configs, srcEmbed, dst)
}
```

Update the config deployment section to use embedded paths:

```go
deploys := map[string]string{
	"configs/zsh/aliases.zsh":  filepath.Join(omzDir, "custom", "aliases.zsh"),
	"configs/zsh/exports.zsh":  filepath.Join(omzDir, "custom", "exports.zsh"),
	"configs/zsh/path.zsh":     filepath.Join(home, ".zsh", "path.zsh"),
	"configs/zsh/.zshrc":       filepath.Join(home, ".zshrc"),
}

for src, dst := range deploys {
	if err := deployOrDryRun(o, src, dst); err != nil {
		fmt.Fprintf(opts.Stdout, "warning: could not deploy %s: %v\n", filepath.Base(src), err)
	}
}

// ctdev completions
zfuncDir := filepath.Join(home, ".zfunc")
if !o.DryRun {
	os.MkdirAll(zfuncDir, 0755)
}
if err := deployOrDryRun(o, "configs/zsh/completions/_ctdev", filepath.Join(zfuncDir, "_ctdev")); err != nil {
	fmt.Fprintf(opts.Stdout, "warning: could not deploy completions: %v\n", err)
}

// Copy exports.local.zsh only if it doesn't exist
localExports := filepath.Join(omzDir, "custom", "exports.local.zsh")
if !o.DryRun {
	if _, err := os.Stat(localExports); os.IsNotExist(err) {
		if err := sysutil.DeployFileFromFS(Configs, "configs/zsh/exports.local.zsh", localExports); err == nil {
			fmt.Fprintln(opts.Stdout, "Created exports.local.zsh - customize it!")
		}
	}
} else {
	fmt.Fprintf(o.Stdout, "[dry-run] deploy exports.local.zsh if not exists\n")
}
```

- [ ] **Step 5: Update setup/apply.go to use embedded configs**

In `ctdev/setup/apply.go`, update `applyXbindkeys` and `applyWireplumberLDAC` to use embedded configs:

In `applyXbindkeys`:
```go
content, err := Configs.ReadFile("configs/xbindkeys/.xbindkeysrc")
if err != nil {
	return fmt.Errorf("read embedded .xbindkeysrc: %w", err)
}
if err := os.WriteFile(configDst, content, 0644); err != nil {
	return fmt.Errorf("write .xbindkeysrc: %w", err)
}

desktopContent, err := Configs.ReadFile("configs/xbindkeys/xbindkeys.desktop")
if err != nil {
	return fmt.Errorf("read embedded xbindkeys.desktop: %w", err)
}
```

Remove the `DotfilesRoot`-based path construction for these files. Keep `DotfilesRoot` for the setup.sh/gpu.sh bash delegation.

In `applyWireplumberLDAC`:
```go
content, err := Configs.ReadFile("configs/wireplumber/51-ldac-hq.conf")
if err != nil {
	return fmt.Errorf("read embedded wireplumber config: %w", err)
}
tmpFile, err := os.CreateTemp("", "wireplumber-*")
if err != nil {
	return err
}
defer os.Remove(tmpFile.Name())
if _, err := tmpFile.Write(content); err != nil {
	return err
}
tmpFile.Close()
if err := sudoRun("cp", tmpFile.Name(), confDst); err != nil {
	return fmt.Errorf("copy wireplumber config: %w", err)
}
```

- [ ] **Step 6: Delete dotfiles.go**

```bash
git rm ctdev/component/dotfiles.go
```

- [ ] **Step 7: Build and test**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev
go build ./... && go test ./...
```

- [ ] **Step 8: Commit**

```bash
git add ctdev/component/ ctdev/setup/apply.go
git commit -m "refactor: use go:embed for config deployment, replace symlinks with copy-with-backup"
```

---

## Task 6: Delete dead bash code

**Files:** Delete many files and directories

- [ ] **Step 1: Delete dead cmds/ scripts**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles
git rm cmds/cleanup.sh cmds/configure.sh cmds/info.sh cmds/install.sh cmds/uninstall.sh cmds/update.sh
```

- [ ] **Step 2: Delete dead lib/ scripts**

```bash
git rm lib/cli.sh lib/components.sh lib/keys.sh lib/packages.sh lib/github.sh lib/logging.sh lib/platform.sh
```

- [ ] **Step 3: Delete all component install/uninstall bash scripts**

```bash
find components/ -name "install.sh" -o -name "uninstall.sh" -o -name "configure.sh" -o -name "nerd_fonts.sh" | xargs git rm
```

- [ ] **Step 4: Delete empty component directories and directories with only bash scripts removed**

```bash
# Remove directories that are now empty or only had scripts
for dir in components/*/; do
    if [ -z "$(ls -A "$dir" 2>/dev/null)" ]; then
        rmdir "$dir"
    fi
done
# Also remove the top-level config/ directory since configs are now embedded
git rm -r config/
```

Note: Keep `components/` directories that still have non-script files. After step 3, the remaining directories with files should be: ghostty, zsh, git, claude-code, tmux. But since we copied all configs into `ctdev/` in Task 4, these source directories are now dead too.

```bash
git rm -r components/
```

- [ ] **Step 5: Verify nothing references deleted files**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev
grep -rn "components/" --include="*.go" .
grep -rn '"config/' --include="*.go" .
```

The only remaining references should be in `setup/apply.go` for `DotfilesRoot` + `"config/..."` paths, which were updated in Task 5 to use embedded configs. Fix any remaining references.

- [ ] **Step 6: Build and test**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev
go build ./... && go test ./...
```

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "chore: delete dead bash scripts, component dirs, and config dir"
```

---

## Task 7: Update documentation

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: Update CLAUDE.md**

Read the current CLAUDE.md, then rewrite it to reflect the Go-native architecture. Key changes:

- Remove bash utility references (`log_info`, `detect_os`, `safe_symlink`, `run_cmd`, `install_package`, `ensure_git_repo`, `maybe_sudo`, `set -euo pipefail`)
- Remove "Adding a new component" bash template
- Add Go component template showing how to add a new component
- Update directory structure to show `ctdev/` layout
- Remove references to deleted `lib/`, `cmds/` scripts (keep `cmds/setup.sh` and `cmds/gpu.sh` + `lib/utils.sh` and `lib/gpu.sh` as noted)
- Update component count if changed
- Keep the CLI usage section (it's accurate)
- Keep Git commits section
- Keep Releases section

New "Adding a new component" section:

```markdown
## Adding a new component

Create `ctdev/component/<name>.go`:

\`\`\`go
package component

import (
    "context"
    "fmt"

    "github.com/ConnerTechnology/dotfiles/ctdev/platform"
    "github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func nameInstall(ctx context.Context, opts ExecOpts) error {
    p := platform.Detect()
    o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

    if !opts.Force && sysutil.CommandExists("name") {
        fmt.Fprintln(opts.Stdout, "name already installed")
        return nil
    }

    fmt.Fprintln(opts.Stdout, "Installing name...")

    switch p.PackageManager {
    case "brew":
        return sysutil.InstallPackage(o, "name")
    case "apt":
        return sysutil.InstallPackage(o, "name")
    default:
        return fmt.Errorf("name not supported for: %s", p.PackageManager)
    }
}

func nameUninstall(ctx context.Context, opts ExecOpts) error {
    o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
    fmt.Fprintln(opts.Stdout, "Removing name...")
    return sysutil.RemovePackage(o, "name")
}
\`\`\`

Then add to `ctdev/component/registry.go`:

\`\`\`go
{Name: "name", Description: "Description", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: nameInstall, GoUninstall: nameUninstall, Tags: []string{"tag1", "tag2"}},
\`\`\`

For simple package-manager-only installs, use the helper:
\`\`\`go
{Name: "name", ..., GoInstall: SimplePackageInstaller("name"), GoUninstall: SimplePackageUninstaller("name"), ...},
\`\`\`

If the component has config files, place them in `ctdev/component/configs/<name>/` and use `sysutil.DeployFileFromFS(Configs, "configs/<name>/file", dest)` to deploy them.
```

New directory structure:

```markdown
## Directory structure

\`\`\`
ctdev/                 Go module root
  cmd/                 Cobra command handlers
  component/           Component registry, installers, and config files
    configs/           Embedded config files deployed by installers
  platform/            OS/arch detection
  setup/               Linux system settings (dconf, GRUB, systemd)
    configs/           Embedded setup config files
  state/               Install markers and XDG state
  sysutil/             System utilities (packages, downloads, exec)
  tui/                 Bubble Tea UI models
    checklist/         Update checklist
    info/              System info display
    picker/            Component picker
    progress/          Install/uninstall progress
    setup/             Setup wizard
    styles/            Shared Lip Gloss styles
  internal/shell/      Shell execution wrapper
cmds/                  Remaining bash scripts (setup, gpu — pending Go port)
lib/                   Shared bash utilities (used by cmds/)
\`\`\`
```

- [ ] **Step 2: Update README.md**

Keep the README concise. Key changes:
- Remove the manual `git clone` install path (the bootstrap script handles everything)
- Remove the `~/dotfiles/uninstall.sh` reference
- Keep everything else (commands, platform support, devcontainers)

- [ ] **Step 3: Build and test**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev
go build ./... && go test ./...
```

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md README.md
git commit -m "docs: update CLAUDE.md and README.md for Go-native architecture"
```
