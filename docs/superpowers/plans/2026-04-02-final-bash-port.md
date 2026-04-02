# Final Bash-to-Go Port + Release Workflow — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port GPU signing and setup reset/macOS to native Go, add release workflow, delete all remaining bash.

**Architecture:** New `ctdev/gpu/` package for MOK/DKMS/Secure Boot logic. New functions in `ctdev/setup/` for reset and macOS. GitHub Actions release workflow builds 4 platform binaries.

**Tech Stack:** Go 1.24+, Cobra, os/exec for system commands, GitHub Actions

**Spec:** `docs/superpowers/specs/2026-04-02-final-bash-port-design.md`

---

## Task 1: Create GPU detection package

**Files:**
- Create: `ctdev/gpu/detect.go`
- Create: `ctdev/gpu/detect_test.go`

- [ ] **Step 1: Create detect.go with constants and pure parsing functions**

Create `ctdev/gpu/detect.go`. Port all detection logic from `lib/gpu.sh`. Every function that calls an external command should be structured as: run command → parse output. Extract the parsing into testable helpers where output parsing is non-trivial.

Key functions to implement:
- `IsSecureBootEnabled()` — runs `mokutil --sb-state`, checks for "SecureBoot enabled"
- `IsNvidiaLoaded()` — runs `lsmod`, checks for "nvidia " at line start
- `GetDriverVersion()` — tries nvidia-smi then modinfo
- `DetectVariant()` — checks modinfo license field ("MIT"/"GPL" = open, else closed)
- `GetRenderingBackend()` — runs glxinfo, parses renderer string
- `MOKKeyExists()` — os.Stat on MOK.priv and MOK.der
- `MOKKeyEnrolled()` — gets certificate fingerprint via openssl, checks mokutil --list-enrolled
- `DKMSSigningConfigured()` — checks framework.conf for uncommented mok_signing_key/mok_certificate
- `DKMSFrameworkConfConfigured()` — helper for above
- `ModuleSignatureValid()` — compares modinfo signer against certificate CN
- `FindMOKClutter()` — lists files in MOK dir that aren't MOK.priv/MOK.der
- `GetModuleSigner()` — modinfo nvidia | grep signer
- `GetNvidiaDKMSInfo()` — parses dkms status output
- `FindNvidiaModules()` — finds .ko files in DKMS path

Constants:
```go
const (
    MOKDir            = "/var/lib/shim-signed/mok"
    MOKPriv           = MOKDir + "/MOK.priv"
    MOKCert           = MOKDir + "/MOK.der"
    DKMSFrameworkConf = "/etc/dkms/framework.conf"
    DKMSConfDir       = "/etc/dkms/framework.conf.d"
    DKMSConf          = DKMSConfDir + "/sign-modules.conf"
    DKMSSignScript    = "/etc/dkms/sign-module.sh"
)
```

- [ ] **Step 2: Create detect_test.go with parser tests**

Test the output-parsing functions with sample command output. Use table-driven tests.

```go
func TestParseMokutilSBState(t *testing.T) {
    tests := []struct {
        name   string
        output string
        want   bool
    }{
        {"enabled", "SecureBoot enabled\n", true},
        {"disabled", "SecureBoot disabled\n", false},
        {"empty", "", false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := parseSecureBootState(tt.output)
            if got != tt.want {
                t.Errorf("parseSecureBootState(%q) = %v, want %v", tt.output, got, tt.want)
            }
        })
    }
}

func TestParseLsmodNvidia(t *testing.T) {
    tests := []struct {
        name   string
        output string
        want   bool
    }{
        {"loaded", "nvidia 123456 0\nnvidia_drm 45678 1\n", true},
        {"not loaded", "i915 123456 0\n", false},
        {"empty", "", false},
    }
    // ...
}

func TestParseDKMSFrameworkConf(t *testing.T) {
    tests := []struct {
        name    string
        content string
        want    bool
    }{
        {"configured", "mok_signing_key=/var/lib/shim-signed/mok/MOK.priv\nmok_certificate=/var/lib/shim-signed/mok/MOK.der\n", true},
        {"commented", "# mok_signing_key=/var/lib/shim-signed/mok/MOK.priv\n", false},
        {"missing", "some_other_key=value\n", false},
    }
    // ...
}

func TestParseModinfoVariant(t *testing.T) {
    tests := []struct {
        name    string
        license string
        want    string
    }{
        {"open-mit", "Dual MIT/GPL", "open"},
        {"open-gpl", "GPL v2", "open"},
        {"closed", "NVIDIA", "closed"},
        {"empty", "", "unknown"},
    }
    // ...
}
```

