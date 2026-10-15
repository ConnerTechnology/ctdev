package diagnose

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
)

// fakeProbe answers the managed checks' questions from tables, so no test
// shells out to the machine running it.
type fakeProbe struct {
	paths    map[string]string // binary name -> path on PATH
	versions map[string]string // binary path -> raw version output
	states   map[string]string // systemd unit -> `systemctl is-active` output
}

func (p fakeProbe) LookPath(name string) (string, error) {
	if path, ok := p.paths[name]; ok {
		return path, nil
	}
	return "", errors.New("not found")
}

func (p fakeProbe) Version(_ context.Context, path string, _ ...string) string {
	return p.versions[path]
}

// UnitState answers like `systemctl is-active`: one line per unit asked.
func (p fakeProbe) UnitState(_ context.Context, units ...string) string {
	out := make([]string, len(units))
	for i, u := range units {
		out[i] = p.states[u]
	}
	return strings.Join(out, "\n")
}

func sshdStates(service, sshdService, socket, sshdSocket string) map[string]string {
	return map[string]string{
		"ssh.service": service, "sshd.service": sshdService,
		"ssh.socket": socket, "sshd.socket": sshdSocket,
	}
}

func runSSHRow(t *testing.T, states map[string]string) Result {
	t.Helper()
	f := linuxFacts("git")
	f.SSHUnitInstalled = true
	return findCheck(t, managedChecks(linuxInfo, f, fakeProbe{states: states}), "service.ssh").Run(context.Background(), Facts{})
}

func TestSSHRowServiceActiveIsRunning(t *testing.T) {
	res := runSSHRow(t, sshdStates("active", "inactive", "inactive", "inactive"))
	if res.Severity != OK || res.Detail != "running" {
		t.Errorf("result = %v %q, want OK running", res.Severity, res.Detail)
	}
}

// Newer Ubuntu starts sshd from ssh.socket, so the service is inactive until
// someone connects.
func TestSSHRowSocketActiveIsRunning(t *testing.T) {
	res := runSSHRow(t, sshdStates("inactive", "inactive", "active", "inactive"))
	if res.Severity != OK {
		t.Errorf("severity = %v (%q), want OK", res.Severity, res.Detail)
	}
}

// An installed sshd that's switched off is often deliberate on a desktop, so
// it's information, not a failure, and doctor doesn't push to enable it.
func TestSSHRowOffIsInfoNotFailure(t *testing.T) {
	res := runSSHRow(t, sshdStates("inactive", "inactive", "inactive", "inactive"))
	if res.Severity != Info {
		t.Fatalf("severity = %v, want Info", res.Severity)
	}
	if res.Detail != "off" {
		t.Errorf("detail = %q, want off", res.Detail)
	}
	if res.Advice != "" {
		t.Errorf("advice = %q, want none", res.Advice)
	}
}

func TestSSHRowUnreadableIsSkipped(t *testing.T) {
	res := runSSHRow(t, map[string]string{})
	if res.Severity != Skipped {
		t.Errorf("severity = %v, want Skipped when systemd gives no answer", res.Severity)
	}
}

func TestSSHDUnitListed(t *testing.T) {
	cases := []struct {
		name, out string
		want      bool
	}{
		{"debian", "ssh.service enabled enabled\nssh.socket disabled enabled", true},
		{"fedora", "sshd.service enabled disabled\nsshd.socket disabled disabled", true},
		{"none", "", false},
		{"unrelated", "sshd-keygen.service static -", false},
	}
	for _, tc := range cases {
		if got := sshdUnitListed(tc.out); got != tc.want {
			t.Errorf("%s: sshdUnitListed = %v, want %v", tc.name, got, tc.want)
		}
	}
}

var linuxInfo = platform.Info{OS: platform.Linux}

func linuxFacts(components ...string) Facts {
	f := Facts{Platform: linuxInfo, Components: map[string]bool{}}
	for _, c := range components {
		f.Components[c] = true
	}
	return f
}

func checkIDs(checks []Check) []string {
	ids := make([]string, 0, len(checks))
	for _, c := range checks {
		ids = append(ids, c.ID)
	}
	return ids
}

func findCheck(t *testing.T, checks []Check, id string) Check {
	t.Helper()
	for _, c := range checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no %s check in %v", id, checkIDs(checks))
	return Check{}
}

// A machine ctdev reports nothing installed on gets no tool or service rows,
// not a column of "n/a" — not even ssh, whose unit exists on plenty of
// machines ctdev has never touched.
func TestManagedChecksNothingInstalled(t *testing.T) {
	f := linuxFacts()
	f.SSHUnitInstalled = true
	got := managedChecks(linuxInfo, f, fakeProbe{})
	if len(got) != 0 {
		t.Errorf("managedChecks with no components = %v, want none", checkIDs(got))
	}
}

func TestManagedChecksRowPerInstalledTool(t *testing.T) {
	got := checkIDs(managedChecks(linuxInfo, linuxFacts("git", "tmux", "vscode", "claude-code"), fakeProbe{}))
	want := []string{"tool.git", "tool.tmux", "tool.code", "tool.claude"}
	for _, id := range want {
		if !slices.Contains(got, id) {
			t.Errorf("missing %s in %v", id, got)
		}
	}
	if slices.Contains(got, "tool.jq") {
		t.Errorf("jq is not installed but has a row: %v", got)
	}
}

func TestToolCheckShowsVersion(t *testing.T) {
	p := fakeProbe{
		paths:    map[string]string{"git": "/usr/bin/git"},
		versions: map[string]string{"/usr/bin/git": "git version 2.43.0\nsecond line\n"},
	}
	res := findCheck(t, managedChecks(linuxInfo, linuxFacts("git"), p), "tool.git").Run(context.Background(), Facts{})
	if res.Severity != OK {
		t.Fatalf("severity = %v, want OK (%q)", res.Severity, res.Detail)
	}
	if res.Detail != "git version 2.43.0" {
		t.Errorf("detail = %q, want the first line of the version output", res.Detail)
	}
}

