# Go Helper Library + First Component Migrations

## Summary

Create a shared Go helper library (`ctdev/sysutil/`) for package management, system operations, and file downloads. Migrate the 6 simplest components from bash scripts to pure Go using `GoInstall`/`GoUninstall` functions, establishing patterns for the remaining 29 components.

## Scope

- New `ctdev/sysutil/` package with shared helpers
- Migrate 6 components: jq, shellcheck, tmux, btop, earlyoom, solaar
- Small executor change to support `ErrUnsupportedOS` sentinel for Go components
- Bash scripts remain in `components/` as reference but are unwired from the registry

## Sentinel Error for OS Skip

The executor currently handles exit code 2 (unsupported OS) only for bash scripts. For Go components, define a sentinel error:

```go
// component/component.go
var ErrUnsupportedOS = errors.New("unsupported OS")
```

Update `executor.go` Install/Uninstall to check for this:
```go
if c.GoInstall != nil {
    result.Err = c.GoInstall(ctx, opts)
    if errors.Is(result.Err, ErrUnsupportedOS) {
        result.Skipped = true
        result.Err = nil
    }
    return result
}
```

Linux-only components return `ErrUnsupportedOS` on macOS.

## Package: `ctdev/sysutil/`

All helper functions accept an `io.Writer` for output so the progress TUI can capture messages. Functions also accept a `dryRun` bool where applicable.

### `sysutil/pm.go` — Package Manager Helpers

```go
// Opts controls behavior of package manager operations.
type Opts struct {
    Stdout io.Writer // output destination (progress TUI captures this)
    DryRun bool      // if true, print what would happen but don't execute
}

// InstallPackage installs one or more packages using the system package manager.
func InstallPackage(o Opts, names ...string) error

// RemovePackage removes one or more packages.
func RemovePackage(o Opts, names ...string) error

// IsPackageInstalled checks if a package is installed via the system package manager.
func IsPackageInstalled(name string) bool
```

Implementation details:
- apt: `sudo apt-get install -y -qq <names...>` (stdout/stderr to o.Stdout)
- brew: `brew install <names...>`
- dnf: `sudo dnf install -y <names...>`
- Returns error if package manager is unsupported
- `IsPackageInstalled`: uses `dpkg -s` (apt), `brew list` (brew), `rpm -q` (dnf)
- When `o.DryRun` is true, prints the command that would run and returns nil

### `sysutil/sys.go` — System Helpers

```go
// CommandExists checks if a command is available on PATH.
func CommandExists(name string) bool

// ServiceEnable enables a systemd service. Requires sudo.
func ServiceEnable(o Opts, name string) error

// ServiceDisable stops and disables a systemd service. Requires sudo.
func ServiceDisable(o Opts, name string) error

// ServiceStart starts a systemd service. Requires sudo.
func ServiceStart(o Opts, name string) error

// SafeSymlink creates a symlink, removing existing file/link at dst first.
func SafeSymlink(src, dst string) error
```

### `sysutil/download.go` — Download Helpers

```go
// DownloadFile downloads a URL to a local file path.
func DownloadFile(url, dest string) error
```

Not needed for the first 6 components but included for Tier 2+ migrations.

### `sysutil/exec.go` — Execution Helpers

```go
// Run executes a command, routing output to the given writer.
func Run(o Opts, name string, args ...string) error

// SudoRun executes a command with sudo prefix.
func SudoRun(o Opts, name string, args ...string) error
```

These are the primitives that `InstallPackage`, `ServiceEnable`, etc. build on.

## Component Migration Pattern

Each migrated component gets a Go file in the `component/` package. Components receive `ExecOpts` which has `Stdout`, `Stderr`, `DryRun`, `Force`, `Verbose` — they construct `sysutil.Opts` from it.

```go
// component/install_jq.go
package component

import (
    "context"
    "fmt"
    "github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func jqInstall(ctx context.Context, opts ExecOpts) error {
    o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
    if !opts.Force && sysutil.CommandExists("jq") {
        fmt.Fprintln(opts.Stdout, "jq already installed")
        return nil
    }
    return sysutil.InstallPackage(o, "jq")
}

func jqUninstall(ctx context.Context, opts ExecOpts) error {
    o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
    return sysutil.RemovePackage(o, "jq")
}
```