- [ ] **Step 3: Run tests**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go test ./gpu/ -v
```

- [ ] **Step 4: Commit**

```bash
git add ctdev/gpu/
git commit -m "feat: add GPU detection package with MOK/DKMS status checks"
```

---

## Task 2: Create GPU setup/recover actions

**Files:**
- Create: `ctdev/gpu/setup.go`

- [ ] **Step 1: Create setup.go with action functions**

Port the action functions from `lib/gpu.sh` and the orchestration from `cmds/gpu.sh`:

```go
package gpu

import (
    "fmt"
    "io"
    "os"
    "os/exec"
    "strings"
    "time"
)

type Opts struct {
    Stdout  io.Writer
    Stdin   io.Reader
    DryRun  bool
    Force   bool
    Verbose bool
}

func RunSetup(opts Opts) error
func RunRecover(opts Opts) error
func CreateMOKKeypair(opts Opts) error
func ConfigureDKMSFramework(opts Opts) error
func ConfigureDKMSLegacy(opts Opts) error
func EnrollMOK(opts Opts) error
func RebuildDKMS(opts Opts) error
func SignNvidiaModules(opts Opts) error
func CleanMOKClutter(opts Opts) error
```

`RunSetup` orchestrates: pre-flight checks → create keypair → clean clutter → configure DKMS → enroll MOK → rebuild DKMS → print reboot instructions.

`RunRecover` orchestrates: check keys exist → check if already enrolled → enroll MOK → print reboot instructions.

`EnrollMOK` must pass stdin through for the mokutil password prompt:
```go
cmd := exec.Command("sudo", "mokutil", "--import", MOKCert)
cmd.Stdin = opts.Stdin
cmd.Stdout = opts.Stdout
cmd.Stderr = opts.Stdout
```

All functions respect `opts.DryRun` by printing what would happen.

- [ ] **Step 2: Build**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add ctdev/gpu/setup.go
git commit -m "feat: add GPU setup and recovery actions"
```

---

## Task 3: Create GPU info display

**Files:**
- Create: `ctdev/gpu/info.go`

- [ ] **Step 1: Create info.go**

Port the structured status checking from `cmds/gpu.sh` `gpu_info()`:

```go
package gpu

type StatusCheck struct {
    Name   string
    Pass   bool
    Detail string
}

func GatherStatus() []StatusCheck
```

`GatherStatus` runs all detection checks and returns structured results:
1. Secure Boot status
2. NVIDIA driver loaded + variant
3. MOK key exists
4. MOK directory clutter check
5. DKMS signing configured
6. MOK key enrolled
7. Module signature matches enrolled key

Also port `ShowHardwareInfo(w io.Writer)` — the detailed nvidia-smi hardware info display from `lib/gpu.sh` `show_gpu_hardware_info()`.

- [ ] **Step 2: Build**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add ctdev/gpu/info.go
git commit -m "feat: add GPU info display with structured status checks"
```

---

## Task 4: Wire GPU package into cmd/gpu.go

**Files:**
- Modify: `ctdev/cmd/gpu.go`

- [ ] **Step 1: Rewrite cmd/gpu.go to use gpu package**

Replace the entire file. Remove all `detect*` helper functions and bash delegation. The command becomes a thin Cobra wrapper:

`runGPUInfo` calls `gpu.GatherStatus()` and `gpu.ShowHardwareInfo()`, renders with lipgloss styles.

`runGPUSetup` calls `gpu.RunSetup(gpu.Opts{Stdout: os.Stdout, Stdin: os.Stdin, DryRun: flagDryRun, Force: flagForce})`.

`runGPUSetup` with `--recover` calls `gpu.RunRecover(...)`.

Remove `dotfilesRoot()` usage from gpu.go — no more bash script paths.

- [ ] **Step 2: Build and test**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go build ./... && go test ./...
```

