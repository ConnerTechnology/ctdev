# ctdev Go CLI Rewrite — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite ctdev from bash to a Go CLI with rich Bubble Tea TUI, shelling out to existing bash scripts.

**Architecture:** Cobra + Viper CLI with Bubble Tea TUI layer. Component registry as Go structs with bash bridge executor. XDG-compliant state. Every command has interactive (TUI) and batch modes.

**Tech Stack:** Go 1.24+, cobra, viper, bubbletea v2 (`charm.land/bubbletea/v2`), bubbles v2 (`charm.land/bubbles/v2`), lipgloss v2 (`charm.land/lipgloss/v2`)

**Bubble Tea v2 notes:** `View()` returns `tea.View` (use `tea.NewView(s)`). Key events are `tea.KeyPressMsg` (not `tea.KeyMsg`). Import paths use `charm.land/` not `github.com/charmbracelet/`.

**Spec:** `docs/superpowers/specs/2026-03-13-go-cli-rewrite-design.md`

---

## File Structure

```
ctdev/
├── main.go                      # Entry point, version var, calls cmd.Execute()
├── go.mod
├── go.sum
├── Makefile                     # Build with ldflags version injection
├── cmd/
│   ├── root.go                  # Root cobra command, global flags, Viper init
│   ├── install.go               # Install command — picker TUI or direct install
│   ├── uninstall.go             # Uninstall command — picker filtered to installed
│   ├── update.go                # Update command — scan + checklist TUI
│   ├── setup.go                 # Setup command — wizard TUI
│   ├── info.go                  # Info command — system dashboard
│   ├── cleanup.go               # Cleanup command — task checklist
│   ├── configure.go             # Configure command — git config TUI
│   └── gpu.go                   # GPU command — info + setup
├── component/
│   ├── component.go             # Component struct, Category, OS types, ExecOpts
│   ├── component_test.go        # Tests for filtering, dependency resolution
│   ├── registry.go              # All 36 component definitions
│   ├── registry_test.go         # Registry validation tests
│   ├── executor.go              # Bash bridge + Go executor
│   └── executor_test.go         # Executor tests with test scripts
├── platform/
│   ├── detect.go                # OS, arch, package manager detection
│   ├── detect_test.go
│   └── info.go                  # Info struct + system info gathering
├── state/
│   ├── paths.go                 # XDG path helpers
│   ├── paths_test.go
│   ├── config.go                # Viper-backed user config
│   ├── config_test.go
│   ├── markers.go               # Install markers (JSON)
│   ├── markers_test.go
│   └── migrate.go               # Old marker migration
├── tui/
│   ├── styles/
│   │   └── styles.go            # Shared Lip Gloss styles
│   ├── picker/
│   │   ├── picker.go            # Multi-select component picker model
│   │   └── picker_test.go
│   ├── progress/
│   │   ├── progress.go          # Installation progress model
│   │   └── progress_test.go
│   ├── checklist/
│   │   ├── checklist.go         # Update checklist model
│   │   └── checklist_test.go
│   ├── wizard/
│   │   ├── wizard.go            # Setup wizard model
│   │   └── wizard_test.go
│   └── info/
│       └── info.go              # System info display model
└── internal/
    └── shell/
        ├── shell.go             # exec.Command helpers
        └── shell_test.go
```

---

## Chunk 1: Foundation — Go Module, Platform, State, Executor

This chunk produces a working `ctdev` binary with `ctdev --version`, `ctdev info` (basic), and the component executor that can shell out to bash scripts. No TUI yet — batch mode only.

### Task 1: Initialize Go Module

**Files:**
- Create: `ctdev/main.go`
- Create: `ctdev/go.mod`
- Create: `ctdev/Makefile`

- [ ] **Step 1: Create go.mod**

```bash
cd ctdev && go mod init github.com/ConnerTechnology/dotfiles/ctdev
```

- [ ] **Step 2: Write main.go**

```go
package main

import (
	"fmt"
	"os"

	"github.com/ConnerTechnology/dotfiles/ctdev/cmd"
)

var version = "dev"

func main() {
	cmd.SetVersion(version)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 3: Write Makefile**

```makefile
VERSION := $(shell cat ../VERSION)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build clean test

build:
	go build $(LDFLAGS) -o ctdev .

clean:
	rm -f ctdev

test:
	go test ./...
```

- [ ] **Step 4: Verify it compiles (will fail — cmd package missing, that's expected)**

Run: `cd ctdev && go build .`
Expected: compilation error about missing cmd package

- [ ] **Step 5: Commit**

```bash
git add ctdev/main.go ctdev/go.mod ctdev/Makefile
git commit -m "feat: initialize Go module for ctdev CLI"
```

### Task 2: Root Cobra Command + Global Flags

**Files:**
- Create: `ctdev/cmd/root.go`

- [ ] **Step 1: Write root.go with global flags**

```go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	version string

	flagVerbose bool
	flagDryRun  bool
	flagForce   bool
	flagBatch   bool
)

func SetVersion(v string) {
	version = v
}

var rootCmd = &cobra.Command{
	Use:   "ctdev",
	Short: "Development environment manager",
	Long:  "ctdev manages your development environment — install components, update packages, and configure your system.",
	Version: "",
}

func Execute() error {
	rootCmd.Version = version
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVarP(&flagDryRun, "dry-run", "n", false, "preview changes without applying")
	rootCmd.PersistentFlags().BoolVarP(&flagForce, "force", "f", false, "force re-run install scripts")
	rootCmd.PersistentFlags().BoolVar(&flagBatch, "batch", false, "non-interactive mode")
}

func initConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("$HOME/.config/ctdev")
	viper.AutomaticEnv()
	_ = viper.ReadInConfig()
}

func isBatchMode() bool {
	if flagBatch {
		return true
	}
	fi, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return fi.Mode()&os.ModeCharDevice == 0
}
```

- [ ] **Step 2: Add cobra and viper dependencies**

Run: `cd ctdev && go get github.com/spf13/cobra@latest github.com/spf13/viper@latest`

- [ ] **Step 3: Build and test version flag**

Run: `cd ctdev && go build -ldflags "-X main.version=test" -o ctdev . && ./ctdev --version`
Expected: `ctdev version test`

- [ ] **Step 4: Commit**

```bash
git add ctdev/cmd/root.go ctdev/go.mod ctdev/go.sum
git commit -m "feat: add root cobra command with global flags"
```

### Task 3: Platform Detection

**Files:**
- Create: `ctdev/platform/detect.go`
- Create: `ctdev/platform/detect_test.go`
- Create: `ctdev/platform/info.go`

- [ ] **Step 1: Write failing test for platform detection**

```go
package platform

import (
	"runtime"
	"testing"
)

func TestDetectOS(t *testing.T) {
	info := Detect()
	if runtime.GOOS == "linux" {
		if info.OS != Linux {
			t.Errorf("expected Linux, got %s", info.OS)
		}
		if info.PackageManager == "" {
			t.Error("expected package manager to be detected")
		}
	} else if runtime.GOOS == "darwin" {
		if info.OS != MacOS {
			t.Errorf("expected MacOS, got %s", info.OS)
		}
		if info.PackageManager != "brew" {
			t.Errorf("expected brew, got %s", info.PackageManager)
		}
	}
	if info.Arch == "" {
		t.Error("expected arch to be detected")
	}
}

func TestDetectArch(t *testing.T) {
	arch := detectArch()
	if arch != "amd64" && arch != "arm64" {
		t.Errorf("unexpected arch: %s", arch)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ctdev && go test ./platform/`
Expected: FAIL — package doesn't exist yet

- [ ] **Step 3: Write detect.go**

```go
package platform

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type OS string

const (
	Linux   OS = "linux"
	MacOS   OS = "macos"
	Unknown OS = "unknown"
)

type Info struct {
	OS             OS
	Distro         string // "linuxmint", "ubuntu", etc.
	Arch           string // "amd64", "arm64"
	PackageManager string // "apt", "dnf", "pacman", "brew"
	IsContainer    bool
}

func Detect() Info {
	info := Info{
		OS:   detectOS(),
		Arch: detectArch(),
	}
	info.Distro = detectDistro()
	info.PackageManager = detectPackageManager(info.OS)
	info.IsContainer = detectContainer()
	return info
}

func detectOS() OS {
	switch runtime.GOOS {
	case "linux":
		return Linux
	case "darwin":
		return MacOS
	default:
		return Unknown
	}
}

func detectArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	default:
		return runtime.GOARCH
	}
}

func detectDistro() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "ID=") {
			return strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		}
	}
	return ""
}

func detectPackageManager(osType OS) string {
	if osType == MacOS {
		return "brew"
	}
	managers := []struct {
		cmd  string
		name string
	}{
		{"apt", "apt"},
		{"dnf", "dnf"},
		{"pacman", "pacman"},
	}
	for _, m := range managers {
		if _, err := exec.LookPath(m.cmd); err == nil {
			return m.name
		}
	}
	return "unknown"
}

func detectContainer() bool {
	if os.Getenv("REMOTE_CONTAINERS") != "" || os.Getenv("CODESPACES") != "" {
		return true
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	return false
}
```

- [ ] **Step 4: Run tests**

Run: `cd ctdev && go test ./platform/ -v`
Expected: PASS

- [ ] **Step 5: Write info.go for system info gathering**

```go
package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type SystemInfo struct {
	Platform    Info
	Hostname    string
	Shell       string
	DotfilesDir string
	CPUModel    string
	CPUThreads  int
	MemoryGB    int
}

func GatherSystemInfo(dotfilesDir string) SystemInfo {
	info := SystemInfo{
		Platform:    Detect(),
		DotfilesDir: dotfilesDir,
		CPUThreads:  runtime.NumCPU(),
	}
	info.Hostname, _ = os.Hostname()
	info.Shell = filepath.Base(os.Getenv("SHELL"))
	info.CPUModel = readCPUModel()
	info.MemoryGB = readMemoryGB()
	return info
}

func readCPUModel() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		if err != nil {
			return "unknown"
		}
		return strings.TrimSpace(string(out))
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "unknown"
}

