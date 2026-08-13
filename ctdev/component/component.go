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

// RootNeed says when a component's install or uninstall shells out as root, so
// ctdev only asks for a sudo password when something is actually going to use
// it. A container without sudo (or without privilege escalation at all) can
// still install everything that lives in $HOME.
type RootNeed int

const (
	// RootWhenMissing is the zero value and the common case: root is needed to
	// put the software in place — a package manager, /usr/local, a systemd unit
	// — while re-running over an already-installed component only re-syncs
	// config files under $HOME.
	RootWhenMissing RootNeed = iota
	// RootAlways marks components that do privileged work on every run,
	// installed or not: restic redeploys its /etc units, caddy brings its stack
	// up through sudo, nomachine re-applies its ufw rule, smartmontools
	// re-enables smartd.
	RootAlways
	// RootNever marks components that install and uninstall entirely inside
	// $HOME — via Homebrew, an upstream user-scope installer, or the docker
	// socket. ctdev must never prompt for a password on their account.
	RootNever
)

type Component struct {
	Name         string
	Description  string
	Category     Category
	SupportedOS  []OS
	Dependencies []string
	Tags         []string

	// Root declares when this component needs root; see RootNeed. The zero
	// value assumes a package-manager install, so only exceptions declare it.
	Root RootNeed

	// BrewNeedsRoot marks the few components whose Homebrew install escalates.
	// Root can't answer this on its own because it isn't OS-aware: `apt install
	// docker` needs root while `brew install jq` does not, so on Homebrew the
	// default has to flip. Set it for casks that ship a pkg payload, a system
	// extension, or a privileged helper — Homebrew calls sudo itself for those,
	// from inside the progress TUI, where an un-cached credential hangs.
	BrewNeedsRoot bool

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

	// NoSudoPrompt is set when a progress TUI owns the terminal; it reaches
	// sysutil.Opts through execOpts. See sysutil.Opts.NoSudoPrompt.
	NoSudoPrompt bool
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

// InstallNeedsRoot reports whether installing these components can shell out as
// root. ctdev caches a sudo password up front only when this is true: the
// progress TUI owns the terminal once installs start, so a prompt from inside
// one would hang — but asking when nothing needs root is what broke
// shell-config-only installs in containers that have no sudo to ask with.
func InstallNeedsRoot(pm string, names []string, force bool) bool {
	for _, name := range names {
		c := FindByName(name)
		if c == nil {
			continue
		}
		if c.Root == RootNever {
			continue
		}
		// Homebrew installs into a prefix the user owns, so the usual "a package
		// install needs root" assumption is backwards there: only the components
		// flagged BrewNeedsRoot escalate. Without this a Mac would be asked for a
		// password to `brew install jq`.
		if pm == "brew" {
			if c.BrewNeedsRoot && (force || !c.IsInstalled()) {
				return true
			}
			continue
		}
		if c.Root == RootAlways {
			return true
		}
		// --force re-runs the install step even when it's already there.
		if force || !c.IsInstalled() {
			return true
		}
	}
	return false
}

// UninstallNeedsRoot reports whether removing these components needs root:
// anything that didn't install into $HOME took a package or a system path with
// it, and that has to be undone as root.
func UninstallNeedsRoot(names []string) bool {
	for _, name := range names {
		if c := FindByName(name); c != nil && c.Root != RootNever {
			return true
		}
	}
	return false
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