- [ ] **Step 3: Commit**

```bash
git add ctdev/cmd/gpu.go
git commit -m "refactor: wire GPU package into cmd, remove bash delegation"
```

---

## Task 5: Port setup reset to Go

**Files:**
- Create: `ctdev/setup/reset.go`

- [ ] **Step 1: Create reset.go**

Port `linux_mint_reset()` from `cmds/setup.sh`. Each dconf/gsettings reset is an `exec.Command` call:

```go
package setup

func ResetLinuxDefaults(w io.Writer, dryRun bool) error
```

The function:
1. Resets power settings: `powerprofilesctl set balanced`, dconf resets
2. Resets screensaver settings: dconf resets
3. Resets keyboard: gsettings reset + `xset r rate`
4. Resets mouse: dconf resets
5. Resets sound: dconf reset
6. Resets nemo: dconf reset
7. Removes wireplumber LDAC config
8. Stops xbindkeys, removes autostart
9. Resets GRUB settings (uses existing `applyGrubVar` from apply.go)
10. Resets NVIDIA suspend GRUB params + disables services
11. Removes wifi suspend hook
12. Disables fstrim timer
13. Runs `sudo update-grub`

Each step prints what it's doing. DryRun prints what would happen.

- [ ] **Step 2: Build**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add ctdev/setup/reset.go
git commit -m "feat: port Linux setup reset to native Go"
```

---

## Task 6: Port macOS setup to Go

**Files:**
- Create: `ctdev/setup/macos.go`

- [ ] **Step 1: Create macos.go**

Port `macos_apply()` from `cmds/setup.sh`:

```go
package setup

func ApplyMacOSDefaults(w io.Writer, dryRun bool) error
```

Implementation is straightforward — each `defaults write` becomes:
```go
exec.Command("defaults", "write", "com.apple.dock", "autohide", "-bool", "true").Run()
```

Group the commands by category (Dock, Sound, Finder, Keyboard, Dialogs, Security).
End with `killall Dock` and `killall Finder`.

DryRun mode prints what would be configured without running anything.

- [ ] **Step 2: Build**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add ctdev/setup/macos.go
git commit -m "feat: port macOS setup defaults to native Go"
```

---

## Task 7: Wire setup reset/macOS into cmd/setup.go

**Files:**
- Modify: `ctdev/cmd/setup.go`

- [ ] **Step 1: Replace bash delegation with Go calls**

In `runSetupReset()`:
```go
func runSetupReset() error {
    return setup.ResetLinuxDefaults(os.Stdout, flagDryRun)
}
```

In `runMacOSSetup()`:
```go
func runMacOSSetup() error {
    return setup.ApplyMacOSDefaults(os.Stdout, flagDryRun)
}
```

Remove the bash script delegation code. Remove `exec` import if no longer needed. Remove `dotfilesRoot()` usage from setup.go.

- [ ] **Step 2: Remove DotfilesRoot from setup package**

In `ctdev/setup/apply.go`, check if `DotfilesRoot` is still used. The only remaining reference was for `applyNvidiaSigning` which calls `cmds/gpu.sh`. Since GPU is now handled by the `gpu` package, this reference can be removed.

If `DotfilesRoot` is only used by the nvidia signing apply function, update that function to call `gpu.RunSetup` or remove it.