func readMemoryGB() int {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0
		}
		var bytes int
		fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &bytes)
		return snapToStandardSize(bytes / (1024 * 1024 * 1024))
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			var kb int
			fmt.Sscanf(line, "MemTotal: %d kB", &kb)
			return snapToStandardSize(kb / (1024 * 1024))
		}
	}
	return 0
}

func snapToStandardSize(gb int) int {
	sizes := []int{4, 8, 16, 32, 64, 128, 256}
	for _, s := range sizes {
		if gb <= s {
			return s
		}
	}
	return gb
}
```

- [ ] **Step 6: Commit**

```bash
git add ctdev/platform/
git commit -m "feat: add platform detection and system info"
```

### Task 4: XDG State — Paths, Config, Markers

**Files:**
- Create: `ctdev/state/paths.go`
- Create: `ctdev/state/paths_test.go`
- Create: `ctdev/state/config.go`
- Create: `ctdev/state/config_test.go`
- Create: `ctdev/state/markers.go`
- Create: `ctdev/state/markers_test.go`
- Create: `ctdev/state/migrate.go`

- [ ] **Step 1: Write failing test for XDG paths**

```go
package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDir(t *testing.T) {
	dir := ConfigDir()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".config", "ctdev")
	if dir != expected {
		t.Errorf("expected %s, got %s", expected, dir)
	}
}

func TestStateDir(t *testing.T) {
	dir := StateDir()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".local", "state", "ctdev")
	if dir != expected {
		t.Errorf("expected %s, got %s", expected, dir)
	}
}

func TestCacheDir(t *testing.T) {
	dir := CacheDir()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".cache", "ctdev")
	if dir != expected {
		t.Errorf("expected %s, got %s", expected, dir)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ctdev && go test ./state/`
Expected: FAIL

- [ ] **Step 3: Write paths.go**

```go
package state

import (
	"os"
	"path/filepath"
)

func ConfigDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "ctdev")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ctdev")
}

func StateDir() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "ctdev")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "ctdev")
}

func CacheDir() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, "ctdev")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "ctdev")
}
```

- [ ] **Step 4: Run path tests**

Run: `cd ctdev && go test ./state/ -v -run TestConfigDir`
Expected: PASS

- [ ] **Step 5: Write failing test for markers**

```go
package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMarkerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ms := NewMarkerStore(dir)

	now := time.Now().Truncate(time.Second)
	marker := InstallMarker{
		InstalledAt: now,
		Version:     "1.0.0",
		UpdatedAt:   now,
	}

	if err := ms.Save("docker", marker); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := ms.Load("docker")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if !got.InstalledAt.Equal(marker.InstalledAt) {
		t.Errorf("installed_at: got %v, want %v", got.InstalledAt, marker.InstalledAt)
	}
	if got.Version != "1.0.0" {
		t.Errorf("version: got %s, want 1.0.0", got.Version)
	}
}

func TestMarkerExists(t *testing.T) {
	dir := t.TempDir()
	ms := NewMarkerStore(dir)

	if ms.Exists("docker") {
		t.Error("expected docker to not exist")
	}

	now := time.Now()
	_ = ms.Save("docker", InstallMarker{InstalledAt: now, UpdatedAt: now})

	if !ms.Exists("docker") {
		t.Error("expected docker to exist")
	}
}

func TestMarkerRemove(t *testing.T) {
	dir := t.TempDir()
	ms := NewMarkerStore(dir)

	now := time.Now()
	_ = ms.Save("docker", InstallMarker{InstalledAt: now, UpdatedAt: now})
	_ = ms.Remove("docker")

	if ms.Exists("docker") {
		t.Error("expected docker to be removed")
	}
}

