# Go Helper Library + Component Migration Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create shared Go helpers for system operations and migrate components from bash to pure Go.

**Architecture:** A `sysutil` package provides thin wrappers around package managers, systemd, filesystem ops, GitHub releases, APT repo management, and checksum verification. Each component gets a Go file with install/uninstall functions. The executor handles `ErrUnsupportedOS` for Go components.

**Tech Stack:** Go, `os/exec`, `platform.Detect()` for OS/package manager detection

## Status

**Completed (22/35 components migrated):**
- Chunk 1: sysutil package + ErrUnsupportedOS executor fix
- Chunk 2: jq, shellcheck, tmux, btop, earlyoom, solaar
- Chunk 3: chatgpt, claude-desktop, linear, logi-options, cleanmymac (macOS-only brew casks)
- Chunk 4: 1password, gh, slack, vscode, terraform (APT repo setup components)
- Chunk 5: age, doctl, git-spice, helm, kubectl, sops (GitHub release downloads)
- Bonus: helm update version detection fix (major version awareness)
- Bonus: platform.Info.Codename field for APT repo URLs

**Remaining on bash (13 components):**

Medium (convertible with more work):
- bun — brew or curl installer script
- chrome — brew cask or multi-distro deb/dnf
- claude-code — brew or npm + config symlinks
- codex — brew cask or npm
- dbeaver — brew cask or multi-distro repo setup
- docker — brew cask or repo setup + usermod + systemctl
- ghostty — brew cask or community installer + config symlink
- tailscale — brew cask or apt repo + systemctl

Complex (leave as bash):
- node — nodenv version manager, npm globals, eval hooks
- ruby — rbenv version manager, build deps, gems, eval hooks
- zsh — Oh My Zsh, Pure prompt, git repos, chsh (also used by devcontainer.sh)
- git — interactive user config prompts
- fonts — custom nerd_fonts.sh script

## sysutil Package Reference

| File | Helpers |
|------|---------|
| `opts.go` | `Opts{Stdout, DryRun}` |
| `exec.go` | `Run`, `SudoRun` |
| `pm.go` | `InstallPackage`, `RemovePackage`, `IsPackageInstalled`, `BrewCaskInstall`, `BrewCaskRemove` |
| `sys.go` | `CommandExists`, `ServiceEnable`, `ServiceDisable`, `ServiceStart`, `SafeSymlink` |
| `download.go` | `DownloadFile`, `GitHubLatestVersion`, `VerifyChecksumFile`, `VerifyChecksum`, `InstallBinary` |
| `apt.go` | `AddAPTKeyring`, `AddAPTSource`, `APTUpdate` |

**Spec:** `docs/superpowers/specs/2026-03-14-go-helpers-migration-design.md`

---

## File Structure

### New Files
| File | Responsibility |
|------|---------------|
| `ctdev/sysutil/opts.go` | Opts type (Stdout, DryRun) |
| `ctdev/sysutil/exec.go` | Run/SudoRun command execution helpers |
| `ctdev/sysutil/pm.go` | InstallPackage/RemovePackage/IsPackageInstalled |
| `ctdev/sysutil/pm_test.go` | Tests for package manager helpers |
| `ctdev/sysutil/sys.go` | CommandExists, ServiceEnable/Disable/Start, SafeSymlink |
| `ctdev/sysutil/sys_test.go` | Tests for system helpers |
| `ctdev/sysutil/download.go` | DownloadFile HTTP helper |
| `ctdev/component/install_jq.go` | jq install/uninstall |
| `ctdev/component/install_shellcheck.go` | shellcheck install/uninstall |
| `ctdev/component/install_tmux.go` | tmux install/uninstall |
| `ctdev/component/install_btop.go` | btop install/uninstall |
| `ctdev/component/install_earlyoom.go` | earlyoom install/uninstall |
| `ctdev/component/install_solaar.go` | solaar install/uninstall |

### Modified Files
| File | Changes |
|------|---------|
| `ctdev/component/component.go` | Add `ErrUnsupportedOS` sentinel error |
| `ctdev/component/executor.go` | Handle `ErrUnsupportedOS` in GoInstall/GoUninstall paths |
| `ctdev/component/executor_test.go` | Add test for ErrUnsupportedOS → Skipped mapping |
| `ctdev/component/registry.go` | Switch 6 components to GoInstall/GoUninstall |