Registry entry changes from:
```go
{Name: "jq", ..., BashInstall: "components/jq/install.sh", BashUninstall: "components/jq/uninstall.sh"}
```
To:
```go
{Name: "jq", ..., GoInstall: jqInstall, GoUninstall: jqUninstall}
```

## Components to Migrate

### 1. jq
- Install: `InstallPackage("jq")`
- Detect: `CommandExists("jq")`
- Uninstall: `RemovePackage("jq")`

### 2. shellcheck
- Install: `InstallPackage("shellcheck")`
- Detect: `CommandExists("shellcheck")`
- Uninstall: `RemovePackage("shellcheck")`

### 3. tmux
- Install: `InstallPackage("tmux")`
- Detect: `CommandExists("tmux")`
- Uninstall: `RemovePackage("tmux")`

### 4. btop
- Install: `InstallPackage("btop")`
- Detect: `CommandExists("btop")`
- Uninstall: `RemovePackage("btop")`

### 5. earlyoom
- Linux only (return `ErrUnsupportedOS` on macOS)
- Install: `InstallPackage("earlyoom")` + `ServiceEnable("earlyoom")` + `ServiceStart("earlyoom")`
- Detect: `CommandExists("earlyoom")`
- Uninstall: `ServiceDisable("earlyoom")` + `RemovePackage("earlyoom")`

### 6. solaar
- Linux only (return `ErrUnsupportedOS` on macOS)
- Install: `InstallPackage("solaar")`
- Detect: `CommandExists("solaar")`
- Uninstall: `RemovePackage("solaar")`

## File Structure

### New Files
| File | Responsibility |
|------|---------------|
| `ctdev/sysutil/opts.go` | Opts type shared across sysutil |
| `ctdev/sysutil/exec.go` | Run and SudoRun helpers |
| `ctdev/sysutil/pm.go` | Package manager install/remove/check |
| `ctdev/sysutil/pm_test.go` | Tests for package manager helpers |
| `ctdev/sysutil/sys.go` | System helpers (command exists, systemd, symlink) |
| `ctdev/sysutil/sys_test.go` | Tests for system helpers |
| `ctdev/sysutil/download.go` | HTTP download helper |
| `ctdev/component/install_jq.go` | jq Go install/uninstall |
| `ctdev/component/install_shellcheck.go` | shellcheck Go install/uninstall |
| `ctdev/component/install_tmux.go` | tmux Go install/uninstall |
| `ctdev/component/install_btop.go` | btop Go install/uninstall |
| `ctdev/component/install_earlyoom.go` | earlyoom Go install/uninstall |
| `ctdev/component/install_solaar.go` | solaar Go install/uninstall |

### Modified Files
| File | Changes |
|------|---------|
| `ctdev/component/component.go` | Add `ErrUnsupportedOS` sentinel |
| `ctdev/component/executor.go` | Check `ErrUnsupportedOS` in GoInstall/GoUninstall path |
| `ctdev/component/registry.go` | Switch 6 components from BashInstall/BashUninstall to GoInstall/GoUninstall |

## Testing Strategy

- `sysutil/pm_test.go`: Test `IsPackageInstalled` with dpkg mock or known state. Test dry-run mode prints command without executing.
- `sysutil/sys_test.go`: Test `CommandExists` with known commands ("go" exists, "nonexistent-xyz" doesn't). Test `SafeSymlink` with temp directories.
- `component/executor_test.go`: Add test for `ErrUnsupportedOS` → `Skipped: true` mapping.
- Component install functions: Integration-tested by running `ctdev install jq --dry-run` and verifying output.

## Key Decisions

- **Package name `sysutil`** not `pkg`: More descriptive, follows Go naming conventions.
- **`Opts` struct for output routing**: All helpers accept `Opts{Stdout, DryRun}` so output flows to the progress TUI and dry-run is respected throughout.
- **Thin wrappers, not abstractions**: `InstallPackage` detects the package manager internally. Components don't need to know which one.
- **One file per component**: Keeps migrations isolated and reviewable.
- **Bash scripts kept as reference**: Not deleted, just unwired from registry.
- **Executor change required**: Small addition of `ErrUnsupportedOS` check in the GoInstall path to preserve skip semantics.
- **earlyoom uninstall stops service**: `ServiceDisable` before `RemovePackage` to ensure clean removal.