- [ ] **Step 3: Build and test**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go build ./... && go test ./...
```

- [ ] **Step 4: Commit**

```bash
git add ctdev/cmd/setup.go ctdev/setup/apply.go
git commit -m "refactor: wire Go setup reset/macOS, remove bash delegation"
```

---

## Task 8: Delete remaining bash and clean up

**Files:**
- Delete: `cmds/setup.sh`, `cmds/gpu.sh`
- Delete: `lib/utils.sh`, `lib/gpu.sh`
- Delete: `cmds/` directory
- Delete: `lib/` directory
- Modify: `ctdev/cmd/info.go` (clean up dotfilesRoot if no longer needed)
- Modify: `ctdev/component/executor.go` (remove bash bridge if dead)
- Modify: `CLAUDE.md` (remove cmds/ and lib/ from directory structure)

- [ ] **Step 1: Delete bash files and directories**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles
git rm cmds/setup.sh cmds/gpu.sh lib/utils.sh lib/gpu.sh
rmdir cmds lib
```

- [ ] **Step 2: Check if dotfilesRoot() is still needed**

```bash
grep -rn "dotfilesRoot()" ctdev/cmd/ --include="*.go"
```

Remaining callers after setup.go and gpu.go are cleaned up:
- `cmd/info.go`: `GatherSystemInfo(dotfilesRoot())` — check if `GatherSystemInfo` still needs it
- `cmd/install.go` and `cmd/uninstall.go`: `NewExecutor(dotfilesRoot())` — executor uses it for bash bridge, which is now dead code

If `GatherSystemInfo` doesn't need the dotfiles path anymore, remove the parameter. If `NewExecutor` doesn't need it (all components are Go), remove the parameter and the bash bridge.

- [ ] **Step 3: Clean up executor bash bridge**

Since all 35 components have Go installers, `executor.runBash()` is dead code. However, removing it changes the executor's interface. Check if any tests rely on it. If only `executor_test.go` uses it, update the tests too.

Keep the bash bridge if it provides useful fallback capability, or remove it if all components are Go. The conservative choice is to keep it but note it's unused.

- [ ] **Step 4: Update CLAUDE.md**

Remove `cmds/` and `lib/` from the directory structure section:

```
ctdev/                 Go module root
  cmd/                 Cobra command handlers
  component/           Component registry, installers, and embedded config files
    configs/           Config files deployed by installers (go:embed)
  gpu/                 GPU/NVIDIA signing management
  platform/            OS/arch detection
  setup/               System settings (Linux dconf/GRUB, macOS defaults)
    configs/           Setup config files (go:embed)
  state/               Install markers and XDG state
  sysutil/             System utilities (packages, downloads, deploy, exec)
  tui/                 Bubble Tea UI models
  internal/shell/      Shell execution wrapper
```

- [ ] **Step 5: Build, vet, test**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev
go vet ./... && go build ./... && go test ./...
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "chore: delete all remaining bash, fully Go-native"
```

---

## Task 9: Add release workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Create release workflow**

Create `.github/workflows/release.yml`:

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  release:
    name: Build and Release
    runs-on: ubuntu-latest

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: ctdev/go.mod

      - name: Get version from tag
        id: version
        run: echo "VERSION=${GITHUB_REF_NAME#v}" >> "$GITHUB_OUTPUT"

      - name: Build binaries
        working-directory: ctdev
        run: |
          VERSION=${{ steps.version.outputs.VERSION }}
          LDFLAGS="-X main.version=$VERSION"

          GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o ../dist/ctdev-linux-amd64 .
          GOOS=linux GOARCH=arm64 go build -ldflags "$LDFLAGS" -o ../dist/ctdev-linux-arm64 .
          GOOS=darwin GOARCH=amd64 go build -ldflags "$LDFLAGS" -o ../dist/ctdev-darwin-amd64 .
          GOOS=darwin GOARCH=arm64 go build -ldflags "$LDFLAGS" -o ../dist/ctdev-darwin-arm64 .

      - name: Create release
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          gh release create "$GITHUB_REF_NAME" \
            --title "ctdev $GITHUB_REF_NAME" \
            --generate-notes \
            dist/ctdev-linux-amd64 \
            dist/ctdev-linux-arm64 \
            dist/ctdev-darwin-amd64 \
            dist/ctdev-darwin-arm64
```

Binary names match what `install.sh` expects: `ctdev-{linux,darwin}-{amd64,arm64}`.

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "feat: add GitHub Actions release workflow"
```