func TestMigrateOldMarkers(t *testing.T) {
	oldDir := t.TempDir()
	newDir := t.TempDir()

	// Create old-style marker
	oldFile := filepath.Join(oldDir, "docker.installed")
	os.WriteFile(oldFile, []byte("2026-01-15T10:30:00Z"), 0644)

	ms := NewMarkerStore(newDir)
	migrated, err := MigrateOldMarkers(oldDir, ms)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if migrated != 1 {
		t.Errorf("expected 1 migrated, got %d", migrated)
	}

	got, err := ms.Load("docker")
	if err != nil {
		t.Fatalf("load after migrate: %v", err)
	}
	if got.Version != "unknown" {
		t.Errorf("version: got %s, want unknown", got.Version)
	}
}
```

- [ ] **Step 6: Run to verify failure**

Run: `cd ctdev && go test ./state/ -v -run TestMarker`
Expected: FAIL

- [ ] **Step 7: Write markers.go**

```go
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type InstallMarker struct {
	InstalledAt time.Time `json:"installed_at"`
	Version     string    `json:"version"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type MarkerStore struct {
	dir string
}

func NewMarkerStore(dir string) *MarkerStore {
	return &MarkerStore{dir: dir}
}

func DefaultMarkerStore() *MarkerStore {
	return NewMarkerStore(filepath.Join(StateDir(), "components"))
}

func (inst *MarkerStore) Save(name string, m InstallMarker) error {
	if err := os.MkdirAll(inst.dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(inst.dir, name+".json"), data, 0644)
}

func (inst *MarkerStore) Load(name string) (InstallMarker, error) {
	var m InstallMarker
	data, err := os.ReadFile(filepath.Join(inst.dir, name+".json"))
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(data, &m)
	return m, err
}

func (inst *MarkerStore) Exists(name string) bool {
	_, err := os.Stat(filepath.Join(inst.dir, name+".json"))
	return err == nil
}

func (inst *MarkerStore) Remove(name string) error {
	return os.Remove(filepath.Join(inst.dir, name+".json"))
}

func (inst *MarkerStore) List() ([]string, error) {
	entries, err := os.ReadDir(inst.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) == ".json" {
			names = append(names, name[:len(name)-5])
		}
	}
	return names, nil
}
```

- [ ] **Step 8: Write migrate.go**

```go
package state

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func MigrateOldMarkers(oldDir string, store *MarkerStore) (int, error) {
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	migrated := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".installed") {
			continue
		}
		componentName := strings.TrimSuffix(name, ".installed")

		if store.Exists(componentName) {
			continue
		}

		data, err := os.ReadFile(filepath.Join(oldDir, name))
		if err != nil {
			continue
		}

		installedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
		if err != nil {
			installedAt = time.Now()
		}

		marker := InstallMarker{
			InstalledAt: installedAt,
			Version:     "unknown",
			UpdatedAt:   installedAt,
		}

		if err := store.Save(componentName, marker); err != nil {
			continue
		}
		migrated++
	}
	return migrated, nil
}
```

- [ ] **Step 9: Run all marker tests**

Run: `cd ctdev && go test ./state/ -v`
Expected: PASS

- [ ] **Step 10: Write config.go**

```go
package state

import (
	"github.com/spf13/viper"
)

type Config struct {
	DefaultComponents []string `mapstructure:"default_components"`
	UpdateInterval    string   `mapstructure:"update_interval"`
}

func LoadConfig() Config {
	var cfg Config
	cfg.DefaultComponents = viper.GetStringSlice("default_components")
	cfg.UpdateInterval = viper.GetString("update_interval")
	if cfg.UpdateInterval == "" {
		cfg.UpdateInterval = "weekly"
	}
	return cfg
}
```

- [ ] **Step 11: Write config_test.go**

```go
package state

import (
	"testing"

	"github.com/spf13/viper"
)

func TestLoadConfigDefaults(t *testing.T) {
	viper.Reset()
	cfg := LoadConfig()
	if cfg.UpdateInterval != "weekly" {
		t.Errorf("expected weekly, got %s", cfg.UpdateInterval)
	}
	if len(cfg.DefaultComponents) != 0 {
		t.Errorf("expected no default components, got %v", cfg.DefaultComponents)
	}
}

func TestLoadConfigFromViper(t *testing.T) {
	viper.Reset()
	viper.Set("update_interval", "daily")
	viper.Set("default_components", []string{"zsh", "git"})
	cfg := LoadConfig()
	if cfg.UpdateInterval != "daily" {
		t.Errorf("expected daily, got %s", cfg.UpdateInterval)
	}
	if len(cfg.DefaultComponents) != 2 {
		t.Errorf("expected 2 default components, got %d", len(cfg.DefaultComponents))
	}
}
```

- [ ] **Step 12: Run all state tests**

Run: `cd ctdev && go test ./state/ -v`
Expected: PASS

- [ ] **Step 13: Commit**

```bash
git add ctdev/state/
git commit -m "feat: add XDG state management, markers, and migration"
```

### Task 5: Component Model + Registry

**Files:**
- Create: `ctdev/component/component.go`
- Create: `ctdev/component/component_test.go`
- Create: `ctdev/component/registry.go`
- Create: `ctdev/component/registry_test.go`

- [ ] **Step 1: Write failing test for component filtering**

```go
package component

import "testing"

func TestFilterByOS(t *testing.T) {
	comps := []Component{
		{Name: "docker", SupportedOS: []OS{OSAny}},
		{Name: "cleanmymac", SupportedOS: []OS{OSMacOS}},
		{Name: "earlyoom", SupportedOS: []OS{OSLinux}},
	}

	linux := FilterByOS(comps, OSLinux)
	if len(linux) != 2 {
		t.Errorf("expected 2 linux components, got %d", len(linux))
	}

	macos := FilterByOS(comps, OSMacOS)
	if len(macos) != 2 {
		t.Errorf("expected 2 macos components, got %d", len(macos))
	}
}

func TestResolveDependencies(t *testing.T) {
	comps := []Component{
		{Name: "helm", Dependencies: []string{"kubectl"}},
		{Name: "kubectl"},
		{Name: "docker"},
	}

	deps := ResolveDependencies(comps, []string{"helm"})
	if len(deps) != 2 {
		t.Errorf("expected 2 (helm + kubectl), got %d", len(deps))
	}
	if deps[0] != "kubectl" {
		t.Errorf("expected kubectl first (dependency), got %s", deps[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ctdev && go test ./component/`
Expected: FAIL

- [ ] **Step 3: Write component.go**

```go
package component

import (
	"context"
	"io"
)

type Category string

const (
	CategoryCLI      Category = "CLI Tools"
	CategoryDesktop  Category = "Desktop Applications"
	CategoryRuntime  Category = "Development Runtimes"
	CategorySecurity Category = "Security & Encryption"
	CategoryInfra    Category = "Infrastructure"
	CategorySystem   Category = "System Tools"
)

type OS string

const (
	OSLinux OS = "linux"
	OSMacOS OS = "macos"
	OSAny   OS = "any"
)

type Component struct {
	Name         string
	Description  string
	Category     Category
	SupportedOS  []OS
	Dependencies []string
	Tags         []string

	GoInstall   func(ctx context.Context, opts ExecOpts) error
	GoUninstall func(ctx context.Context, opts ExecOpts) error

	BashInstall   string
	BashUninstall string
}

type ExecOpts struct {
	DryRun  bool
	Force   bool
	Verbose bool
	Stdout  io.Writer
	Stderr  io.Writer
}

func (inst *Component) SupportsOS(os OS) bool {
	for _, supported := range inst.SupportedOS {
		if supported == OSAny || supported == os {
			return true
		}
	}
	return false
}

func FilterByOS(components []Component, os OS) []Component {
	var result []Component
	for _, c := range components {
		if c.SupportsOS(os) {
			result = append(result, c)
		}
	}
	return result
}

func GroupByCategory(components []Component) map[Category][]Component {
	groups := make(map[Category][]Component)
	for _, c := range components {
		groups[c.Category] = append(groups[c.Category], c)
	}
	return groups
}

func ResolveDependencies(all []Component, selected []string) []string {
	lookup := make(map[string]*Component)
	for i := range all {
		lookup[all[i].Name] = &all[i]
	}

	seen := make(map[string]bool)
	var result []string

	var resolve func(name string)
	resolve = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		if c, ok := lookup[name]; ok {
			for _, dep := range c.Dependencies {
				resolve(dep)
			}
		}
		result = append(result, name)
	}

	for _, name := range selected {
		resolve(name)
	}
	return result
}
```

- [ ] **Step 4: Run component tests**

Run: `cd ctdev && go test ./component/ -v`
Expected: PASS

- [ ] **Step 5: Write registry.go with all 36 components**

```go
package component

var Registry = []Component{
	{Name: "1password", Description: "1Password password manager", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, BashInstall: "components/1password/install.sh", BashUninstall: "components/1password/uninstall.sh", Tags: []string{"password", "security"}},
	{Name: "age", Description: "age file encryption tool", Category: CategorySecurity, SupportedOS: []OS{OSAny}, BashInstall: "components/age/install.sh", BashUninstall: "components/age/uninstall.sh", Tags: []string{"encryption", "crypto"}},
	{Name: "bleachbit", Description: "System cleaner for Linux", Category: CategorySystem, SupportedOS: []OS{OSLinux}, BashInstall: "components/bleachbit/install.sh", BashUninstall: "components/bleachbit/uninstall.sh", Tags: []string{"cleanup", "disk"}},
	{Name: "btop", Description: "Resource monitor", Category: CategoryCLI, SupportedOS: []OS{OSAny}, BashInstall: "components/btop/install.sh", BashUninstall: "components/btop/uninstall.sh", Tags: []string{"monitor", "htop"}},
	{Name: "bun", Description: "JavaScript runtime and package manager", Category: CategoryCLI, SupportedOS: []OS{OSAny}, BashInstall: "components/bun/install.sh", BashUninstall: "components/bun/uninstall.sh", Tags: []string{"javascript", "node"}},
	{Name: "chatgpt", Description: "ChatGPT desktop application", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, BashInstall: "components/chatgpt/install.sh", BashUninstall: "components/chatgpt/uninstall.sh", Tags: []string{"ai", "openai"}},
	{Name: "chrome", Description: "Google Chrome browser", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, BashInstall: "components/chrome/install.sh", BashUninstall: "components/chrome/uninstall.sh", Tags: []string{"browser", "web"}},
	{Name: "cleanmymac", Description: "CleanMyMac system cleaner", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS}, BashInstall: "components/cleanmymac/install.sh", BashUninstall: "components/cleanmymac/uninstall.sh", Tags: []string{"cleanup", "disk"}},
	{Name: "claude-code", Description: "Claude Code CLI and configuration", Category: CategoryCLI, SupportedOS: []OS{OSAny}, BashInstall: "components/claude-code/install.sh", BashUninstall: "components/claude-code/uninstall.sh", Tags: []string{"ai", "anthropic"}},
	{Name: "claude-desktop", Description: "Claude desktop application", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS}, BashInstall: "components/claude-desktop/install.sh", BashUninstall: "components/claude-desktop/uninstall.sh", Tags: []string{"ai", "anthropic"}},
	{Name: "codex", Description: "OpenAI Codex CLI", Category: CategoryCLI, SupportedOS: []OS{OSAny}, BashInstall: "components/codex/install.sh", BashUninstall: "components/codex/uninstall.sh", Tags: []string{"ai", "openai"}},
	{Name: "dbeaver", Description: "DBeaver database tool", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, BashInstall: "components/dbeaver/install.sh", BashUninstall: "components/dbeaver/uninstall.sh", Tags: []string{"database", "sql"}},
	{Name: "docker", Description: "Docker container runtime", Category: CategoryCLI, SupportedOS: []OS{OSAny}, BashInstall: "components/docker/install.sh", BashUninstall: "components/docker/uninstall.sh", Tags: []string{"container", "devops"}},
	{Name: "doctl", Description: "DigitalOcean CLI", Category: CategoryCLI, SupportedOS: []OS{OSAny}, BashInstall: "components/doctl/install.sh", BashUninstall: "components/doctl/uninstall.sh", Tags: []string{"cloud", "digitalocean"}},
	{Name: "earlyoom", Description: "Early OOM killer for Linux", Category: CategorySystem, SupportedOS: []OS{OSLinux}, BashInstall: "components/earlyoom/install.sh", BashUninstall: "components/earlyoom/uninstall.sh", Tags: []string{"memory", "oom"}},
	{Name: "fonts", Description: "Nerd Fonts for terminal", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, BashInstall: "components/fonts/install.sh", BashUninstall: "components/fonts/uninstall.sh", Tags: []string{"nerd", "terminal"}},
	{Name: "gh", Description: "GitHub CLI", Category: CategoryCLI, SupportedOS: []OS{OSAny}, BashInstall: "components/gh/install.sh", BashUninstall: "components/gh/uninstall.sh", Tags: []string{"github", "git"}},
	{Name: "ghostty", Description: "Ghostty terminal emulator", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, BashInstall: "components/ghostty/install.sh", BashUninstall: "components/ghostty/uninstall.sh", Tags: []string{"terminal", "emulator"}},
	{Name: "git", Description: "Git configuration and aliases", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, BashInstall: "components/git/install.sh", BashUninstall: "components/git/uninstall.sh", Tags: []string{"vcs", "version"}},
	{Name: "git-spice", Description: "Git Spice stacked branches tool", Category: CategoryCLI, SupportedOS: []OS{OSAny}, BashInstall: "components/git-spice/install.sh", BashUninstall: "components/git-spice/uninstall.sh", Tags: []string{"git", "stacked"}},
	{Name: "helm", Description: "Kubernetes package manager", Category: CategoryCLI, SupportedOS: []OS{OSAny}, Dependencies: []string{"kubectl"}, BashInstall: "components/helm/install.sh", BashUninstall: "components/helm/uninstall.sh", Tags: []string{"kubernetes", "k8s"}},
	{Name: "jq", Description: "JSON processor", Category: CategoryCLI, SupportedOS: []OS{OSAny}, BashInstall: "components/jq/install.sh", BashUninstall: "components/jq/uninstall.sh", Tags: []string{"json", "parser"}},
	{Name: "kubectl", Description: "Kubernetes CLI", Category: CategoryCLI, SupportedOS: []OS{OSAny}, BashInstall: "components/kubectl/install.sh", BashUninstall: "components/kubectl/uninstall.sh", Tags: []string{"kubernetes", "k8s"}},
	{Name: "linear", Description: "Linear issue tracker", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS}, BashInstall: "components/linear/install.sh", BashUninstall: "components/linear/uninstall.sh", Tags: []string{"issues", "project"}},
	{Name: "logi-options", Description: "Logitech Options+", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS}, BashInstall: "components/logi-options/install.sh", BashUninstall: "components/logi-options/uninstall.sh", Tags: []string{"logitech", "mouse"}},
	{Name: "node", Description: "Node.js via nodenv", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, BashInstall: "components/node/install.sh", BashUninstall: "components/node/uninstall.sh", Tags: []string{"javascript", "nodejs"}},
	{Name: "ruby", Description: "Ruby via rbenv", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, BashInstall: "components/ruby/install.sh", BashUninstall: "components/ruby/uninstall.sh", Tags: []string{"rbenv", "rails"}},
	{Name: "shellcheck", Description: "Shell script linter", Category: CategoryCLI, SupportedOS: []OS{OSAny}, BashInstall: "components/shellcheck/install.sh", BashUninstall: "components/shellcheck/uninstall.sh", Tags: []string{"lint", "bash"}},
	{Name: "slack", Description: "Slack messaging", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, BashInstall: "components/slack/install.sh", BashUninstall: "components/slack/uninstall.sh", Tags: []string{"messaging", "chat"}},
	{Name: "solaar", Description: "Logitech Unifying/Bolt receiver manager", Category: CategorySystem, SupportedOS: []OS{OSLinux}, BashInstall: "components/solaar/install.sh", BashUninstall: "components/solaar/uninstall.sh", Tags: []string{"logitech", "bluetooth"}},
	{Name: "sops", Description: "Mozilla SOPS secrets manager", Category: CategorySecurity, SupportedOS: []OS{OSAny}, BashInstall: "components/sops/install.sh", BashUninstall: "components/sops/uninstall.sh", Tags: []string{"secrets", "encrypt"}},
	{Name: "tailscale", Description: "Tailscale VPN", Category: CategorySecurity, SupportedOS: []OS{OSAny}, BashInstall: "components/tailscale/install.sh", BashUninstall: "components/tailscale/uninstall.sh", Tags: []string{"vpn", "network"}},
	{Name: "terraform", Description: "Terraform infrastructure tool", Category: CategoryInfra, SupportedOS: []OS{OSAny}, BashInstall: "components/terraform/install.sh", BashUninstall: "components/terraform/uninstall.sh", Tags: []string{"iac", "cloud"}},
	{Name: "tmux", Description: "Terminal multiplexer", Category: CategoryCLI, SupportedOS: []OS{OSAny}, BashInstall: "components/tmux/install.sh", BashUninstall: "components/tmux/uninstall.sh", Tags: []string{"terminal", "session"}},
	{Name: "vscode", Description: "Visual Studio Code", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, BashInstall: "components/vscode/install.sh", BashUninstall: "components/vscode/uninstall.sh", Tags: []string{"editor", "ide"}},
	{Name: "zsh", Description: "Zsh, Oh My Zsh, Pure prompt, plugins", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, BashInstall: "components/zsh/install.sh", BashUninstall: "components/zsh/uninstall.sh", Tags: []string{"shell", "ohmyzsh"}},
}

func FindByName(name string) *Component {
	for i := range Registry {
		if Registry[i].Name == name {
			return &Registry[i]
		}
	}
	return nil
}

func AllNames() []string {
	names := make([]string, len(Registry))
	for i, c := range Registry {
		names[i] = c.Name
	}
	return names
}
```

- [ ] **Step 6: Write registry_test.go**

```go
package component

import "testing"

func TestRegistryHas36Components(t *testing.T) {
	if len(Registry) != 36 {
		t.Errorf("expected 36 components, got %d", len(Registry))
	}
}

func TestRegistryNoDuplicates(t *testing.T) {
	seen := make(map[string]bool)
	for _, c := range Registry {
		if seen[c.Name] {
			t.Errorf("duplicate component: %s", c.Name)
		}
		seen[c.Name] = true
	}
}

func TestRegistryAllHaveBashScripts(t *testing.T) {
	for _, c := range Registry {
		if c.BashInstall == "" {
			t.Errorf("component %s missing BashInstall", c.Name)
		}
		if c.BashUninstall == "" {
			t.Errorf("component %s missing BashUninstall", c.Name)
		}
	}
}

func TestFindByName(t *testing.T) {
	c := FindByName("docker")
	if c == nil {
		t.Fatal("expected to find docker")
	}
	if c.Category != CategoryCLI {
		t.Errorf("expected CLI Tools, got %s", c.Category)
	}
}

func TestHelmDependsOnKubectl(t *testing.T) {
	c := FindByName("helm")
	if c == nil {
		t.Fatal("expected to find helm")
	}
	if len(c.Dependencies) != 1 || c.Dependencies[0] != "kubectl" {
		t.Errorf("expected helm to depend on kubectl, got %v", c.Dependencies)
	}
}
```

- [ ] **Step 7: Run all component tests**

Run: `cd ctdev && go test ./component/ -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add ctdev/component/
git commit -m "feat: add component model, registry, and dependency resolution"
```

### Task 6: Executor (Bash Bridge)

**Files:**
- Create: `ctdev/component/executor.go`
- Create: `ctdev/component/executor_test.go`
- Create: `ctdev/internal/shell/shell.go`
- Create: `ctdev/internal/shell/shell_test.go`

- [ ] **Step 1: Write shell helper**

```go
package shell

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

type RunOpts struct {
	Env    []string
	Dir    string
	Stdout io.Writer
	Stderr io.Writer
}

func Run(ctx context.Context, name string, args []string, opts RunOpts) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), opts.Env...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	return cmd.Run()
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return 1
}

func BoolEnv(name string, val bool) string {
	if val {
		return fmt.Sprintf("%s=true", name)
	}
	return fmt.Sprintf("%s=false", name)
}
```

- [ ] **Step 2: Write shell_test.go**

```go
package shell

import (
	"bytes"
	"context"
	"testing"
)

func TestRun(t *testing.T) {
	var stdout bytes.Buffer
	err := Run(context.Background(), "echo", []string{"hello"}, RunOpts{
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stdout.String() != "hello\n" {
		t.Errorf("unexpected stdout: %q", stdout.String())
	}
}

func TestExitCode(t *testing.T) {
	if ExitCode(nil) != 0 {
		t.Error("nil error should return 0")
	}
	err := Run(context.Background(), "bash", []string{"-c", "exit 2"}, RunOpts{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	if ExitCode(err) != 2 {
		t.Errorf("expected exit code 2, got %d", ExitCode(err))
	}
}

func TestBoolEnv(t *testing.T) {
	if BoolEnv("FOO", true) != "FOO=true" {
		t.Error("expected FOO=true")
	}
	if BoolEnv("FOO", false) != "FOO=false" {
		t.Error("expected FOO=false")
	}
}
```

- [ ] **Step 3: Run shell tests**

Run: `cd ctdev && go test ./internal/shell/ -v`
Expected: PASS

- [ ] **Step 4: Write executor.go**

```go
package component

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/internal/shell"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

type Executor struct {
	DotfilesRoot string
	Platform     platform.Info
}

func NewExecutor(dotfilesRoot string) *Executor {
	return &Executor{
		DotfilesRoot: dotfilesRoot,
		Platform:     platform.Detect(),
	}
}

type ExecResult struct {
	Component string
	Err       error
	ExitCode  int
	Skipped   bool // exit code 2 = unsupported OS
}

func (inst *Executor) Install(ctx context.Context, c *Component, opts ExecOpts) ExecResult {
	result := ExecResult{Component: c.Name}

	if c.GoInstall != nil {
		result.Err = c.GoInstall(ctx, opts)
		return result
	}

	err := inst.runBash(ctx, c.BashInstall, opts)
	result.ExitCode = shell.ExitCode(err)
	if result.ExitCode == 2 {
		result.Skipped = true
		return result
	}
	result.Err = err
	return result
}

func (inst *Executor) Uninstall(ctx context.Context, c *Component, opts ExecOpts) ExecResult {
	result := ExecResult{Component: c.Name}

	if c.GoUninstall != nil {
		result.Err = c.GoUninstall(ctx, opts)
		return result
	}

	err := inst.runBash(ctx, c.BashUninstall, opts)
	result.ExitCode = shell.ExitCode(err)
	if result.ExitCode == 2 {
		result.Skipped = true
		return result
	}
	result.Err = err
	return result
}

func (inst *Executor) runBash(ctx context.Context, script string, opts ExecOpts) error {
	scriptPath := filepath.Join(inst.DotfilesRoot, script)
	env := []string{
		shell.BoolEnv("FORCE", opts.Force),
		shell.BoolEnv("DRY_RUN", opts.DryRun),
		shell.BoolEnv("VERBOSE", opts.Verbose),
		fmt.Sprintf("DOTFILES_ROOT=%s", inst.DotfilesRoot),
	}
	return shell.Run(ctx, "bash", []string{scriptPath}, shell.RunOpts{
		Env:    env,
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
	})
}
```

- [ ] **Step 3: Write executor_test.go with test scripts**

```go
package component

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExecutorBashBridge(t *testing.T) {
	dir := t.TempDir()

	// Create a test install script
	scriptDir := filepath.Join(dir, "components", "test")
	os.MkdirAll(scriptDir, 0755)

	installScript := filepath.Join(scriptDir, "install.sh")
	os.WriteFile(installScript, []byte("#!/usr/bin/env bash\necho 'installed test'\n"), 0755)

	exec := NewExecutor(dir)
	c := &Component{
		Name:        "test",
		BashInstall: "components/test/install.sh",
	}

	var stdout bytes.Buffer
	result := exec.Install(context.Background(), c, ExecOpts{
		Stdout: &stdout,
		Stderr: os.Stderr,
	})

	if result.Err != nil {
		t.Fatalf("install failed: %v", result.Err)
	}
	if stdout.String() != "installed test\n" {
		t.Errorf("unexpected stdout: %q", stdout.String())
	}
}

func TestExecutorExitCode2Skipped(t *testing.T) {
	dir := t.TempDir()

	scriptDir := filepath.Join(dir, "components", "test")
	os.MkdirAll(scriptDir, 0755)

	installScript := filepath.Join(scriptDir, "install.sh")
	os.WriteFile(installScript, []byte("#!/usr/bin/env bash\nexit 2\n"), 0755)

	exec := NewExecutor(dir)
	c := &Component{
		Name:        "test",
		BashInstall: "components/test/install.sh",
	}

	result := exec.Install(context.Background(), c, ExecOpts{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})

	if !result.Skipped {
		t.Error("expected result to be skipped for exit code 2")
	}
}

func TestExecutorGoInstallOverride(t *testing.T) {
	exec := NewExecutor(t.TempDir())

	called := false
	c := &Component{
		Name: "test",
		GoInstall: func(ctx context.Context, opts ExecOpts) error {
			called = true
			return nil
		},
	}

	result := exec.Install(context.Background(), c, ExecOpts{})
	if result.Err != nil {
		t.Fatalf("install failed: %v", result.Err)
	}
	if !called {
		t.Error("expected GoInstall to be called")
	}
}
```

- [ ] **Step 4: Run executor tests**

Run: `cd ctdev && go test ./component/ -v -run TestExecutor`
Expected: PASS

- [ ] **Step 5: Run all tests**

Run: `cd ctdev && go test ./...`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add ctdev/component/executor.go ctdev/component/executor_test.go ctdev/internal/
git commit -m "feat: add bash bridge executor and shell helpers"
```

### Task 7: Basic Info Command (Batch Mode)

**Files:**
- Modify: `ctdev/cmd/root.go`
- Create: `ctdev/cmd/info.go`

- [ ] **Step 1: Write info.go command**

```go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/state"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show system information",
	RunE:  runInfo,
}

