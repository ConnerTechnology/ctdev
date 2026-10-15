package diagnose

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
)

// fakeProbe answers the managed checks' questions from tables, so no test
// shells out to the machine running it.
type fakeProbe struct {
	paths    map[string]string // binary name -> path on PATH
	files    map[string]bool   // absolute paths that exist
	versions map[string]string // binary path -> raw version output
	units    map[string]bool   // systemd units whose unit file is installed
	states   map[string]string // systemd unit -> `systemctl is-active` output
}

func (p fakeProbe) LookPath(name string) (string, error) {
	if path, ok := p.paths[name]; ok {
		return path, nil
	}
	return "", errors.New("not found")
}

func (p fakeProbe) FileExists(path string) bool { return p.files[path] }

func (p fakeProbe) Version(_ context.Context, path string, _ ...string) string {
	return p.versions[path]
}

func (p fakeProbe) UnitInstalled(_ context.Context, unit string) bool { return p.units[unit] }

func (p fakeProbe) UnitState(_ context.Context, unit string) string { return p.states[unit] }

func linuxFacts(components ...string) Facts {
	f := Facts{Platform: platform.Info{OS: platform.Linux}, Components: map[string]bool{}}
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
	got := managedChecks(linuxFacts(), fakeProbe{units: map[string]bool{"ssh": true}})
	if len(got) != 0 {
		t.Errorf("managedChecks with no components = %v, want none", checkIDs(got))
	}
}

func TestManagedChecksRowPerInstalledTool(t *testing.T) {
	got := checkIDs(managedChecks(linuxFacts("git", "tmux", "vscode", "claude-code"), fakeProbe{}))
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
	res := findCheck(t, managedChecks(linuxFacts("git"), p), "tool.git").Run(context.Background(), Facts{})
	if res.Severity != OK {
		t.Fatalf("severity = %v, want OK (%q)", res.Severity, res.Detail)
	}
	if res.Detail != "git version 2.43.0" {
		t.Errorf("detail = %q, want the first line of the version output", res.Detail)
	}
}

func TestToolCheckMissingBinaryFails(t *testing.T) {
	res := findCheck(t, managedChecks(linuxFacts("node"), fakeProbe{}), "tool.node").Run(context.Background(), Facts{})
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
func TestGoFallsBackToUsrLocal(t *testing.T) {
	p := fakeProbe{
		files:    map[string]bool{"/usr/local/go/bin/go": true},
		versions: map[string]string{"/usr/local/go/bin/go": "go version go1.26.0 linux/amd64"},
	}
	res := findCheck(t, managedChecks(linuxFacts("go"), p), "tool.go").Run(context.Background(), Facts{})
	if res.Severity != OK || res.Detail != "go version go1.26.0 linux/amd64" {
		t.Errorf("result = %v %q, want OK with the version", res.Severity, res.Detail)
	}
}

func TestManagedChecksServiceRows(t *testing.T) {
	p := fakeProbe{units: map[string]bool{"ssh": true}}
	got := checkIDs(managedChecks(linuxFacts("docker", "tailscale"), p))
	for _, id := range []string{"service.ssh", "service.tailscaled", "service.docker"} {
		if !slices.Contains(got, id) {
			t.Errorf("missing %s in %v", id, got)
		}
	}
}

// ssh isn't a ctdev component; its row needs the OpenSSH server unit itself.
func TestManagedChecksNoSSHRowWithoutUnit(t *testing.T) {
	got := checkIDs(managedChecks(linuxFacts("docker"), fakeProbe{}))
	if slices.Contains(got, "service.ssh") {
		t.Errorf("no ssh unit installed but got a row: %v", got)
	}
	if slices.Contains(got, "service.tailscaled") {
		t.Errorf("tailscale not installed but got a row: %v", got)
	}
}

// Services are systemd units, so other platforms get none.
func TestManagedChecksNoServicesOffLinux(t *testing.T) {
	f := linuxFacts("docker", "tailscale")
	f.Platform.OS = platform.MacOS
	p := fakeProbe{units: map[string]bool{"ssh": true}}
	for _, id := range checkIDs(managedChecks(f, p)) {
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
		res := findCheck(t, managedChecks(linuxFacts("docker"), p), "service.docker").Run(context.Background(), Facts{})
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
	for _, c := range Catalog(platform.Info{OS: platform.Linux}, Facts{Platform: platform.Info{OS: platform.Linux}}) {
		if strings.HasPrefix(c.ID, "tool.") || strings.HasPrefix(c.ID, "service.") {
			t.Errorf("Catalog without components has %s", c.ID)
		}
	}
}
