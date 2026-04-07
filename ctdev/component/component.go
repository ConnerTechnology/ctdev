package component

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

	// DetectCmd is the command name to check if this component is installed.
	// If empty, defaults to Name.
	DetectCmd string
	// DetectPath is an alternative filesystem path to check instead of a command.
	DetectPath string
	// DetectApps lists macOS .app bundle paths to check (e.g. "/Applications/Foo.app").
	// If any exists, the component is considered installed.
	DetectApps []string

	GoInstall   func(ctx context.Context, opts ExecOpts) error
	GoUninstall func(ctx context.Context, opts ExecOpts) error
}

type ExecOpts struct {
	DryRun  bool
	Force   bool
	Verbose bool
	Stdout  io.Writer
	Stderr  io.Writer
}

func (inst *Component) IsInstalled() bool {
	// DetectPath is exclusive — if set, only check the filesystem path.
	if inst.DetectPath != "" {
		_, err := os.Stat(os.ExpandEnv(inst.DetectPath))
		return err == nil
	}
	// Check macOS .app bundles or other filesystem paths.
	for _, app := range inst.DetectApps {
		if _, err := os.Stat(os.ExpandEnv(app)); err == nil {
			return true
		}
	}
	// Fall through to command check (covers Linux for apps like chrome/vscode
	// that have DetectApps for macOS and DetectCmd for Linux).
	cmd := inst.DetectCmd
	if cmd == "" {
		cmd = inst.Name
	}
	_, err := exec.LookPath(cmd)
	return err == nil
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