func init() {
	rootCmd.AddCommand(infoCmd)
}

func dotfilesRoot() string {
	exe, _ := os.Executable()
	return filepath.Dir(filepath.Dir(exe))
}

func runInfo(cmd *cobra.Command, args []string) error {
	info := platform.GatherSystemInfo(dotfilesRoot())
	markers := state.DefaultMarkerStore()
	installed, _ := markers.List()

	fmt.Printf("System Information\n\n")
	fmt.Printf("  %-20s %s\n", "OS", formatOS(info.Platform))
	fmt.Printf("  %-20s %s\n", "Architecture", info.Platform.Arch)
	fmt.Printf("  %-20s %s\n", "Package Manager", info.Platform.PackageManager)
	fmt.Printf("  %-20s %s\n", "Shell", info.Shell)
	fmt.Printf("  %-20s %s\n", "Dotfiles", info.DotfilesDir)
	fmt.Printf("  %-20s %s\n", "ctdev", version)
	fmt.Println()
	fmt.Printf("  %-20s %s (%d threads)\n", "CPU", info.CPUModel, info.CPUThreads)
	fmt.Printf("  %-20s %d GB\n", "Memory", info.MemoryGB)
	fmt.Println()
	fmt.Printf("  Components: %d of %d installed\n", len(installed), len(component.Registry))
	if len(installed) > 0 {
		fmt.Printf("  Installed: %s\n", strings.Join(installed, ", "))
	}

	return nil
}

