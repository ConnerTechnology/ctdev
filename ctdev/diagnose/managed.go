package diagnose

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
)

// The managed checks cover what ctdev itself installed: each tool's binary is
// on PATH, and each service ctdev depends on is running. They are gated on
// Facts.Components, which the caller fills from ctdev's component detection;
// this package never looks at the registry itself, so on its own it still
// assumes nothing about the machine.

// managedTool is a ctdev component that ships a binary worth asking for its
// version.
type managedTool struct {
	component string
	binary    string
	args      []string
	// fallback is where the binary lives when PATH doesn't have it yet.
	fallback string
}

var managedTools = []managedTool{
	{component: "git", binary: "git", args: []string{"--version"}},
	{component: "zsh", binary: "zsh", args: []string{"--version"}},
	{component: "tmux", binary: "tmux", args: []string{"-V"}},
	{component: "jq", binary: "jq", args: []string{"--version"}},
	{component: "node", binary: "node", args: []string{"--version"}},
	{component: "python", binary: "python", args: []string{"--version"}},
	{component: "docker", binary: "docker", args: []string{"--version"}},
	{component: "vscode", binary: "code", args: []string{"--version"}},
	{component: "tailscale", binary: "tailscale", args: []string{"version"}},
	{component: "claude-code", binary: "claude", args: []string{"--version"}},
	{component: "gh", binary: "gh", args: []string{"--version"}},
	{component: "devcontainer", binary: "devcontainer", args: []string{"--version"}},
	// The official tarball installs to /usr/local/go/bin, which a shell
	// started before the install doesn't have on PATH.
	{component: "go", binary: "go", args: []string{"version"}, fallback: "/usr/local/go/bin/go"},
}

// managedService is a systemd unit that has to be running for something ctdev
// installed to work. An empty component means the unit isn't a ctdev
// component, so its own unit file is what says it's installed.
type managedService struct {
	unit      string
	component string
}

var managedServices = []managedService{
	{unit: "ssh"}, // the remote-access lifeline; enabled by `ctdev configure ssh`
	{unit: "tailscaled", component: "tailscale"},
	{unit: "docker", component: "docker"},
}

// managedProbe is everything the managed checks ask the machine, so tests can
// answer from a table instead of shelling out.
type managedProbe interface {
	LookPath(name string) (string, error)
	FileExists(path string) bool
	// Version returns a tool's raw version output, stdout and stderr together.
	Version(ctx context.Context, path string, args ...string) string
	UnitInstalled(ctx context.Context, unit string) bool
	UnitState(ctx context.Context, unit string) string
}

// managedChecks builds a row per installed tool and per installed service.
// Nothing installed means no rows.
func managedChecks(f Facts, p managedProbe) []Check {
	if len(f.Components) == 0 {
		return nil
	}
	var checks []Check
	for _, t := range managedTools {
		if f.Components[t.component] {
			checks = append(checks, toolCheck(t, p))
		}
	}
	if f.Platform.OS != platform.Linux {
		return checks
	}
	for _, s := range managedServices {
		installed := f.Components[s.component]
		if s.component == "" {
			installed = p.UnitInstalled(context.Background(), s.unit)
		}
		if installed {
			checks = append(checks, serviceCheck(s, p))
		}
	}
	return checks
}

func toolCheck(t managedTool, p managedProbe) Check {
	return Check{
		ID:    "tool." + t.binary,
		Name:  t.binary,
		Group: GroupTools,
		Run: func(ctx context.Context, _ Facts) Result {
			path, err := p.LookPath(t.binary)
			if err != nil && t.fallback != "" && p.FileExists(t.fallback) {
				path, err = t.fallback, nil
			}
			if err != nil {
				return failf("Run 'ctdev install "+t.component+"' to put it back, then open a new shell.",
					"%s is installed but %s is not on PATH", t.component, t.binary)
			}
			version := firstLine(p.Version(ctx, path, t.args...))
			if version == "" {
				return okf("%s", path)
			}
			return okf("%s", version)
		},
	}
}

func serviceCheck(s managedService, p managedProbe) Check {
	return Check{
		ID:    "service." + s.unit,
		Name:  s.unit,
		Group: GroupSystem,
		Run: func(ctx context.Context, _ Facts) Result {
			state := strings.TrimSpace(p.UnitState(ctx, s.unit))
			if state == "active" {
				return okf("running")
			}
			if state == "" {
				state = "unknown"
			}
			return failf("Start it with 'sudo systemctl enable --now "+s.unit+"'. 'systemctl status "+s.unit+"' shows why it stopped.",
				"%s is %s", s.unit, state)
		},
	}
}

// systemProbe answers from the machine itself. Every call is read-only and
// needs no root.
type systemProbe struct{}

func (systemProbe) LookPath(name string) (string, error) { return exec.LookPath(name) }

func (systemProbe) FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Version keeps stderr, unlike capture: some tools print their version there.
func (systemProbe) Version(ctx context.Context, path string, args ...string) string {
	out, _ := exec.CommandContext(ctx, path, args...).CombinedOutput()
	return string(out)
}

func (systemProbe) UnitInstalled(ctx context.Context, unit string) bool {
	if !commandExists("systemctl") {
		return false
	}
	out := capture(ctx, "systemctl", "list-unit-files", unit+".service", "--no-legend")
	return strings.Contains(out, unit+".service")
}

func (systemProbe) UnitState(ctx context.Context, unit string) string {
	// is-active exits non-zero for anything but active; the output is the answer.
	out, _ := captureErr(ctx, "systemctl", "is-active", unit)
	return out
}