// A binary path can carry the username (~/.local/bin, ~/.nodenv), and the
// report is meant to be shared.
func TestToolCheckWithoutVersionHidesPath(t *testing.T) {
	p := fakeProbe{paths: map[string]string{"node": "/home/example/.nodenv/shims/node"}}
	res := findCheck(t, managedChecks(linuxInfo, linuxFacts("node"), p), "tool.node").Run(context.Background(), Facts{})
	if res.Severity != OK {
		t.Fatalf("severity = %v, want OK", res.Severity)
	}
	if strings.Contains(res.Detail, "/") || strings.Contains(res.Detail, "example") {
		t.Errorf("detail = %q, must not show the binary's path", res.Detail)
	}
}

func TestToolCheckMissingBinaryFails(t *testing.T) {
	res := findCheck(t, managedChecks(linuxInfo, linuxFacts("node"), fakeProbe{}), "tool.node").Run(context.Background(), Facts{})
	if res.Severity != Fail {
		t.Fatalf("severity = %v, want Fail", res.Severity)
	}
	if !strings.Contains(res.Detail, "not on PATH") {
		t.Errorf("detail = %q, want it to say the binary is not on PATH", res.Detail)
	}
	if res.Advice == "" {
		t.Error("a failure needs advice")
	}
}

// The official Go tarball lands in /usr/local/go/bin, which a fresh shell may
// not have on PATH yet.
func TestGoToolFallsBackToUsrLocal(t *testing.T) {
	i := slices.IndexFunc(managedTools, func(m managedTool) bool { return m.component == "go" })
	if i < 0 || managedTools[i].fallback != "/usr/local/go/bin/go" {
		t.Fatalf("go tool entry should fall back to /usr/local/go/bin/go")
	}
}

func TestToolCheckUsesFallbackOffPath(t *testing.T) {
	fallback := filepath.Join(t.TempDir(), "go")
	if err := os.WriteFile(fallback, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	p := fakeProbe{versions: map[string]string{fallback: "go version go1.26.0 linux/amd64"}}
	tool := managedTool{component: "go", binary: "go", args: []string{"version"}, fallback: fallback}
	res := toolCheck(tool, p).Run(context.Background(), Facts{})
	if res.Severity != OK || res.Detail != "go version go1.26.0 linux/amd64" {
		t.Errorf("result = %v %q, want OK with the version", res.Severity, res.Detail)
	}
}

func TestManagedChecksServiceRows(t *testing.T) {
	f := linuxFacts("docker", "tailscale")
	f.SSHUnitInstalled = true
	got := checkIDs(managedChecks(linuxInfo, f, fakeProbe{}))
	for _, id := range []string{"service.ssh", "service.tailscaled", "service.docker"} {
		if !slices.Contains(got, id) {
			t.Errorf("missing %s in %v", id, got)
		}
	}
}

// ssh isn't a ctdev component; its row needs the OpenSSH server unit itself.
func TestManagedChecksNoSSHRowWithoutUnit(t *testing.T) {
	got := checkIDs(managedChecks(linuxInfo, linuxFacts("docker"), fakeProbe{}))
	if slices.Contains(got, "service.ssh") {
		t.Errorf("no ssh unit installed but got a row: %v", got)
	}
	if slices.Contains(got, "service.tailscaled") {
		t.Errorf("tailscale not installed but got a row: %v", got)
	}
}

// Services are systemd units, so other platforms get none. The gate is the
// platform Catalog was asked for.
func TestManagedChecksNoServicesOffLinux(t *testing.T) {
	f := linuxFacts("docker", "tailscale")
	f.SSHUnitInstalled = true
	for _, id := range checkIDs(managedChecks(platform.Info{OS: platform.MacOS}, f, fakeProbe{})) {
		if strings.HasPrefix(id, "service.") {
			t.Errorf("service row %s on macOS", id)
		}
	}
}

func TestServiceCheckSeverity(t *testing.T) {
	cases := []struct {
		state string
		want  Severity
	}{
		{"active", OK},
		{"inactive", Fail},
		{"failed", Fail},
		{"activating", Fail},
		{"", Fail},
	}
	for _, tc := range cases {
		p := fakeProbe{states: map[string]string{"docker": tc.state}}
		res := findCheck(t, managedChecks(linuxInfo, linuxFacts("docker"), p), "service.docker").Run(context.Background(), Facts{})
		if res.Severity != tc.want {
			t.Errorf("state %q: severity = %v, want %v", tc.state, res.Severity, tc.want)
		}
		if tc.want == Fail && res.Advice == "" {
			t.Errorf("state %q: a failure needs advice", tc.state)
		}
	}
}

// Catalog only adds these rows when the caller filled Facts.Components; the
// package itself assumes nothing.
func TestCatalogHasNoToolRowsWithoutComponents(t *testing.T) {
	f := Facts{Platform: linuxInfo, SSHUnitInstalled: true}
	for _, c := range Catalog(linuxInfo, f) {
		if strings.HasPrefix(c.ID, "tool.") || strings.HasPrefix(c.ID, "service.") {
			t.Errorf("Catalog without components has %s", c.ID)
		}
	}
}

func TestCaptureCombinedKeepsStderr(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	got := captureCombined(context.Background(), sh, "-c", "echo out; echo err 1>&2")
	if !strings.Contains(got, "out") || !strings.Contains(got, "err") {
		t.Errorf("captureCombined = %q, want stdout and stderr", got)
	}
}