func formatOS(p platform.Info) string {
	if p.Distro != "" {
		return fmt.Sprintf("%s (%s)", p.Distro, p.OS)
	}
	return string(p.OS)
}
```

- [ ] **Step 2: Build and test**

Run: `cd ctdev && make build && ./ctdev info`
Expected: System information displayed

- [ ] **Step 3: Test version flag still works**

Run: `cd ctdev && ./ctdev --version`
Expected: version string

- [ ] **Step 4: Commit**

```bash
git add ctdev/cmd/info.go
git commit -m "feat: add info command with system information display"
```

### Task 8: End-to-End Verification

- [ ] **Step 1: Run full test suite**

Run: `cd ctdev && go test ./... -v`
Expected: ALL PASS

- [ ] **Step 2: Build with version injection**

Run: `cd ctdev && make build && ./ctdev --version`
Expected: Shows version from VERSION file

- [ ] **Step 3: Verify info command**

Run: `cd ctdev && ./ctdev info`
Expected: Shows OS, arch, package manager, CPU, memory, installed components

- [ ] **Step 4: Verify help works**

Run: `cd ctdev && ./ctdev --help`
Expected: Shows usage with info subcommand listed

- [ ] **Step 5: If any fixes were needed, commit them**

```bash
git add -A ctdev/ && git diff --cached --quiet || git commit -m "fix: chunk 1 verification fixes"
```

---

## Chunk 2: TUI Styles + Install Command (Picker + Progress)

This chunk adds the Bubble Tea TUI layer — shared styles, the component picker, installation progress view, and wires them into `ctdev install`.

### Task 9: Add Bubble Tea Dependencies

**Files:**
- Modify: `ctdev/go.mod`

- [ ] **Step 1: Add dependencies**

Run: `cd ctdev && go get charm.land/bubbletea/v2@latest charm.land/bubbles/v2@latest charm.land/lipgloss/v2@latest`

- [ ] **Step 2: Verify imports work**

Run: `cd ctdev && go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add ctdev/go.mod ctdev/go.sum
git commit -m "feat: add bubbletea v2, bubbles v2, lipgloss v2 dependencies"
```

### Task 10: Shared TUI Styles

**Files:**
- Create: `ctdev/tui/styles/styles.go`

- [ ] **Step 1: Write shared Lip Gloss styles**

```go
package styles

import "charm.land/lipgloss/v2"

var (
	// Colors
	Green  = lipgloss.Color("#3fb950")
	Red    = lipgloss.Color("#f85149")
	Yellow = lipgloss.Color("#d29922")
	Blue   = lipgloss.Color("#58a6ff")
	Orange = lipgloss.Color("#f0883e")
	Subtle = lipgloss.Color("#8b949e")
	Bright = lipgloss.Color("#f0f6fc")

	// Text styles
	Title    = lipgloss.NewStyle().Bold(true).Foreground(Blue)
	Subtitle = lipgloss.NewStyle().Foreground(Subtle)
	Success  = lipgloss.NewStyle().Foreground(Green)
	Error    = lipgloss.NewStyle().Foreground(Red)
	Warning  = lipgloss.NewStyle().Foreground(Yellow)
	Dimmed   = lipgloss.NewStyle().Foreground(Subtle)

	// Component styles
	Selected   = lipgloss.NewStyle().Foreground(Green).SetString("◉")
	Unselected = lipgloss.NewStyle().Foreground(Subtle).SetString("○")
	Cursor     = lipgloss.NewStyle().Background(lipgloss.Color("#161b22"))

	// Category header
	CategoryHeader = lipgloss.NewStyle().Bold(true).Foreground(Orange)

	// Status bar
	StatusBar = lipgloss.NewStyle().
		Foreground(Subtle).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#30363d")).
		MarginTop(1).
		PaddingTop(1)

	// Help text
	Help = lipgloss.NewStyle().Foreground(Subtle)
)
```

- [ ] **Step 2: Commit**

```bash
git add ctdev/tui/styles/
git commit -m "feat: add shared Lip Gloss TUI styles"
```

### Task 11: Component Picker TUI Model

**Files:**
- Create: `ctdev/tui/picker/picker.go`
- Create: `ctdev/tui/picker/picker_test.go`

- [ ] **Step 1: Write picker model**

The picker is a Bubble Tea model that shows components grouped by category, supports multi-select with Space, category collapse with Tab, fuzzy filter with `/`, and confirms with Enter.

```go
package picker

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
)

type item struct {
	component  component.Component
	isCategory bool
	category   component.Category
	collapsed  bool
	installed  bool
}

type Model struct {
	items      []item
	cursor     int
	selected   map[string]bool
	filtering  bool
	filter     string
	filtered   []int // indices into items
	quitting   bool
	confirmed  bool
	width      int
	height     int
	platform   component.OS
}

type Result struct {
	Selected []string
	Quit     bool
}

func New(components []component.Component, installed map[string]bool, os component.OS) Model {
	filtered := component.FilterByOS(components, os)
	groups := component.GroupByCategory(filtered)

	categoryOrder := []component.Category{
		component.CategoryCLI,
		component.CategoryDesktop,
		component.CategoryRuntime,
		component.CategorySecurity,
		component.CategoryInfra,
		component.CategorySystem,
	}

	var items []item
	for _, cat := range categoryOrder {
		comps, ok := groups[cat]
		if !ok || len(comps) == 0 {
			continue
		}
		items = append(items, item{isCategory: true, category: cat})
		for _, c := range comps {
			items = append(items, item{
				component: c,
				installed: installed[c.Name],
			})
		}
	}

	return Model{
		items:    items,
		selected: make(map[string]bool),
		platform: os,
	}
}

func (inst Model) Init() tea.Cmd {
	return nil
}

func (inst Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		inst.width = msg.Width
		inst.height = msg.Height
		return inst, nil

	case tea.KeyPressMsg:
		if inst.filtering {
			return inst.updateFilter(msg)
		}

		switch msg.String() {
		case "q", "ctrl+c":
			inst.quitting = true
			return inst, tea.Quit
		case "enter":
			inst.confirmed = true
			return inst, tea.Quit
		case "up", "k":
			inst.moveCursor(-1)
		case "down", "j":
			inst.moveCursor(1)
		case " ":
			inst.toggleSelected()
		case "tab":
			inst.toggleCategory()
		case "/":
			inst.filtering = true
			inst.filter = ""
		case "a":
			inst.selectAll()
		case "n":
			inst.selectNone()
		}
	}
	return inst, nil
}

func (inst *Model) moveCursor(dir int) {
	for {
		inst.cursor += dir
		if inst.cursor < 0 {
			inst.cursor = 0
			return
		}
		if inst.cursor >= len(inst.items) {
			inst.cursor = len(inst.items) - 1
			return
		}
		// Skip hidden items (collapsed categories)
		if !inst.isHidden(inst.cursor) {
			return
		}
	}
}

func (inst *Model) isHidden(idx int) bool {
	if inst.items[idx].isCategory {
		return false
	}
	// Find parent category
	for i := idx - 1; i >= 0; i-- {
		if inst.items[i].isCategory {
			return inst.items[i].collapsed
		}
	}
	return false
}

func (inst *Model) toggleSelected() {
	if inst.cursor >= 0 && inst.cursor < len(inst.items) {
		it := inst.items[inst.cursor]
		if !it.isCategory {
			if inst.selected[it.component.Name] {
				delete(inst.selected, it.component.Name)
			} else {
				inst.selected[it.component.Name] = true
			}
		}
	}
}

func (inst *Model) toggleCategory() {
	if inst.cursor >= 0 && inst.cursor < len(inst.items) && inst.items[inst.cursor].isCategory {
		inst.items[inst.cursor].collapsed = !inst.items[inst.cursor].collapsed
	}
}

func (inst *Model) selectAll() {
	for _, it := range inst.items {
		if !it.isCategory && !it.installed {
			inst.selected[it.component.Name] = true
		}
	}
}

func (inst *Model) selectNone() {
	inst.selected = make(map[string]bool)
}

func (inst Model) updateFilter(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc":
		inst.filtering = false
		return inst, nil
	case "backspace":
		if len(inst.filter) > 0 {
			inst.filter = inst.filter[:len(inst.filter)-1]
		}
	default:
		if len(msg.String()) == 1 {
			inst.filter += msg.String()
		}
	}
	return inst, nil
}