---

## Chunk 1: Sysutil Package + Executor Fix

### Task 1: Opts and Exec Helpers

**Files:**
- Create: `ctdev/sysutil/opts.go`
- Create: `ctdev/sysutil/exec.go`

- [ ] **Step 1: Create opts.go**

```go
package sysutil

import "io"

// Opts controls behavior of sysutil operations.
type Opts struct {
	Stdout io.Writer // output destination (progress TUI captures this)
	DryRun bool      // print what would happen but don't execute
}
```

- [ ] **Step 2: Create exec.go**

```go
package sysutil

import (
	"fmt"
	"os/exec"
)

// Run executes a command, routing output to opts.Stdout.
// If opts.DryRun, prints the command without executing.
func Run(o Opts, name string, args ...string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] %s %s\n", name, joinArgs(args))
		return nil
	}
	cmd := exec.Command(name, args...)
	cmd.Stdout = o.Stdout
	cmd.Stderr = o.Stdout
	return cmd.Run()
}

// SudoRun executes a command with sudo.
func SudoRun(o Opts, name string, args ...string) error {
	sudoArgs := append([]string{name}, args...)
	return Run(o, "sudo", sudoArgs...)
}

func joinArgs(args []string) string {
	result := ""
	for i, a := range args {
		if i > 0 {
			result += " "
		}
		result += a
	}
	return result
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go build ./sysutil/`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add ctdev/sysutil/opts.go ctdev/sysutil/exec.go
git commit -m "feat: add sysutil opts and exec helpers"
```

### Task 2: Package Manager Helpers

**Files:**
- Create: `ctdev/sysutil/pm.go`
- Create: `ctdev/sysutil/pm_test.go`

- [ ] **Step 1: Write pm_test.go**

```go
package sysutil

import (
	"bytes"
	"testing"
)

func TestInstallPackageDryRun(t *testing.T) {
	var buf bytes.Buffer
	o := Opts{Stdout: &buf, DryRun: true}
	err := InstallPackage(o, "testpkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if out == "" {
		t.Error("expected dry-run output, got empty")
	}
}

func TestRemovePackageDryRun(t *testing.T) {
	var buf bytes.Buffer
	o := Opts{Stdout: &buf, DryRun: true}
	err := RemovePackage(o, "testpkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if out == "" {
		t.Error("expected dry-run output, got empty")
	}
}

func TestIsPackageInstalledKnown(t *testing.T) {
	// "go" should be installed since we're running Go tests
	if !CommandExists("go") {
		t.Skip("go not on PATH")
	}
}

