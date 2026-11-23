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
	sudoOut  map[string]string // root-only probes: "name args..." → stdout
	stateDir string
	home     string
	onSudo   func(cmd []string) // optional side effect of a root command
}

func useFakeRemote(t *testing.T) *fakeRemote {
	t.Helper()
	f := &fakeRemote{
		probes:   map[string]string{},
		commands: map[string]bool{"code": true, "ufw": true},
		sudoOut:  map[string]string{},
		stateDir: t.TempDir(),
		home:     t.TempDir(),
	}
	t.Setenv("HOME", f.home)
	origSudo, origRun, origWrite := remoteSudo, remoteRun, remoteWriteFile
	origProbe, origSudoProbe, origUser := remoteProbe, remoteSudoProbe, remoteUsername
	origState, origExists, origSSHD, origUFWConf := remoteStateDir, remoteCommandExists, sshdPath, ufwConfPath
	origInstall, origMigration := remoteInstall, sshSocketMigrationDropIn
	remoteSudo = func(_ context.Context, _ sysutil.Opts, name string, args ...string) error {
		cmd := append([]string{"sudo", name}, args...)
		f.calls = append(f.calls, cmd)
		if f.onSudo != nil {
			f.onSudo(cmd)
		}
		return nil
	}
	remoteInstall = func(ctx context.Context, o sysutil.Opts, pkgs []string) error {
		return remoteSudo(ctx, o, "apt-get", append([]string{"install", "-y", "-qq"}, pkgs...)...)
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
	remoteSudoProbe = func(_ context.Context, name string, args ...string) (string, error) {
		out, ok := f.sudoOut[strings.Join(append([]string{name}, args...), " ")]
		if !ok {
			return "", errors.New("exit status 1")
		}
		return out, nil
	}
	remoteUsername = func() (string, error) { return "tester", nil }
	remoteStateDir = func() string { return f.stateDir }
	remoteCommandExists = func(name string) bool { return f.commands[name] }
	sshdPath = filepath.Join(t.TempDir(), "sshd")
	if err := os.WriteFile(sshdPath, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	ufwConfPath = filepath.Join(t.TempDir(), "ufw.conf")
	sshSocketMigrationDropIn = filepath.Join(t.TempDir(), "00-socket.conf")
	t.Cleanup(func() {
		remoteInstall, sshSocketMigrationDropIn = origInstall, origMigration
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

// `configure ssh` offers exactly the two server settings (ADR-0023), and the
// categories remote access once lived in are gone.
func TestRemoteAccessSettingsAreUnderSSH(t *testing.T) {
	want := []string{
		"SSH server",
		"Key-only login",
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

// `ctdev install ssh` turns an off server on with the default mode, and keeps
// a mode already chosen in `configure ssh`.
func TestEnsureSSHServer(t *testing.T) {
	tests := []struct {
		name   string
		probes map[string]string
		want   []string
	}{
		{"off gets on demand", map[string]string{}, []string{"sudo", "systemctl", "enable", "--now", "ssh.socket"}},
		{"always on stays", map[string]string{"systemctl is-active ssh.service": "active"}, []string{"sudo", "systemctl", "enable", "--now", "ssh.service"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := useFakeRemote(t)
			f.probes = tt.probes
			if err := EnsureSSHServer(context.Background(), quietOpts); err != nil {
				t.Fatal(err)
			}
			if !f.ran(tt.want...) {
				t.Errorf("calls = %q, want %q among them", f.calls, tt.want)
			}
		})
	}
}

func TestDisableSSHServerWithoutOpenSSHDoesNothing(t *testing.T) {
	f := useFakeRemote(t)
	sshdPath = filepath.Join(t.TempDir(), "missing")
	if err := DisableSSHServer(context.Background(), quietOpts); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Errorf("calls = %q, want none", f.calls)
	}
}

func TestSSHServerStartsAtBoot(t *testing.T) {
	tests := []struct {
		name      string
		installed bool
		probes    map[string]string
		want      bool
	}{
		{"socket enabled", true, map[string]string{"systemctl is-enabled ssh.socket": "enabled"}, true},
		{"service enabled", true, map[string]string{"systemctl is-enabled ssh.service": "enabled"}, true},
		{"running but not enabled", true, map[string]string{"systemctl is-active ssh.service": "active"}, false},
		{"not installed", false, map[string]string{"systemctl is-enabled ssh.socket": "enabled"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := useFakeRemote(t)
			f.probes = tt.probes
			if !tt.installed {
				sshdPath = filepath.Join(t.TempDir(), "missing")
			}
			if got := SSHServerStartsAtBoot(context.Background()); got != tt.want {
				t.Errorf("SSHServerStartsAtBoot = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplySSHKeyOnlyOff(t *testing.T) {
	f := useFakeRemote(t)
	if err := applySSHKeyOnly(context.Background(), quietOpts, "disabled"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"sudo", "rm", "-f", sshdDropIn, sshdLegacyDropIn},
		{"sudo", "systemctl", "try-reload-or-restart", "ssh.service"},
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("calls = %q, want %q", f.calls, want)
	}
}

func TestApplySSHKeyOnlyOn(t *testing.T) {
	f := useFakeRemote(t)
	if err := applySSHKeyOnly(context.Background(), quietOpts, "enabled"); err != nil {
		t.Fatal(err)
	}
	if !f.ran("write", sshdDropIn) || !f.ran("sudo", "sshd", "-t") {
		t.Errorf("expected drop-in written and validated: %q", f.calls)
	}
}

// ctdevUFWStatus is `ufw status` with every rule ctdev added.
func ctdevUFWStatus() string {
	var b strings.Builder
	b.WriteString("Status: active\n\nTo                         Action      From\n--                         ------      ----\n")
	for _, r := range ufwRules() {
		b.WriteString(r.port + "/" + r.proto + "                     ALLOW       " + r.cidr + "\n")
	}
	return b.String()
}

func writeAuthorizedKey(t *testing.T, f *fakeRemote) {
	t.Helper()
	dir := filepath.Join(f.home, ".ssh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "authorized_keys"), []byte("ssh-ed25519 AAAAexample test@example.invalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDetectSSHKeyOnly(t *testing.T) {
	t.Run("drop-in present, no key", func(t *testing.T) {
		f := useFakeRemote(t)
		f.sudoOut["sshd -T"] = "pubkeyauthentication yes\npasswordauthentication yes"
		if got := detectSSHKeyOnly(context.Background()); got != "disabled" {
			t.Errorf("got %q, want disabled", got)
		}
	})
	t.Run("key, passwords still on", func(t *testing.T) {
		f := useFakeRemote(t)
		writeAuthorizedKey(t, f)
		f.sudoOut["test -e "+sshdDropIn] = ""
		f.sudoOut["sshd -T"] = "pubkeyauthentication yes\npasswordauthentication yes"
		if got := detectSSHKeyOnly(context.Background()); got != "disabled" {
			t.Errorf("got %q, want disabled", got)
		}
	})
	t.Run("key, passwords off", func(t *testing.T) {
		f := useFakeRemote(t)
		writeAuthorizedKey(t, f)
		f.sudoOut["test -e "+sshdDropIn] = ""
		f.sudoOut["sshd -T"] = "pubkeyauthentication yes\npasswordauthentication no"
		if got := detectSSHKeyOnly(context.Background()); got != "enabled" {
			t.Errorf("got %q, want enabled", got)
		}
	})
	t.Run("passwords off by another file, no ctdev drop-in", func(t *testing.T) {
		f := useFakeRemote(t)
		writeAuthorizedKey(t, f)
		f.sudoOut["sshd -T"] = "pubkeyauthentication yes\npasswordauthentication no"
		if got := detectSSHKeyOnly(context.Background()); got != "disabled" {
			t.Errorf("got %q, want disabled", got)
		}
	})
	t.Run("sshd -T unreadable", func(t *testing.T) {
		f := useFakeRemote(t)
		writeAuthorizedKey(t, f)
		if got := detectSSHKeyOnly(context.Background()); got != ValueUnknown {
			t.Errorf("got %q, want %q", got, ValueUnknown)
		}
	})
}

// ctdev's drop-in must sort ahead of drop-ins like 50-cloud-init.conf,
// because sshd keeps the first value it reads.
func TestSSHDDropInSortsFirst(t *testing.T) {
	if name := filepath.Base(sshdDropIn); name >= "50-cloud-init.conf" {
		t.Errorf("drop-in %q sorts after 50-cloud-init.conf", name)
	}
}

func TestApplySSHKeyOnlyRemovesLegacyDropIn(t *testing.T) {
	f := useFakeRemote(t)
	if err := applySSHKeyOnly(context.Background(), quietOpts, "enabled"); err != nil {
		t.Fatal(err)
	}
	if !f.ran("sudo", "rm", "-f", sshdLegacyDropIn) {
		t.Errorf("legacy 99-ctdev.conf not removed: %q", f.calls)
	}
}

func TestApplySSHKeyOnlyWarnsWhenOverridden(t *testing.T) {
	f := useFakeRemote(t)
	writeAuthorizedKey(t, f)
	f.sudoOut["sshd -T"] = "passwordauthentication yes"
	var out strings.Builder
	if err := applySSHKeyOnly(context.Background(), sysutil.Opts{Stdout: &out}, "enabled"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "another sshd setting") {
		t.Errorf("want an override warning, got %q", out.String())
	}
}

func TestApplySSHKeyOnlySkipsWithoutSSHD(t *testing.T) {
	f := useFakeRemote(t)
	sshdPath = filepath.Join(t.TempDir(), "missing")
	var out strings.Builder
	if err := applySSHKeyOnly(context.Background(), sysutil.Opts{Stdout: &out}, "enabled"); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 || !strings.Contains(out.String(), "skipped: no sshd") {
		t.Errorf("want skipped with no calls, got %q / %q", out.String(), f.calls)
	}
}

// One `configure ssh --batch` run on a machine without openssh-server: the
// SSH server setting installs it, and key-only login, applied after it in
// the same run, still writes its drop-in.
func TestBatchDeliversKeyOnlyOnAFreshMachine(t *testing.T) {
	f := useFakeRemote(t)
	sshdPath = filepath.Join(t.TempDir(), "sshd")
	f.onSudo = func(cmd []string) {
		if len(cmd) > 2 && cmd[1] == "apt-get" && cmd[len(cmd)-1] == "openssh-server" {
			_ = os.WriteFile(sshdPath, nil, 0o755)
		}
	}
	var keyOnly *Setting
	for i := range Registry {
		if Registry[i].Name == "Key-only login" {
			keyOnly = &Registry[i]
		}
	}
	if keyOnly == nil {
		t.Fatal("Key-only login not in Registry")
	}
	if gateLinux() && !keyOnly.HardwareFn() {
		t.Fatal("Key-only login is hidden before sshd is installed")
	}
	if err := applySSHServer(context.Background(), quietOpts, sshOnDemand); err != nil {
		t.Fatal(err)
	}
	if err := applySSHKeyOnly(context.Background(), quietOpts, "enabled"); err != nil {
		t.Fatal(err)
	}
	if !f.ran("write", sshdDropIn) {
		t.Errorf("drop-in not written in the same run: %q", f.calls)
	}
}

func TestApplySSHServerAlwaysOnNotesSocketMigrationDropIn(t *testing.T) {
	f := useFakeRemote(t)
	if err := os.WriteFile(sshSocketMigrationDropIn, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := applySSHServer(context.Background(), sysutil.Opts{Stdout: &out}, sshAlwaysOn); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "discourse.ubuntu.com") {
		t.Errorf("want a note pointing at Ubuntu's revert steps, got %q", out.String())
	}
	for _, c := range f.calls {
		if c[1] == "rm" {
			t.Errorf("the migration drop-in must not be deleted: %q", c)
		}
	}
}

func writeTunnelService(t *testing.T, f *fakeRemote) {
	t.Helper()
	dir := filepath.Join(f.home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "code-tunnel.service"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUFWHasCtdevRules(t *testing.T) {
	if !ufwHasCtdevRules(ctdevUFWStatus()) {
		t.Error("full ctdev status should match")
	}
	partial := strings.Replace(ctdevUFWStatus(), "60000:61000/udp                     ALLOW       10.0.0.0/8\n", "", 1)
	if ufwHasCtdevRules(partial) {
		t.Error("a missing Mosh rule should not match")
	}
}

// ctdevAddedEverything sets up a machine where ctdev turned on all four
// retired settings, with ctdev's UFW rules the only ones.
func ctdevAddedEverything(t *testing.T) *fakeRemote {
	t.Helper()
	f := useFakeRemote(t)
	writeTunnelService(t, f)
	f.probes["loginctl show-user tester --property=Linger"] = "Linger=yes"
	if err := os.WriteFile(filepath.Join(f.stateDir, moshLocaleMarker), []byte("en_US.UTF-8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ufwConfPath, []byte("ENABLED=yes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.sudoOut["ufw status"] = ctdevUFWStatus()
	f.onSudo = func(cmd []string) {
		if len(cmd) > 2 && cmd[1] == "ufw" && cmd[2] == "delete" {
			f.sudoOut["ufw status"] = "Status: active"
		}
	}
	return f
}

func TestRetireRemovedSSHSettingsTurnsOffWhatCtdevAdded(t *testing.T) {
	if !gateLinux() {
		t.Skip("the retired settings were Linux-only")
	}
	f := ctdevAddedEverything(t)
	if err := RetireRemovedSSHSettings(context.Background(), quietOpts); err != nil {
		t.Fatal(err)
	}
	tunnel := f.index("code", "tunnel", "service", "uninstall")
	linger := f.index("sudo", "loginctl", "disable-linger", "tester")
	if tunnel < 0 || linger < 0 || tunnel > linger {
		t.Errorf("want the tunnel uninstalled, then lingering off: %q", f.calls)
	}
	if !f.ran("sudo", "localedef", "--delete-from-archive", "en_US.utf8") {
		t.Errorf("the locale ctdev added stays: %q", f.calls)
	}
	if _, err := os.Stat(filepath.Join(f.stateDir, moshLocaleMarker)); err == nil {
		t.Error("the locale marker stays")
	}
	for _, r := range ufwRules() {
		if !f.ran("sudo", "ufw", "delete", "allow", "from", r.cidr, "to", "any", "port", r.port, "proto", r.proto) {
			t.Errorf("ctdev's rule %v stays: %q", r, f.calls)
		}
	}
	if !f.ran("sudo", "ufw", "--force", "disable") {
		t.Errorf("UFW stays on with no rules left: %q", f.calls)
	}
}

// A firewall with rules of its own stays up once ctdev's rules are gone.
func TestRetireRemovedSSHSettingsKeepsAFirewallWithOtherRules(t *testing.T) {
	if !gateLinux() {
		t.Skip("the retired settings were Linux-only")
	}
	f := ctdevAddedEverything(t)
	f.onSudo = func(cmd []string) {
		if len(cmd) > 2 && cmd[1] == "ufw" && cmd[2] == "delete" {
			f.sudoOut["ufw status"] = "Status: active\n\nTo                         Action      From\n--                         ------      ----\n53                         ALLOW       Anywhere"
		}
	}
	if err := RetireRemovedSSHSettings(context.Background(), quietOpts); err != nil {
		t.Fatal(err)
	}
	if !f.ran("sudo", "ufw", "delete", "allow", "from", "10.0.0.0/8", "to", "any", "port", "22", "proto", "tcp") {
		t.Errorf("ctdev's rules stay: %q", f.calls)
	}
	if f.ran("sudo", "ufw", "--force", "disable") {
		t.Error("UFW turned off while other rules remain")
	}
}

// A machine where ctdev never turned the retired settings on: the locale was
// already there, UFW is on with someone else's rule, no lingering, and no
// tunnel. Nothing is touched.
func TestRetireRemovedSSHSettingsLeavesAloneWhatCtdevDidNotAdd(t *testing.T) {
	f := useFakeRemote(t)
	f.probes["locale -a"] = "C\nen_US.utf8"
	f.probes["loginctl show-user tester --property=Linger"] = "Linger=no"
	if err := os.WriteFile(ufwConfPath, []byte("ENABLED=yes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.sudoOut["ufw status"] = "Status: active\n\nTo                         Action      From\n--                         ------      ----\n22/tcp                     ALLOW       192.168.0.0/16"
	var out strings.Builder
	if err := RetireRemovedSSHSettings(context.Background(), sysutil.Opts{Stdout: &out}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 || out.Len() != 0 {
		t.Errorf("want nothing touched, got calls %q and output %q", f.calls, out.String())
	}
}

// Under --dry-run sudo isn't cached, so UFW's rules can't be read and the
// firewall is left alone rather than guessed at.
func TestRetireRemovedSSHSettingsLeavesUFWAloneWhenItCantReadIt(t *testing.T) {
	f := useFakeRemote(t)
	if err := os.WriteFile(ufwConfPath, []byte("ENABLED=yes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RetireRemovedSSHSettings(context.Background(), sysutil.Opts{Stdout: &strings.Builder{}, DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Errorf("want UFW untouched, got %q", f.calls)
	}
}

// After one clean pass the cleanup is done: lingering turned back on by hand
// stays on.
func TestRetireRemovedSSHSettingsRunsOnce(t *testing.T) {
	if !gateLinux() {
		t.Skip("the retired settings were Linux-only")
	}
	f := useFakeRemote(t)
	f.probes["loginctl show-user tester --property=Linger"] = "Linger=yes"
	if err := RetireRemovedSSHSettings(context.Background(), quietOpts); err != nil {
		t.Fatal(err)
	}
	f.calls = nil
	if err := RetireRemovedSSHSettings(context.Background(), quietOpts); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Errorf("second run touched the machine: %q", f.calls)
	}
}

// A failed off path leaves the cleanup to retry on the next run.
func TestRetireRemovedSSHSettingsRetriesAfterAFailure(t *testing.T) {
	if !gateLinux() {
		t.Skip("the retired settings were Linux-only")
	}
	f := useFakeRemote(t)
	f.probes["loginctl show-user tester --property=Linger"] = "Linger=yes"
	failing := remoteSudo
	remoteSudo = func(context.Context, sysutil.Opts, string, ...string) error { return errors.New("exit status 1") }
	if err := RetireRemovedSSHSettings(context.Background(), quietOpts); err == nil {
		t.Fatal("want the failure reported")
	}
	remoteSudo = failing
	if err := RetireRemovedSSHSettings(context.Background(), quietOpts); err != nil {
		t.Fatal(err)
	}
	if !f.ran("sudo", "loginctl", "disable-linger", "tester") {
		t.Errorf("the second run didn't retry: %q", f.calls)
	}
}