func (inst Model) matchesFilter(c component.Component) bool {
	if inst.filter == "" {
		return true
	}
	f := strings.ToLower(inst.filter)
	return strings.Contains(strings.ToLower(c.Name), f) ||
		strings.Contains(strings.ToLower(c.Description), f) ||
		matchTags(c.Tags, f)
}

func matchTags(tags []string, filter string) bool {
	for _, t := range tags {
		if strings.Contains(strings.ToLower(t), filter) {
			return true
		}
	}
	return false
}

func (inst Model) View() tea.View {
	var b strings.Builder

	b.WriteString(styles.Title.Render("Select components to install"))
	b.WriteString("\n")
	b.WriteString(styles.Help.Render("Space toggle · Tab expand/collapse · / filter · Enter confirm · q quit"))
	b.WriteString("\n\n")

	if inst.filtering {
		b.WriteString(fmt.Sprintf("Filter: %s█\n\n", inst.filter))
	}

	for i, it := range inst.items {
		if inst.isHidden(i) {
			continue
		}
		if !it.isCategory && !inst.matchesFilter(it.component) {
			continue
		}

		isCursor := i == inst.cursor
		line := ""

		if it.isCategory {
			arrow := "▼"
			if it.collapsed {
				arrow = "▶"
			}
			count := inst.countInCategory(it.category)
			line = styles.CategoryHeader.Render(fmt.Sprintf("%s %s", arrow, string(it.category)))
			if it.collapsed {
				line += styles.Dimmed.Render(fmt.Sprintf(" (%d components)", count))
			}
		} else {
			indicator := styles.Unselected.String()
			if inst.selected[it.component.Name] {
				indicator = styles.Selected.String()
			} else if it.installed {
				indicator = styles.Success.Render("●")
			}
			name := lipgloss.NewStyle().Foreground(styles.Bright).Width(14).Render(it.component.Name)
			desc := styles.Dimmed.Render(it.component.Description)
			line = fmt.Sprintf("  %s %s %s", indicator, name, desc)
		}

		if isCursor {
			line = styles.Cursor.Render(line)
		}
		b.WriteString(line + "\n")
	}

	// Status bar
	selectedCount := len(inst.selected)
	installedCount := inst.countInstalled()
	status := fmt.Sprintf("%s · %d already installed",
		styles.Success.Render(fmt.Sprintf("%d selected", selectedCount)),
		installedCount,
	)
	b.WriteString(styles.StatusBar.Render(status))

	return tea.NewView(b.String())
}

func (inst Model) countInCategory(cat component.Category) int {
	count := 0
	for _, it := range inst.items {
		if !it.isCategory && it.component.Category == cat {
			count++
		}
	}
	return count
}

func (inst Model) countInstalled() int {
	count := 0
	for _, it := range inst.items {
		if !it.isCategory && it.installed {
			count++
		}
	}
	return count
}

func (inst Model) GetResult() Result {
	if inst.quitting && !inst.confirmed {
		return Result{Quit: true}
	}
	var names []string
	for name := range inst.selected {
		names = append(names, name)
	}
	return Result{Selected: names}
}
```

- [ ] **Step 2: Write picker test**

```go
package picker

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/component"
)

func testComponents() []component.Component {
	return []component.Component{
		{Name: "docker", Description: "Container runtime", Category: component.CategoryCLI, SupportedOS: []component.OS{component.OSAny}},
		{Name: "btop", Description: "Resource monitor", Category: component.CategoryCLI, SupportedOS: []component.OS{component.OSAny}},
		{Name: "chrome", Description: "Browser", Category: component.CategoryDesktop, SupportedOS: []component.OS{component.OSAny}},
	}
}

func TestPickerSelectToggle(t *testing.T) {
	m := New(testComponents(), map[string]bool{}, component.OSLinux)
	// NOTE: KeyPressMsg field names may differ in bubbletea v2 at build time.
	// Check `charm.land/bubbletea/v2` godoc for exact struct fields.
	// If fields changed, construct key messages using the v2 API helpers.
	// Move to first component (skip category header)
	m.moveCursor(1) // direct method call avoids fragile msg construction
	// Select it
	m.toggleSelected()

	result := m.GetResult()
	if len(m.selected) != 1 {
		t.Errorf("expected 1 selected, got %d", len(m.selected))
	}
	_ = result
}

func TestPickerQuitReturnsNoSelection(t *testing.T) {
	m := New(testComponents(), map[string]bool{}, component.OSLinux)
	m.quitting = true // directly set state instead of fragile msg construction

	result := m.GetResult()
	if !result.Quit {
		t.Error("expected quit result")
	}
}
```

- [ ] **Step 3: Run picker tests**

Run: `cd ctdev && go test ./tui/picker/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add ctdev/tui/picker/
git commit -m "feat: add component picker TUI model"
```

### Task 12: Installation Progress TUI Model

**Files:**
- Create: `ctdev/tui/progress/progress.go`

- [ ] **Step 1: Write progress model**

```go
package progress

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	bprogress "charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
)

type ComponentStatus int

const (
	StatusWaiting ComponentStatus = iota
	StatusRunning
	StatusDone
	StatusFailed
	StatusSkipped
)

type ComponentState struct {
	Name      string
	Status    ComponentStatus
	Output    []string // last N lines of stdout
	Error     string
	Duration  time.Duration
}

// Messages
type InstallStartMsg struct{ Name string }
type InstallOutputMsg struct{ Name, Line string }
type InstallDoneMsg struct{ Name string; Duration time.Duration }
type InstallFailMsg struct{ Name, Error string; Duration time.Duration }
type InstallSkipMsg struct{ Name string }
type AllDoneMsg struct{}

type Model struct {
	components  []ComponentState
	current     int
	spinner     spinner.Model
	progressBar bprogress.Model
	done        bool
	startTime   time.Time
	width       int
}

func New(names []string) Model {
	s := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(styles.Orange)),
	)
	p := bprogress.New(
		bprogress.WithDefaultBlend(),
		bprogress.WithWidth(40),
	)

	states := make([]ComponentState, len(names))
	for i, name := range names {
		states[i] = ComponentState{Name: name, Status: StatusWaiting}
	}

	return Model{
		components:  states,
		spinner:     s,
		progressBar: p,
		startTime:   time.Now(),
	}
}

func (inst Model) Init() tea.Cmd {
	return inst.spinner.Tick
}

func (inst Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		inst.width = msg.Width
		return inst, nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return inst, tea.Quit
		}
	case InstallStartMsg:
		for i := range inst.components {
			if inst.components[i].Name == msg.Name {
				inst.components[i].Status = StatusRunning
				inst.current = i
				break
			}
		}
	case InstallOutputMsg:
		for i := range inst.components {
			if inst.components[i].Name == msg.Name {
				inst.components[i].Output = appendTail(inst.components[i].Output, msg.Line, 3)
				break
			}
		}
	case InstallDoneMsg:
		for i := range inst.components {
			if inst.components[i].Name == msg.Name {
				inst.components[i].Status = StatusDone
				inst.components[i].Duration = msg.Duration
				break
			}
		}
		cmd := inst.progressBar.SetPercent(inst.donePercent())
		return inst, cmd
	case InstallFailMsg:
		for i := range inst.components {
			if inst.components[i].Name == msg.Name {
				inst.components[i].Status = StatusFailed
				inst.components[i].Error = msg.Error
				inst.components[i].Duration = msg.Duration
				break
			}
		}
		cmd := inst.progressBar.SetPercent(inst.donePercent())
		return inst, cmd
	case InstallSkipMsg:
		for i := range inst.components {
			if inst.components[i].Name == msg.Name {
				inst.components[i].Status = StatusSkipped
				break
			}
		}
	case AllDoneMsg:
		inst.done = true
		return inst, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		inst.spinner, cmd = inst.spinner.Update(msg)
		return inst, cmd
	case bprogress.FrameMsg:
		m, cmd := inst.progressBar.Update(msg)
		inst.progressBar = m
		return inst, cmd
	}
	return inst, nil
}

func (inst Model) View() tea.View {
	var b strings.Builder

	if inst.done {
		b.WriteString(inst.viewSummary())
	} else {
		b.WriteString(inst.viewProgress())
	}

	return tea.NewView(b.String())
}

func (inst Model) viewProgress() string {
	var b strings.Builder

	doneCount := inst.countDone()
	total := len(inst.components)
	b.WriteString(fmt.Sprintf("Installing %d components\n\n", total))
	b.WriteString(fmt.Sprintf("  %s  %d of %d\n\n", inst.progressBar.View(), doneCount, total))

	for _, c := range inst.components {
		switch c.Status {
		case StatusDone:
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				styles.Success.Render("✓"),
				styles.Dimmed.Render(c.Name),
				styles.Dimmed.Render(fmt.Sprintf("%.1fs", c.Duration.Seconds())),
			))
		case StatusRunning:
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				inst.spinner.View(),
				lipgloss.NewStyle().Bold(true).Foreground(styles.Bright).Render(c.Name),
				styles.Orange.String(),
			))
			for _, line := range c.Output {
				b.WriteString(fmt.Sprintf("    %s\n", styles.Dimmed.Render(line)))
			}
		case StatusFailed:
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				styles.Error.Render("✗"),
				c.Name,
				styles.Error.Render(c.Error),
			))
		case StatusWaiting:
			b.WriteString(fmt.Sprintf("  %s %s\n",
				styles.Dimmed.Render("○"),
				styles.Dimmed.Render(c.Name),
			))
		}
	}

	elapsed := time.Since(inst.startTime)
	b.WriteString(fmt.Sprintf("\n  Elapsed: %.1fs  ·  Ctrl+C to cancel\n", elapsed.Seconds()))
	return b.String()
}

