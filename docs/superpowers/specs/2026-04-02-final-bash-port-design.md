# Final Bash-to-Go Port + Release Workflow — Design Spec

## Overview

Port the last 4 bash scripts to Go, delete `cmds/` and `lib/` entirely, and add a GitHub Actions release workflow. After this, the entire codebase is Go-native with zero bash dependencies.

## 1. GPU Signing Package (`ctdev/gpu/`)

Port `lib/gpu.sh` (~614 lines) and `cmds/gpu.sh` (~446 lines) into a new `ctdev/gpu/` package.

### File structure

**`ctdev/gpu/detect.go`** — Read-only detection functions (no system mutations):

```go
func IsSecureBootEnabled() bool        // mokutil --sb-state
func IsNvidiaLoaded() bool             // lsmod | grep nvidia
func GetDriverVersion() string         // nvidia-smi or modinfo
func DetectVariant() string            // "open", "closed", "unknown"
func MOKKeyExists() bool               // stat /var/lib/shim-signed/mok/MOK.{priv,der}
func MOKKeyEnrolled() bool             // mokutil --list-enrolled, compare fingerprint
func DKMSSigningConfigured() bool      // check framework.conf entries
func ModuleSignatureValid() bool       // modinfo signer matches MOK fingerprint
func FindMOKClutter() []string         // files in MOK dir that aren't MOK.priv/MOK.der
func GetRenderingBackend() string      // glxinfo or "unknown"
func GetModuleSigner() string          // modinfo nvidia | grep signer
```

Constants:
```go
const (
    MOKDir            = "/var/lib/shim-signed/mok"
    MOKPriv           = MOKDir + "/MOK.priv"
    MOKCert           = MOKDir + "/MOK.der"
    DKMSFrameworkConf = "/etc/dkms/framework.conf"
    DKMSConfDir       = "/etc/dkms/framework.conf.d"
)
```

**`ctdev/gpu/setup.go`** — Mutating actions for GPU setup:

```go
type Opts struct {
    Stdout  io.Writer
    DryRun  bool
    Force   bool
    Verbose bool
}

func RunSetup(opts Opts) error        // Full setup flow (create keys, configure DKMS, enroll MOK, rebuild)
func RunRecover(opts Opts) error      // Re-enroll MOK after CMOS reset
func CreateMOKKeypair(opts Opts) error
func ConfigureDKMSFramework(opts Opts) error
func EnrollMOK(opts Opts) error       // mokutil --import (needs stdin passthrough)
func RebuildDKMS(opts Opts) error     // dkms autoinstall
func CleanMOKClutter(opts Opts) error
```

`EnrollMOK` must pass through stdin for the mokutil password prompt. Use `cmd.Stdin = os.Stdin`.

**`ctdev/gpu/info.go`** — Info display:

```go
type StatusCheck struct {
    Name   string
    Status bool   // pass/fail
    Detail string // human-readable detail
}

func GatherStatus() []StatusCheck     // Run all detection checks, return structured results
```

The current `cmd/gpu.go` `runGPUInfo` function's display logic moves here as a pure data function. The Cobra command just prints the results.

### cmd/gpu.go changes

Becomes a thin wrapper:
- `runGPUInfo` calls `gpu.GatherStatus()` and renders with lipgloss
- `runGPUSetup` calls `gpu.RunSetup(gpu.Opts{...})`
- `runGPUSetup` with `--recover` calls `gpu.RunRecover(gpu.Opts{...})`

Remove all bash script delegation (`exec.Command("bash", script)`).

## 2. Setup Reset (`ctdev/setup/reset.go`)

Port `linux_mint_reset()` (~95 lines of bash) into native Go.

**`ctdev/setup/reset.go`:**

```go
func ResetLinuxDefaults(dryRun bool) error
```

This function:
1. Resets dconf keys: power, screensaver, keyboard, mouse, sound, nemo settings
2. Resets gsettings keys: keyboard repeat/delay/interval/numlock
3. Runs `xset r rate` to reset X key repeat
4. Removes wireplumber LDAC config
5. Stops xbindkeys, removes autostart and config
6. Resets GRUB settings (timeout, menu, os-prober)
7. Resets NVIDIA suspend GRUB params and disables services (if nvidia loaded)
8. Removes wifi suspend hook
9. Disables fstrim timer
10. Runs update-grub

Each step uses `exec.Command` with appropriate sudo where needed. Uses existing `setup` package helpers where they exist (like `applyGrubVar`).

## 3. macOS Setup (`ctdev/setup/macos.go`)

Port `macos_apply()` (~60 lines) into native Go.

**`ctdev/setup/macos.go`:**

```go
func ApplyMacOSDefaults(dryRun bool) error
```

This function runs `defaults write` commands for:
- Dock: autohide, disable launch animations, hide recent apps
- Sound: beep feedback
- Finder: path bar, status bar, no .DS_Store on network/USB, search scope, list view, quit menu
- Keyboard: disable smart quotes/dashes/spelling/capitalization/period, key repeat rates
- Dialogs: expand save/print panels
- Security: require password after sleep
- Then: `killall Dock`, `killall Finder`

## 4. Release Workflow

**`.github/workflows/release.yml`:**

Triggered on push of `v*` tags.

Steps:
1. Checkout code
2. Set up Go (from `ctdev/go.mod`)
3. Build 4 binaries with ldflags:
   - `ctdev-linux-amd64`
   - `ctdev-linux-arm64`
   - `ctdev-darwin-amd64`
   - `ctdev-darwin-arm64`
4. Create GitHub release using `gh release create`
5. Upload the 4 binaries as release assets

Binary names must match what `install.sh` expects: `ctdev-{os}-{arch}`.

## 5. Delete Remaining Bash

After all ports are complete:
- Delete `cmds/setup.sh`, `cmds/gpu.sh`
- Delete `lib/utils.sh`, `lib/gpu.sh`
- Delete `cmds/` and `lib/` directories
- Remove `dotfilesRoot()` from `cmd/info.go` if no longer referenced
- Remove `DotfilesRoot` variable from `setup/` package if no longer needed (check if `applyNvidiaSigning` or other setup functions still use it)
- Update CLAUDE.md directory structure to remove `cmds/` and `lib/`

## 6. Testing

- `gpu/detect.go`: Test parsing of `mokutil`, `lsmod`, `modinfo` output with sample data (extract parsers as pure functions)
- `gpu/info.go`: Test `GatherStatus` produces correct check structure
- `setup/reset.go`: Test dry-run outputs expected messages
- `setup/macos.go`: Test dry-run outputs expected messages
- Release workflow: tested by pushing a tag

## Out of Scope

- GPU driver installation (ctdev only handles signing)
- macOS GPU support (not applicable)
- Windows support