func TestIsPackageInstalledUnknown(t *testing.T) {
	if CommandExists("nonexistent-package-xyz-12345") {
		t.Error("expected nonexistent command to not exist")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go test ./sysutil/ -v -run TestInstall`
Expected: FAIL — functions not defined

- [ ] **Step 3: Write pm.go**

```go
package sysutil

import (
	"fmt"
	"os/exec"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

// InstallPackage installs packages using the detected system package manager.
func InstallPackage(o Opts, names ...string) error {
	pm := platform.Detect().PackageManager
	switch pm {
	case "apt":
		return SudoRun(o, "apt-get", append([]string{"install", "-y", "-qq"}, names...)...)
	case "brew":
		return Run(o, "brew", append([]string{"install"}, names...)...)
	case "dnf":
		return SudoRun(o, "dnf", append([]string{"install", "-y"}, names...)...)
	case "pacman":
		return SudoRun(o, "pacman", append([]string{"-S", "--noconfirm"}, names...)...)
	default:
		return fmt.Errorf("unsupported package manager: %s", pm)
	}
}

// RemovePackage removes packages using the detected system package manager.
func RemovePackage(o Opts, names ...string) error {
	pm := platform.Detect().PackageManager
	switch pm {
	case "apt":
		return SudoRun(o, "apt-get", append([]string{"remove", "-y"}, names...)...)
	case "brew":
		return Run(o, "brew", append([]string{"uninstall"}, names...)...)
	case "dnf":
		return SudoRun(o, "dnf", append([]string{"remove", "-y"}, names...)...)
	case "pacman":
		return SudoRun(o, "pacman", append([]string{"-R", "--noconfirm"}, names...)...)
	default:
		return fmt.Errorf("unsupported package manager: %s", pm)
	}
}

// IsPackageInstalled checks if a package is installed via the system package manager.
func IsPackageInstalled(name string) bool {
	pm := platform.Detect().PackageManager
	switch pm {
	case "apt":
		return exec.Command("dpkg", "-s", name).Run() == nil
	case "brew":
		return exec.Command("brew", "list", name).Run() == nil
	case "dnf":
		return exec.Command("rpm", "-q", name).Run() == nil
	default:
		return false
	}
}
```

- [ ] **Step 4: Run tests**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go test ./sysutil/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add ctdev/sysutil/pm.go ctdev/sysutil/pm_test.go
git commit -m "feat: add package manager helpers"
```

### Task 3: System Helpers

**Files:**
- Create: `ctdev/sysutil/sys.go`
- Create: `ctdev/sysutil/sys_test.go`

- [ ] **Step 1: Write sys_test.go**

```go
package sysutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommandExistsTrue(t *testing.T) {
	if !CommandExists("go") {
		t.Error("expected 'go' to exist on PATH")
	}
}

func TestCommandExistsFalse(t *testing.T) {
	if CommandExists("nonexistent-cmd-xyz-99999") {
		t.Error("expected nonexistent command to return false")
	}
}

func TestSafeSymlinkCreates(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	dst := filepath.Join(dir, "link.txt")
	os.WriteFile(src, []byte("hello"), 0644)

	if err := SafeSymlink(src, dst); err != nil {
		t.Fatalf("SafeSymlink failed: %v", err)
	}

	target, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("Readlink failed: %v", err)
	}
	if target != src {
		t.Errorf("expected link to %s, got %s", src, target)
	}
}

func TestSafeSymlinkReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	dst := filepath.Join(dir, "link.txt")
	os.WriteFile(src, []byte("hello"), 0644)
	os.WriteFile(dst, []byte("old"), 0644) // existing file at dst

	if err := SafeSymlink(src, dst); err != nil {
		t.Fatalf("SafeSymlink failed: %v", err)
	}

	target, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("Readlink failed: %v", err)
	}
	if target != src {
		t.Errorf("expected link to %s, got %s", src, target)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go test ./sysutil/ -v -run TestCommand`
Expected: FAIL

- [ ] **Step 3: Write sys.go**

```go
package sysutil

import (
	"os"
	"os/exec"
	"path/filepath"
)

// CommandExists checks if a command is available on PATH.
func CommandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// ServiceEnable enables a systemd service.
func ServiceEnable(o Opts, name string) error {
	return SudoRun(o, "systemctl", "enable", name+".service")
}

// ServiceDisable stops and disables a systemd service.
func ServiceDisable(o Opts, name string) error {
	_ = SudoRun(o, "systemctl", "stop", name+".service")
	return SudoRun(o, "systemctl", "disable", name+".service")
}

// ServiceStart starts a systemd service.
func ServiceStart(o Opts, name string) error {
	return SudoRun(o, "systemctl", "start", name+".service")
}

// SafeSymlink creates a symlink at dst pointing to src.
// Removes any existing file or symlink at dst first.
func SafeSymlink(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	os.Remove(dst)
	return os.Symlink(src, dst)
}
```

- [ ] **Step 4: Run tests**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go test ./sysutil/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add ctdev/sysutil/sys.go ctdev/sysutil/sys_test.go
git commit -m "feat: add system helpers (command exists, systemd, symlink)"
```

### Task 4: Download Helper

**Files:**
- Create: `ctdev/sysutil/download.go`

- [ ] **Step 1: Create download.go**

```go
package sysutil

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

// DownloadFile downloads a URL to a local file path.
func DownloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go build ./sysutil/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add ctdev/sysutil/download.go
git commit -m "feat: add HTTP download helper"
```

### Task 5: ErrUnsupportedOS in Executor

**Files:**
- Modify: `ctdev/component/component.go`
- Modify: `ctdev/component/executor.go`
- Modify: `ctdev/component/executor_test.go`

- [ ] **Step 1: Add test for ErrUnsupportedOS**

Add to `ctdev/component/executor_test.go`:

```go
func TestExecutorGoInstallUnsupportedOS(t *testing.T) {
	exec := NewExecutor(t.TempDir())

	c := &Component{
		Name: "test-skip",
		GoInstall: func(ctx context.Context, opts ExecOpts) error {
			return ErrUnsupportedOS
		},
	}

	result := exec.Install(context.Background(), c, ExecOpts{})
	if !result.Skipped {
		t.Error("expected Skipped=true for ErrUnsupportedOS")
	}
	if result.Err != nil {
		t.Errorf("expected Err=nil for skipped, got %v", result.Err)
	}
}

func TestExecutorGoUninstallUnsupportedOS(t *testing.T) {
	exec := NewExecutor(t.TempDir())

	c := &Component{
		Name: "test-skip",
		GoUninstall: func(ctx context.Context, opts ExecOpts) error {
			return ErrUnsupportedOS
		},
	}

	result := exec.Uninstall(context.Background(), c, ExecOpts{})
	if !result.Skipped {
		t.Error("expected Skipped=true for ErrUnsupportedOS")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go test ./component/ -v -run TestExecutorGo.*Unsupported`
Expected: FAIL — ErrUnsupportedOS not defined

- [ ] **Step 3: Add ErrUnsupportedOS to component.go**

Add to `ctdev/component/component.go` after the imports:

```go
import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
)

// ErrUnsupportedOS is returned by GoInstall/GoUninstall when a component
// doesn't support the current OS. The executor maps this to Skipped.
var ErrUnsupportedOS = errors.New("unsupported OS")
```

- [ ] **Step 4: Update executor.go Install method**

Replace the GoInstall block in `executor.go` Install:

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

Add `"errors"` to executor.go imports.

- [ ] **Step 5: Update executor.go Uninstall method**

Same pattern for GoUninstall block:

```go
if c.GoUninstall != nil {
	result.Err = c.GoUninstall(ctx, opts)
	if errors.Is(result.Err, ErrUnsupportedOS) {
		result.Skipped = true
		result.Err = nil
	}
	return result
}
```

- [ ] **Step 6: Run tests**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go test ./component/ -v`
Expected: All pass including new ErrUnsupportedOS tests

- [ ] **Step 7: Commit**

```bash
git add ctdev/component/component.go ctdev/component/executor.go ctdev/component/executor_test.go
git commit -m "feat: add ErrUnsupportedOS sentinel for Go components"
```

---

## Chunk 2: Migrate 6 Components

### Task 6: Migrate jq, shellcheck, tmux, btop

**Files:**
- Create: `ctdev/component/install_jq.go`
- Create: `ctdev/component/install_shellcheck.go`
- Create: `ctdev/component/install_tmux.go`
- Create: `ctdev/component/install_btop.go`
- Modify: `ctdev/component/registry.go`

These 4 components share an identical pattern: install/remove a single package.

- [ ] **Step 1: Create install_jq.go**

```go
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
	fmt.Fprintln(opts.Stdout, "Installing jq...")
	return sysutil.InstallPackage(o, "jq")
}

func jqUninstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing jq...")
	return sysutil.RemovePackage(o, "jq")
}
```

- [ ] **Step 2: Create install_shellcheck.go**

Same pattern as jq but with "shellcheck" package name.

- [ ] **Step 3: Create install_tmux.go**

Same pattern with "tmux".

- [ ] **Step 4: Create install_btop.go**

Same pattern with "btop".

- [ ] **Step 5: Update registry.go for these 4 components**

For each of jq, shellcheck, tmux, btop: replace `BashInstall`/`BashUninstall` with `GoInstall`/`GoUninstall`. Example for jq:

```go
{Name: "jq", Description: "JSON processor", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: jqInstall, GoUninstall: jqUninstall, Tags: []string{"json", "parser"}},
```

Remove the `BashInstall` and `BashUninstall` fields for these 4 components.

- [ ] **Step 6: Verify build**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go build -ldflags "-X main.dotfilesRoot=/home/thomas/Repos/github.com/ConnerTechnology/dotfiles" -o ctdev .`
Expected: No errors

- [ ] **Step 7: Commit**

```bash
git add ctdev/component/install_jq.go ctdev/component/install_shellcheck.go ctdev/component/install_tmux.go ctdev/component/install_btop.go ctdev/component/registry.go
git commit -m "feat: migrate jq, shellcheck, tmux, btop to Go"
```

### Task 7: Migrate earlyoom (Linux-only + systemd)

**Files:**
- Create: `ctdev/component/install_earlyoom.go`
- Modify: `ctdev/component/registry.go`

- [ ] **Step 1: Create install_earlyoom.go**

```go
package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func earlyoomInstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS == platform.MacOS {
		return ErrUnsupportedOS
	}
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	if !opts.Force && sysutil.CommandExists("earlyoom") {
		fmt.Fprintln(opts.Stdout, "earlyoom already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing earlyoom...")
	if err := sysutil.InstallPackage(o, "earlyoom"); err != nil {
		return err
	}
	if err := sysutil.ServiceEnable(o, "earlyoom"); err != nil {
		return err
	}
	return sysutil.ServiceStart(o, "earlyoom")
}

func earlyoomUninstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS == platform.MacOS {
		return ErrUnsupportedOS
	}
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing earlyoom...")
	_ = sysutil.ServiceDisable(o, "earlyoom")
	return sysutil.RemovePackage(o, "earlyoom")
}
```

- [ ] **Step 2: Update registry.go for earlyoom**

Replace bash paths with Go functions:

```go
{Name: "earlyoom", Description: "Early OOM killer for Linux", Category: CategorySystem, SupportedOS: []OS{OSLinux}, GoInstall: earlyoomInstall, GoUninstall: earlyoomUninstall, Tags: []string{"memory", "oom"}},
```

- [ ] **Step 3: Verify build**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go build -ldflags "-X main.dotfilesRoot=/home/thomas/Repos/github.com/ConnerTechnology/dotfiles" -o ctdev .`

- [ ] **Step 4: Commit**

```bash
git add ctdev/component/install_earlyoom.go ctdev/component/registry.go
git commit -m "feat: migrate earlyoom to Go with systemd support"
```

### Task 8: Migrate solaar (Linux-only)

**Files:**
- Create: `ctdev/component/install_solaar.go`
- Modify: `ctdev/component/registry.go`

- [ ] **Step 1: Create install_solaar.go**

```go
package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func solaarInstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS == platform.MacOS {
		return ErrUnsupportedOS
	}
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	if !opts.Force && sysutil.CommandExists("solaar") {
		fmt.Fprintln(opts.Stdout, "solaar already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing solaar...")
	return sysutil.InstallPackage(o, "solaar")
}

func solaarUninstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS == platform.MacOS {
		return ErrUnsupportedOS
	}
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing solaar...")
	return sysutil.RemovePackage(o, "solaar")
}
```

- [ ] **Step 2: Update registry.go for solaar**

```go
{Name: "solaar", Description: "Logitech Unifying/Bolt receiver manager", Category: CategorySystem, SupportedOS: []OS{OSLinux}, GoInstall: solaarInstall, GoUninstall: solaarUninstall, Tags: []string{"logitech", "bluetooth"}},
```

- [ ] **Step 3: Verify build and run all tests**

Run:
```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev
go build -ldflags "-X main.dotfilesRoot=/home/thomas/Repos/github.com/ConnerTechnology/dotfiles" -o ctdev .
go test ./...
```
Expected: All pass

- [ ] **Step 4: Commit**

```bash
git add ctdev/component/install_solaar.go ctdev/component/registry.go
git commit -m "feat: migrate solaar to Go"
```

### Task 9: Build, Install, Smoke Test

- [ ] **Step 1: Build with ldflags**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev
go build -ldflags "-X main.dotfilesRoot=/home/thomas/Repos/github.com/ConnerTechnology/dotfiles" -o ctdev .
```

- [ ] **Step 2: Install**

```bash
command cp -f ./ctdev /home/thomas/.local/bin/ctdev
```

- [ ] **Step 3: Smoke test dry-run install**

Run: `ctdev install jq --dry-run`
Expected: Shows dry-run output from Go install function, no actual package installation

- [ ] **Step 4: Smoke test dry-run uninstall**

Run: `ctdev uninstall jq --dry-run`
Expected: Shows dry-run output, no actual removal