func (inst Model) viewSummary() string {
	var b strings.Builder
	succeeded, failed := 0, 0
	var failedNames []string

	b.WriteString(styles.Success.Render("✓ Installation complete") + "\n\n")

	for _, c := range inst.components {
		switch c.Status {
		case StatusDone:
			succeeded++
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				styles.Success.Render("✓"), c.Name,
				styles.Dimmed.Render(fmt.Sprintf("%.1fs", c.Duration.Seconds())),
			))
		case StatusFailed:
			failed++
			failedNames = append(failedNames, c.Name)
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				styles.Error.Render("✗"), c.Name,
				styles.Error.Render(c.Error),
			))
		case StatusSkipped:
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				styles.Warning.Render("–"), c.Name,
				styles.Dimmed.Render("skipped (unsupported OS)"),
			))
		}
	}

	b.WriteString(fmt.Sprintf("\n  %s · %s\n",
		styles.Success.Render(fmt.Sprintf("%d succeeded", succeeded)),
		styles.Error.Render(fmt.Sprintf("%d failed", failed)),
	))

	if len(failedNames) > 0 {
		b.WriteString(fmt.Sprintf("\n  Retry: ctdev install %s\n", strings.Join(failedNames, " ")))
	}

	return b.String()
}

func (inst Model) donePercent() float64 {
	done := inst.countDone()
	if len(inst.components) == 0 {
		return 1.0
	}
	return float64(done) / float64(len(inst.components))
}

func (inst Model) countDone() int {
	count := 0
	for _, c := range inst.components {
		if c.Status == StatusDone || c.Status == StatusFailed || c.Status == StatusSkipped {
			count++
		}
	}
	return count
}

func appendTail(lines []string, line string, max int) []string {
	lines = append(lines, line)
	if len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	return lines
}
```

- [ ] **Step 2: Write progress_test.go**

```go
package progress

import (
	"testing"
	"time"
)

func TestProgressModelInit(t *testing.T) {
	m := New([]string{"docker", "btop"})
	if len(m.components) != 2 {
		t.Errorf("expected 2 components, got %d", len(m.components))
	}
	if m.components[0].Status != StatusWaiting {
		t.Error("expected initial status to be waiting")
	}
}

func TestProgressModelInstallDone(t *testing.T) {
	m := New([]string{"docker", "btop"})

	updated, _ := m.Update(InstallStartMsg{Name: "docker"})
	m = updated.(Model)
	if m.components[0].Status != StatusRunning {
		t.Error("expected docker to be running")
	}

	updated, _ = m.Update(InstallDoneMsg{Name: "docker", Duration: 3 * time.Second})
	m = updated.(Model)
	if m.components[0].Status != StatusDone {
		t.Error("expected docker to be done")
	}
	if m.donePercent() != 0.5 {
		t.Errorf("expected 50%% done, got %f", m.donePercent())
	}
}

func TestProgressModelInstallFail(t *testing.T) {
	m := New([]string{"docker"})

	updated, _ := m.Update(InstallFailMsg{Name: "docker", Error: "apt lock", Duration: time.Second})
	m = updated.(Model)
	if m.components[0].Status != StatusFailed {
		t.Error("expected docker to be failed")
	}
	if m.components[0].Error != "apt lock" {
		t.Errorf("expected 'apt lock', got %s", m.components[0].Error)
	}
}

func TestProgressOutputTail(t *testing.T) {
	lines := appendTail(nil, "a", 3)
	lines = appendTail(lines, "b", 3)
	lines = appendTail(lines, "c", 3)
	lines = appendTail(lines, "d", 3)
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "b" {
		t.Errorf("expected 'b' first, got %s", lines[0])
	}
}
```

- [ ] **Step 3: Run progress tests**

Run: `cd ctdev && go test ./tui/progress/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add ctdev/tui/progress/
git commit -m "feat: add installation progress TUI model"
```

### Task 13: Install Command — Wire Picker + Progress

**Files:**
- Create: `ctdev/cmd/install.go`

- [ ] **Step 1: Write install command**

```go
package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	comp "github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/state"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/picker"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/progress"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install [component...]",
	Short: "Install components",
	Long:  "Install one or more components. Run without arguments for interactive picker.",
	RunE:  runInstall,
}

func init() {
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	markers := state.DefaultMarkerStore()
	executor := comp.NewExecutor(dotfilesRoot())

	var selected []string

	if len(args) > 0 {
		// Direct mode: validate and install
		for _, name := range args {
			if comp.FindByName(name) == nil {
				return fmt.Errorf("unknown component: %s", name)
			}
		}
		selected = args
	} else if isBatchMode() {
		return fmt.Errorf("no components specified (batch mode requires arguments)")
	} else {
		// Interactive picker
		installed := make(map[string]bool)
		list, _ := markers.List()
		for _, name := range list {
			installed[name] = true
		}

		osType := comp.OS(executor.Platform.OS)
		p := tea.NewProgram(picker.New(comp.Registry, installed, osType), tea.WithAltScreen())
		result, err := p.Run()
		if err != nil {
			return err
		}
		pickerResult := result.(picker.Model).GetResult()
		if pickerResult.Quit || len(pickerResult.Selected) == 0 {
			return nil
		}
		selected = pickerResult.Selected
	}

	// Resolve dependencies
	selected = comp.ResolveDependencies(comp.Registry, selected)

	// Run installation with progress TUI
	return runInstallWithProgress(executor, markers, selected)
}

func runInstallWithProgress(executor *comp.Executor, markers *state.MarkerStore, names []string) error {
	progressModel := progress.New(names)

	p := tea.NewProgram(progressModel)

	// Run installations in background goroutine
	go func() {
		for _, name := range names {
			c := comp.FindByName(name)
			if c == nil {
				continue
			}

			p.Send(progress.InstallStartMsg{Name: name})
			start := time.Now()

			// Create a pipe to capture output
			pr, pw, _ := os.Pipe()
			go func(name string) {
				scanner := bufio.NewScanner(pr)
				for scanner.Scan() {
					p.Send(progress.InstallOutputMsg{Name: name, Line: scanner.Text()})
				}
			}(name)

			result := executor.Install(context.Background(), c, comp.ExecOpts{
				Force:   flagForce,
				DryRun:  flagDryRun,
				Verbose: flagVerbose,
				Stdout:  pw,
				Stderr:  pw,
			})
			pw.Close()

			duration := time.Since(start)

			if result.Skipped {
				p.Send(progress.InstallSkipMsg{Name: name})
			} else if result.Err != nil {
				p.Send(progress.InstallFailMsg{Name: name, Error: result.Err.Error(), Duration: duration})
			} else {
				// Save marker
				markers.Save(name, state.InstallMarker{
					InstalledAt: time.Now(),
					Version:     "unknown",
					UpdatedAt:   time.Now(),
				})
				p.Send(progress.InstallDoneMsg{Name: name, Duration: duration})
			}
		}
		p.Send(progress.AllDoneMsg{})
	}()

	_, err := p.Run()
	return err
}
```

- [ ] **Step 2: Build and test with named components**

Run: `cd ctdev && make build && ./ctdev install --dry-run jq btop`
Expected: Shows progress TUI with dry-run output

- [ ] **Step 3: Test interactive picker launches**

Run: `cd ctdev && ./ctdev install`
Expected: Full-screen picker TUI appears

- [ ] **Step 4: Test help**

Run: `cd ctdev && ./ctdev install --help`
Expected: Shows install usage

- [ ] **Step 5: Commit**

```bash
git add ctdev/cmd/install.go
git commit -m "feat: add install command with picker and progress TUI"
```

### Task 14: Chunk 2 Verification

- [ ] **Step 1: Run full test suite**

Run: `cd ctdev && go test ./... -v`
Expected: ALL PASS

- [ ] **Step 2: Test full install flow (picker -> progress)**

Run: `cd ctdev && ./ctdev install` then select components with Space, confirm with Enter
Expected: Picker -> progress view -> completion summary

- [ ] **Step 3: Commit milestone**

```bash
git commit -m "feat: chunk 2 complete — install command with picker and progress TUI"
```

---

## Chunk 3: Uninstall + Update Commands

### Task 15: Uninstall Command

**Files:**
- Create: `ctdev/cmd/uninstall.go`

Reuses the picker model from Task 11, but filtered to show only installed components. Red selection indicators. The progress model from Task 12 handles execution display.

- [ ] **Step 1: Write uninstall.go**

```go
func runUninstall(cmd *cobra.Command, args []string) error {
	markers := state.DefaultMarkerStore()
	executor := comp.NewExecutor(dotfilesRoot())

	var selected []string

	if len(args) > 0 {
		for _, name := range args {
			if comp.FindByName(name) == nil {
				return fmt.Errorf("unknown component: %s", name)
			}
		}
		selected = args
	} else if isBatchMode() {
		return fmt.Errorf("no components specified")
	} else {
		// Picker filtered to installed only
		installed := make(map[string]bool)
		list, _ := markers.List()
		for _, name := range list {
			installed[name] = true
		}
		// Only pass installed components to picker
		var installedComps []comp.Component
		for _, c := range comp.Registry {
			if installed[c.Name] {
				installedComps = append(installedComps, c)
			}
		}
		osType := comp.OS(executor.Platform.OS)
		p := tea.NewProgram(picker.New(installedComps, installed, osType), tea.WithAltScreen())
		result, err := p.Run()
		if err != nil {
			return err
		}
		pickerResult := result.(picker.Model).GetResult()
		if pickerResult.Quit || len(pickerResult.Selected) == 0 {
			return nil
		}
		selected = pickerResult.Selected
	}

	// Run uninstallation with progress TUI (same pattern as install)
	// On success: markers.Remove(name)
	return runUninstallWithProgress(executor, markers, selected)
}
```

- [ ] **Step 2: Test with args**: `./ctdev uninstall --dry-run jq`
- [ ] **Step 3: Test interactive picker**: `./ctdev uninstall`
- [ ] **Step 4: Commit**

### Task 16: Update Checklist TUI Model

**Files:**
- Create: `ctdev/tui/checklist/checklist.go`

The checklist model displays available updates grouped by source (System Packages, Component Updates, Flatpak, Runtime Updates). Each item shows version diff. All selected by default. Supports `a` (all), `n` (none), Space toggle, Enter confirm.

Key types:

```go
type UpdateItem struct {
	Name       string
	Source     string   // "apt", "flatpak", "git", "runtime"
	CurrentVer string
	NewVer     string
	IsMajor    bool     // major version bump badge
	IsKernel   bool     // kernel update badge
}
```

The model is structurally similar to the picker (grouped list, multi-select) but with different item rendering.

- [ ] **Step 1: Write checklist.go**
- [ ] **Step 2: Write checklist_test.go**
- [ ] **Step 3: Commit**

### Task 17: Update Scan Logic

**Files:**
- Create: `ctdev/cmd/update.go`

The update command has two phases:
1. **Scan phase** — runs checks in parallel (apt, flatpak, git repos, bun) and collects UpdateItems
2. **Checklist phase** — presents the checklist TUI, user selects, then executes

For the scan phase, shell out to:
- `apt list --upgradable 2>/dev/null` (Linux/apt)
- `brew outdated` (macOS)
- `flatpak update --appstream && flatpak remote-ls --updates` (if flatpak exists)
- Git repos: check `git -C <path> rev-list HEAD..@{u} --count` for zsh plugins, nodenv, rbenv

Flags: `--yes`/`-y`, `--check`, `--refresh-keys`

- [ ] **Step 1: Write update scan functions**

Each scanner runs in a goroutine, returns `[]UpdateItem` via a channel:

```go
func scanAPT(ctx context.Context) ([]UpdateItem, error) {
	out, err := exec.CommandContext(ctx, "apt", "list", "--upgradable").Output()
	// Parse lines: "pkg/suite version1 arch [upgradable from: version0]"
	// Return UpdateItem{Name: pkg, Source: "apt", CurrentVer: version0, NewVer: version1}
}

