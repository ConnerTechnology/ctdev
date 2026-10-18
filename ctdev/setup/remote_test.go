package setup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// fakeRemote records every remote-access shell-out instead of running it.
// Root commands are recorded with a leading "sudo".
type fakeRemote struct {
	calls    [][]string
	probes   map[string]string // "name args..." → stdout
	commands map[string]bool   // binaries on PATH
	status   string            // `ufw status` output
	stateDir string
}

func useFakeRemote(t *testing.T) *fakeRemote {
	t.Helper()
	f := &fakeRemote{
		probes:   map[string]string{},
		commands: map[string]bool{"code": true, "ufw": true},
		stateDir: t.TempDir(),
	}
	origSudo, origRun, origWrite := remoteSudo, remoteRun, remoteWriteFile
	origProbe, origSudoProbe, origUser := remoteProbe, remoteSudoProbe, remoteUsername
	origState, origExists, origSSHD, origUFWConf := remoteStateDir, remoteCommandExists, sshdPath, ufwConfPath
	remoteSudo = func(_ context.Context, _ sysutil.Opts, name string, args ...string) error {
		f.calls = append(f.calls, append([]string{"sudo", name}, args...))
		return nil
	}
	remoteRun = func(_ context.Context, _ sysutil.Opts, name string, args ...string) error {
		f.calls = append(f.calls, append([]string{name}, args...))
		return nil
	}
	remoteWriteFile = func(_ context.Context, _ sysutil.Opts, _, path string) error {
		f.calls = append(f.calls, []string{"write", path})
		return nil
	}
	remoteProbe = func(_ context.Context, name string, args ...string) (string, error) {
		out, ok := f.probes[strings.Join(append([]string{name}, args...), " ")]
		if !ok {
			return "", errors.New("exit status 1")
		}
		return out, nil
	}
	remoteSudoProbe = func(_ context.Context, _ string, _ ...string) (string, error) { return f.status, nil }
	remoteUsername = func() (string, error) { return "tester", nil }
	remoteStateDir = func() string { return f.stateDir }
	remoteCommandExists = func(name string) bool { return f.commands[name] }
	sshdPath = filepath.Join(t.TempDir(), "sshd")
	if err := os.WriteFile(sshdPath, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	ufwConfPath = filepath.Join(t.TempDir(), "ufw.conf")
	t.Cleanup(func() {
		remoteSudo, remoteRun, remoteWriteFile = origSudo, origRun, origWrite
		remoteProbe, remoteSudoProbe, remoteUsername = origProbe, origSudoProbe, origUser
		remoteStateDir, remoteCommandExists, sshdPath, ufwConfPath = origState, origExists, origSSHD, origUFWConf
	})
	return f
}

func (f *fakeRemote) ran(cmd ...string) bool {
	for _, c := range f.calls {
		if reflect.DeepEqual(c, cmd) {
			return true
		}
	}
	return false
}

func (f *fakeRemote) index(cmd ...string) int {
	for i, c := range f.calls {
		if reflect.DeepEqual(c, cmd) {
			return i
		}
	}
	return -1
}

var quietOpts = sysutil.Opts{Stdout: &strings.Builder{}}

// The six remote-access settings all live under `configure ssh`, and the
// categories they came from are gone.
func TestRemoteAccessSettingsAreUnderSSH(t *testing.T) {
	want := []string{
		"SSH server",
		"Key-only login",
		"Keep my programs running after I disconnect",
		"Mosh support",
		"Open this machine from vscode.dev",
		"Only allow SSH and Mosh from my local network (firewall)",
	}
	var got []string
	for _, s := range FilterBySlug(Registry, "ssh") {
		got = append(got, s.Name)
		if s.OneWay {
			t.Errorf("%q is one-way; every remote-access setting needs an off switch", s.Name)
		}
		if s.Description == "" {
			t.Errorf("%q has no one-line why", s.Name)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ssh settings = %q, want %q", got, want)
	}
	for _, gone := range []string{"linger", "locale", "tunnel", "ufw"} {
		if n := len(FilterBySlug(Registry, gone)); n != 0 {
			t.Errorf("slug %q still has %d settings", gone, n)
		}
	}
}

func TestSSHServerDefaultIsOnDemand(t *testing.T) {
	for _, s := range FilterBySlug(Registry, "ssh") {
		if s.Name == "SSH server" && s.Default != sshOnDemand {
			t.Errorf("SSH server default = %q, want %q", s.Default, sshOnDemand)
		}
	}
}

func TestDetectSSHServer(t *testing.T) {
	tests := []struct {
		name   string
		probes map[string]string
		want   string
	}{
		{"socket enabled", map[string]string{"systemctl is-enabled ssh.socket": "enabled", "systemctl is-active ssh.service": "inactive"}, sshOnDemand},
		{"service only", map[string]string{"systemctl is-enabled ssh.socket": "disabled", "systemctl is-active ssh.service": "active"}, sshAlwaysOn},
		{"neither", map[string]string{"systemctl is-enabled ssh.socket": "disabled", "systemctl is-active ssh.service": "inactive"}, sshOff},
		{"not installed", map[string]string{}, sshOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := useFakeRemote(t)
			f.probes = tt.probes
			if got := detectSSHServer(context.Background()); got != tt.want {
				t.Errorf("detectSSHServer = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplySSHServerModes(t *testing.T) {
	tests := []struct {
		mode string
		want [][]string
	}{
		{sshOnDemand, [][]string{
			{"sudo", "systemctl", "disable", "--now", "ssh.service"},
			{"sudo", "systemctl", "enable", "--now", "ssh.socket"},
		}},
		{sshAlwaysOn, [][]string{
			{"sudo", "systemctl", "disable", "--now", "ssh.socket"},
			{"sudo", "systemctl", "enable", "--now", "ssh.service"},
		}},
		{sshOff, [][]string{
			{"sudo", "systemctl", "disable", "--now", "ssh.socket", "ssh.service"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			f := useFakeRemote(t)
			if err := applySSHServer(context.Background(), quietOpts, tt.mode); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(f.calls, tt.want) {
				t.Errorf("calls = %q, want %q", f.calls, tt.want)
			}
		})
	}
}

func TestApplySSHServerInstallsOpenSSHWhenMissing(t *testing.T) {
	f := useFakeRemote(t)
	sshdPath = filepath.Join(t.TempDir(), "missing")
	if err := applySSHServer(context.Background(), quietOpts, sshOnDemand); err != nil {
		t.Fatal(err)
	}
	if f.index("sudo", "apt-get", "install", "-y", "-qq", "openssh-server") != 0 {
		t.Errorf("openssh-server should install first: %q", f.calls)
	}
}

func TestApplySSHKeyOnlyOff(t *testing.T) {
	f := useFakeRemote(t)
	if err := applySSHKeyOnly(context.Background(), quietOpts, "disabled"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"sudo", "rm", "-f", sshdDropIn},
		{"sudo", "systemctl", "try-reload-or-restart", "ssh.service"},
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("calls = %q, want %q", f.calls, want)
	}
}

func TestApplySSHKeyOnlyOn(t *testing.T) {
	f := useFakeRemote(t)
	t.Setenv("HOME", t.TempDir())
	if err := applySSHKeyOnly(context.Background(), quietOpts, "enabled"); err != nil {
		t.Fatal(err)
	}
	if !f.ran("write", sshdDropIn) || !f.ran("sudo", "sshd", "-t") {
		t.Errorf("expected drop-in written and validated: %q", f.calls)
	}
}

func TestApplyLingerOnAndOff(t *testing.T) {
	f := useFakeRemote(t)
	_ = applyLinger(context.Background(), quietOpts, "enabled")
	_ = applyLinger(context.Background(), quietOpts, "disabled")
	want := [][]string{
		{"sudo", "loginctl", "enable-linger", "tester"},
		{"sudo", "loginctl", "disable-linger", "tester"},
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("calls = %q, want %q", f.calls, want)
	}
}

func TestDetectLinger(t *testing.T) {
	f := useFakeRemote(t)
	f.probes["loginctl show-user tester --property=Linger"] = "Linger=yes"
	if got := detectLinger(context.Background()); got != "enabled" {
		t.Errorf("detectLinger = %q, want enabled", got)
	}
}

// Turning on vscode.dev also turns on "Keep my programs running", before the
// tunnel service is installed.
func TestApplyCodeTunnelOnTurnsOnLinger(t *testing.T) {
	f := useFakeRemote(t)
	if err := applyCodeTunnel(context.Background(), quietOpts, "enabled"); err != nil {
		t.Fatal(err)
	}
	linger := f.index("sudo", "loginctl", "enable-linger", "tester")
	install := f.index("code", "tunnel", "service", "install", "--accept-server-license-terms")
	if linger < 0 || install < 0 || linger > install {
		t.Errorf("want enable-linger before the tunnel install, got %q", f.calls)
	}
}

func TestApplyCodeTunnelOff(t *testing.T) {
	f := useFakeRemote(t)
	if err := applyCodeTunnel(context.Background(), quietOpts, "disabled"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"code", "tunnel", "service", "uninstall"}}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("calls = %q, want %q", f.calls, want)
	}
}

func TestApplyMoshLocaleOnRemembersItAddedTheLocale(t *testing.T) {
	f := useFakeRemote(t)
	f.probes["locale -a"] = "C\nC.utf8\nPOSIX"
	if err := applyMoshLocale(context.Background(), quietOpts, "enabled"); err != nil {
		t.Fatal(err)
	}
	if !f.ran("sudo", "locale-gen", "en_US.UTF-8") {
		t.Errorf("expected locale-gen: %q", f.calls)
	}
	if _, err := os.Stat(filepath.Join(f.stateDir, moshLocaleMarker)); err != nil {
		t.Errorf("marker not written: %v", err)
	}
}

func TestApplyMoshLocaleOnLeavesAnExistingLocaleUnclaimed(t *testing.T) {
	f := useFakeRemote(t)
	f.probes["locale -a"] = "C\nen_US.utf8"
	if err := applyMoshLocale(context.Background(), quietOpts, "enabled"); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Errorf("locale already there; want no calls, got %q", f.calls)
	}
	if _, err := os.Stat(filepath.Join(f.stateDir, moshLocaleMarker)); err == nil {
		t.Error("marker written for a locale ctdev didn't add")
	}
}

func TestApplyMoshLocaleOffRemovesOnlyWhatCtdevAdded(t *testing.T) {
	t.Run("ctdev added it", func(t *testing.T) {
		f := useFakeRemote(t)
		marker := filepath.Join(f.stateDir, moshLocaleMarker)
		if err := os.WriteFile(marker, []byte("en_US.UTF-8\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := applyMoshLocale(context.Background(), quietOpts, "disabled"); err != nil {
			t.Fatal(err)
		}
		if !f.ran("sudo", "localedef", "--delete-from-archive", "en_US.utf8") {
			t.Errorf("expected the locale deleted: %q", f.calls)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Error("marker should be removed")
		}
	})
	t.Run("it was already there", func(t *testing.T) {
		f := useFakeRemote(t)
		if err := applyMoshLocale(context.Background(), quietOpts, "disabled"); err != nil {
			t.Fatal(err)
		}
		if len(f.calls) != 0 {
			t.Errorf("want no calls, got %q", f.calls)
		}
	})
}

func TestApplyUFWOn(t *testing.T) {
	f := useFakeRemote(t)
	if err := applyUFW(context.Background(), quietOpts, "enabled"); err != nil {
		t.Fatal(err)
	}
	if !f.ran("sudo", "ufw", "allow", "from", "192.168.0.0/16", "to", "any", "port", "22", "proto", "tcp") ||
		!f.ran("sudo", "ufw", "allow", "from", "10.0.0.0/8", "to", "any", "port", "60000:61000", "proto", "udp") {
		t.Errorf("missing LAN allow rules: %q", f.calls)
	}
	if last := f.calls[len(f.calls)-1]; !reflect.DeepEqual(last, []string{"sudo", "ufw", "--force", "enable"}) {
		t.Errorf("last call = %q, want ufw --force enable", last)
	}
}

func TestApplyUFWOff(t *testing.T) {
	t.Run("only ctdev's rules", func(t *testing.T) {
		f := useFakeRemote(t)
		f.status = "Status: active"
		if err := applyUFW(context.Background(), quietOpts, "disabled"); err != nil {
			t.Fatal(err)
		}
		if !f.ran("sudo", "ufw", "delete", "allow", "from", "172.16.0.0/12", "to", "any", "port", "22", "proto", "tcp") {
			t.Errorf("ctdev's rules not deleted: %q", f.calls)
		}
		if !f.ran("sudo", "ufw", "--force", "disable") {
			t.Errorf("UFW not turned off: %q", f.calls)
		}
	})
	t.Run("other rules stay up", func(t *testing.T) {
		f := useFakeRemote(t)
		f.status = "Status: active\n\nTo                         Action      From\n--                         ------      ----\n53                         ALLOW       Anywhere"
		if err := applyUFW(context.Background(), quietOpts, "disabled"); err != nil {
			t.Fatal(err)
		}
		if f.ran("sudo", "ufw", "--force", "disable") {
			t.Error("UFW turned off while other rules remain")
		}
	})
}

func TestDetectUFW(t *testing.T) {
	useFakeRemote(t)
	if got := detectUFW(context.Background()); got != "disabled" {
		t.Errorf("no ufw.conf: got %q, want disabled", got)
	}
	if err := os.WriteFile(ufwConfPath, []byte("# comment\nENABLED=yes\nLOGLEVEL=low\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := detectUFW(context.Background()); got != "enabled" {
		t.Errorf("ENABLED=yes: got %q, want enabled", got)
	}
}
