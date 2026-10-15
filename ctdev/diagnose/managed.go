package diagnose

import (
	"context"
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
// component, so a Fact about its own unit file says whether it's installed.
type managedService struct {
	unit      string
	component string
}

var managedServices = []managedService{
	{unit: "ssh"}, // the remote-access lifeline; enabled by `ctdev configure ssh`
	{unit: "tailscaled", component: "tailscale"},
	{unit: "docker", component: "docker"},
}

// managedProbe is what the managed checks ask the machine while they run, so
// tests can answer from a table instead of shelling out. Building the catalog
// asks it nothing: everything the gates need is already in Facts.
type managedProbe interface {
	LookPath(name string) (string, error)
	// Version returns a tool's raw version output, stdout and stderr together.
	Version(ctx context.Context, path string, args ...string) string
	UnitState(ctx context.Context, unit string) string
}

// managedChecks builds a row per installed tool and per installed service.
// Nothing installed means no rows.
func managedChecks(info platform.Info, f Facts, p managedProbe) []Check {
	if len(f.Components) == 0 {
		return nil
	}
	var checks []Check
	for _, t := range managedTools {
		if f.Components[t.component] {
			checks = append(checks, toolCheck(t, p))
		}
	}
	if info.OS != platform.Linux {
		return checks
	}
	for _, s := range managedServices {
		installed := f.Components[s.component]
		if s.component == "" {
			installed = s.unit == "ssh" && f.SSHUnitInstalled
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
			if err != nil && t.fallback != "" && pathExists(t.fallback) {
				path, err = t.fallback, nil
			}
			if err != nil {
				return failf("Run 'ctdev install "+t.component+"' to put it back, then open a new shell.",
					"%s is installed but %s is not on PATH", t.component, t.binary)
			}
			version := firstLine(p.Version(ctx, path, t.args...))
			if version == "" {
				// Not the path: it can carry the username, and reports get shared.
				return okf("installed")
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

// Version keeps stderr, unlike capture: some tools print their version there.
func (systemProbe) Version(ctx context.Context, path string, args ...string) string {
	return captureCombined(ctx, path, args...)
}

func (systemProbe) UnitState(ctx context.Context, unit string) string {
	// is-active exits non-zero for anything but active; the output is the answer.
	out, _ := captureErr(ctx, "systemctl", "is-active", unit)
	return out
}

// sshUnitInstalled reports whether the OpenSSH server's systemd unit is
// installed. GatherFacts settles it, so building the catalog runs no
// subprocess.
func sshUnitInstalled(ctx context.Context, info platform.Info) bool {
	if info.OS != platform.Linux || !commandExists("systemctl") {
		return false
	}
	out := capture(ctx, "systemctl", "list-unit-files", "ssh.service", "--no-legend")
	return strings.Contains(out, "ssh.service")
}