func scanFlatpak(ctx context.Context) ([]UpdateItem, error) {
	out, err := exec.CommandContext(ctx, "flatpak", "remote-ls", "--updates", "--columns=application,version").Output()
	// Parse tab-separated output
}

func scanGitRepo(ctx context.Context, name, path string) (*UpdateItem, error) {
	// git -C path fetch --quiet
	// git -C path rev-list HEAD..@{u} --count
	// If count > 0, return UpdateItem{Name: name, Source: "git", NewVer: fmt.Sprintf("%d commits behind", count)}
}

func scanBrew(ctx context.Context) ([]UpdateItem, error) {
	out, err := exec.CommandContext(ctx, "brew", "outdated", "--verbose").Output()
	// Parse "pkg (installed) < available"
}
```

- [ ] **Step 2: Write update.go command with scan orchestration**

```go
func runUpdate(cmd *cobra.Command, args []string) error {
	if flagCheck {
		return runUpdateCheck() // scan + print, no install
	}
	if flagRefreshKeys {
		refreshAPTKeys(args) // shell out to existing key refresh logic
	}

	// Phase 1: Scan all sources in parallel
	items := scanAll(context.Background())

	if len(items) == 0 {
		fmt.Println("Everything is up to date.")
		return nil
	}

	// Phase 2: Interactive checklist or batch
	var selected []UpdateItem
	if isBatchMode() || flagYes {
		selected = items
	} else {
		// Launch checklist TUI
		p := tea.NewProgram(checklist.New(items), tea.WithAltScreen())
		result, err := p.Run()
		// ...get selected items from result
	}

	// Phase 3: Execute selected updates (reuse progress model)
	return executeUpdates(selected)
}
```

- [ ] **Step 3: Test `--check` mode**: `./ctdev update --check`
- [ ] **Step 4: Test interactive flow**: `./ctdev update`
- [ ] **Step 5: Commit**

### Task 18: Chunk 3 Verification

- [ ] **Step 1: Run full test suite**: `go test ./...`
- [ ] **Step 2: Test uninstall flow**
- [ ] **Step 3: Test update flow**
- [ ] **Step 4: Commit milestone**

```bash
git commit -m "feat: chunk 3 complete — uninstall and update commands"
```

---

## Chunk 4: Setup Wizard + Info TUI

### Task 19: Wizard TUI Model

**Files:**
- Create: `ctdev/tui/wizard/wizard.go`

Multi-step wizard with sidebar navigation. Each step has a title, description, and list of toggleable options. Steps are defined as data:

```go
type Step struct {
	Title       string
	Description string
	Options     []Option
}

type Option struct {
	Label       string
	Description string
	Enabled     bool
	AlreadyDone bool   // shown as "already active"
	BashScript  string // script to run if enabled
}
```

Steps are OS-dependent — passed in by the setup command.

The model tracks: current step index, option selections per step, completed steps.

Navigation: Enter = next, Esc = back, `s` = skip, Space = toggle option.

Layout: left sidebar (20 chars) + right panel using Lip Gloss.

- [ ] **Step 1: Write wizard.go**
- [ ] **Step 2: Write wizard_test.go** — test step navigation, option toggling
- [ ] **Step 3: Commit**

### Task 20: Setup Command

**Files:**
- Create: `ctdev/cmd/setup.go`

Defines Linux Mint and macOS steps. Flags: `--show`, `--reset`, `--skip-gpu`, `--skip-configure`, `--batch`.

`--show` renders a dashboard using Lip Gloss (4-panel grid showing current config).
`--batch` runs all steps with defaults non-interactively.
Default: launches wizard TUI.

The last wizard step launches the component picker from Task 11.

- [ ] **Step 1: Write setup.go**
- [ ] **Step 2: Test `--show`**: `./ctdev setup --show`
- [ ] **Step 3: Test wizard**: `./ctdev setup`
- [ ] **Step 4: Commit**

### Task 21: Rich Info TUI

**Files:**
- Modify: `ctdev/cmd/info.go`
- Create: `ctdev/tui/info/info.go`

Upgrade the basic info command to use Lip Gloss for rich formatting:
- Styled section headers
- Disk usage with visual progress bars using `progress.ViewAs()`
- Installed components as styled pill badges
- Color-coded disk usage (green < 60%, yellow < 85%, red > 85%)

- [ ] **Step 1: Write info TUI model**
- [ ] **Step 2: Update info.go to use TUI model**
- [ ] **Step 3: Commit**

### Task 22: Chunk 4 Verification

- [ ] **Step 1: Run full test suite**
- [ ] **Step 2: Test setup wizard flow**
- [ ] **Step 3: Test setup --show**
- [ ] **Step 4: Test rich info display**
- [ ] **Step 5: Commit milestone**

```bash
git commit -m "feat: chunk 4 complete — setup wizard and rich info TUI"
```

---

## Chunk 5: Remaining Commands + Build System

### Task 23: Cleanup Command

**Files:**
- Create: `ctdev/cmd/cleanup.go`

Checklist of cleanup tasks (old kernels, APT repos, package cache). Reuses checklist pattern. Linux-only.

- [ ] **Step 1: Write cleanup.go**
- [ ] **Step 2: Test**: `./ctdev cleanup --dry-run`
- [ ] **Step 3: Commit**

### Task 24: Configure Command

**Files:**
- Create: `ctdev/cmd/configure.go`

`ctdev configure git` with Bubble Tea text inputs (name, email) and scope toggle (global/local). Flags: `--name`, `--email`, `--local`, `--show`.

Uses `bubbles/textinput` for the form fields.

- [ ] **Step 1: Write configure.go**
- [ ] **Step 2: Test**: `./ctdev configure git --show`
- [ ] **Step 3: Test interactive**: `./ctdev configure git`
- [ ] **Step 4: Commit**

### Task 25: GPU Command

**Files:**
- Create: `ctdev/cmd/gpu.go`

`ctdev gpu info` — static display (no TUI needed, just Lip Gloss formatting).
`ctdev gpu setup` — step-by-step flow, shells out to existing GPU bash scripts. Flags: `--recover` (re-enroll MOK after CMOS reset), `--force`, `--dry-run`.

- [ ] **Step 1: Write gpu.go with info and setup subcommands, including --recover flag**
- [ ] **Step 2: Test**: `./ctdev gpu info`
- [ ] **Step 3: Test**: `./ctdev gpu setup --help` (verify --recover flag shown)
- [ ] **Step 4: Commit**

### Task 26: Build System + .gitignore

**Files:**
- Modify: `ctdev/Makefile` (add cross-compilation targets)
- Modify: `.gitignore` (add ctdev binary, .superpowers/)

- [ ] **Step 1: Update Makefile with cross-compilation**

```makefile
VERSION := $(shell cat ../VERSION)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
BINARY := ctdev

.PHONY: build clean test build-all

build:
	go build $(LDFLAGS) -o $(BINARY) .

build-all:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY)-darwin-arm64 .

clean:
	rm -f $(BINARY)
	rm -rf dist/

test:
	go test ./...
```

- [ ] **Step 2: Update .gitignore**

Add: `ctdev/ctdev`, `ctdev/dist/`, `.superpowers/`

- [ ] **Step 3: Commit**

```bash
git add ctdev/Makefile .gitignore
git commit -m "feat: add cross-compilation and update gitignore"
```

### Task 27: Marker Migration on First Run

**Files:**
- Modify: `ctdev/cmd/root.go`

Add a `PersistentPreRunE` to the root command that checks for old-style markers and migrates them on first run.

- [ ] **Step 1: Add migration to root.go PersistentPreRunE**
- [ ] **Step 2: Test with old-style markers present**
- [ ] **Step 3: Commit**

### Task 28: Final End-to-End Verification

- [ ] **Step 1: Run full test suite**: `cd ctdev && go test ./... -v`
- [ ] **Step 2: Build**: `make build`
- [ ] **Step 3: Test all commands**:
  - `./ctdev --version`
  - `./ctdev info`
  - `./ctdev install` (interactive)
  - `./ctdev install --dry-run jq` (direct)
  - `./ctdev uninstall` (interactive)
  - `./ctdev update --check`
  - `./ctdev setup --show`
  - `./ctdev cleanup --dry-run`
  - `./ctdev configure git --show`
  - `./ctdev gpu info`
- [ ] **Step 4: Cross-compile**: `make build-all`
- [ ] **Step 5: Commit milestone**

```bash
git commit -m "feat: chunk 5 complete — all commands, build system, migration"
```
