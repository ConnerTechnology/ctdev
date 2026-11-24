package setup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/state"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// ── Remote access (`ctdev configure ssh`) ───────────────────────────────
//
// Every remote-access setting has an on and an off path, and the off path
// removes only what the on path added. The shell-outs go through the package
// vars below so tests can record them instead of touching the machine.

var (
	// remoteSudo runs a root command (systemctl, ufw, loginctl, localedef).
	remoteSudo = sysutil.SudoRun
	// remoteRun runs a command as the user (`code tunnel`).
	remoteRun = sysutil.Run
	// remoteWriteFile writes a root-owned file.
	remoteWriteFile = sysutil.SudoWriteFile
	// remoteProbe reads a command's trimmed stdout without changing anything.
	remoteProbe = runOutput
	// remoteSudoProbe reads a root-only command's stdout (`ufw status`).
	remoteSudoProbe = func(ctx context.Context, name string, args ...string) (string, error) {
		out, err := sysutil.SudoNoPrompt(ctx, name, args...).Output()
		return strings.TrimSpace(string(out)), err
	}
	// remoteUsername is the login lingering is turned off for.
	remoteUsername = func() (string, error) {
		u, err := user.Current()
		if err != nil {
			return "", err
		}
		return u.Username, nil
	}
	// remoteStateDir is where ctdev remembers what it added (state.StateDir).
	remoteStateDir = state.StateDir
	// remoteCommandExists reports whether a binary is on PATH.
	remoteCommandExists = sysutil.CommandExists
	// remoteInstall installs apt packages (openssh-server).
	remoteInstall = applyPackages
	// sshdPath is the OpenSSH server binary; its absence means
	// openssh-server isn't installed.
	sshdPath = "/usr/sbin/sshd"
	// sshSocketMigrationDropIn is left by Ubuntu's 22.10/23.04 upgrade to
	// socket activation; while it exists, ssh.service pulls ssh.socket back in.
	sshSocketMigrationDropIn = "/etc/systemd/system/ssh.service.d/00-socket.conf"
	// ufwConfPath holds UFW's ENABLED= switch, readable without root.
	ufwConfPath = "/etc/ufw/ufw.conf"
)

// SSH server modes, the values of the "SSH server" picker.
const (
	sshOnDemand = "on demand"
	sshAlwaysOn = "always on"
	sshOff      = "off"
)

// sshdDropIn is ctdev's sshd drop-in. sshd keeps the first value it reads
// and includes sshd_config.d/*.conf in name order, so the 00- prefix puts it
// ahead of drop-ins like 50-cloud-init.conf. sshdLegacyDropIn is the name
// older ctdev versions used; it is removed whenever the drop-in is written
// or turned off.
const (
	sshdDropIn       = "/etc/ssh/sshd_config.d/00-ctdev.conf"
	sshdLegacyDropIn = "/etc/ssh/sshd_config.d/99-ctdev.conf"
)

const sshHardeningConf = `# Managed by ctdev — remote access (key-based SSH)
PubkeyAuthentication yes
KbdInteractiveAuthentication no
ClientAliveInterval 60
ClientAliveCountMax 3
`

// Markers in ctdev's state dir. moshLocaleMarker records that the retired
// Mosh support setting generated en_US.UTF-8, so only a locale ctdev added is
// removed. sshSettingsRetiredMarker records that the retired settings were
// turned off, so lingering or a tunnel turned back on by hand stays on.
const (
	moshLocaleMarker         = "mosh-locale-added"
	sshSettingsRetiredMarker = "ssh-settings-retired"
)

// remoteOn reports whether a toggle value asks for the setting to be on.
func remoteOn(value string) bool { return value == "enabled" }

// ── SSH server ──────────────────────────────────────────────────────────

// detectSSHServer reports how the OpenSSH server runs: started on demand by
// ssh.socket, always running as ssh.service, or off.
func detectSSHServer(ctx context.Context) string {
	if out, _ := remoteProbe(ctx, "systemctl", "is-enabled", "ssh.socket"); out == "enabled" {
		return sshOnDemand
	}
	if out, _ := remoteProbe(ctx, "systemctl", "is-active", "ssh.service"); out == "active" {
		return sshAlwaysOn
	}
	return sshOff
}

// applySSHServer puts the OpenSSH server in the given mode, installing
// openssh-server first when a mode that runs it is picked.
func applySSHServer(ctx context.Context, o sysutil.Opts, mode string) error {
	if mode != sshOff {
		if _, err := os.Stat(sshdPath); err != nil {
			if err := remoteInstall(ctx, o, []string{"openssh-server"}); err != nil {
				return fmt.Errorf("install openssh-server: %w", err)
			}
		}
	}
	switch mode {
	case sshOnDemand:
		// A standalone ssh.service holds port 22, so stop it before the
		// socket takes over. Sessions already open survive the stop.
		if err := remoteSudo(ctx, o, "systemctl", "disable", "--now", "ssh.service"); err != nil {
			return err
		}
		return remoteSudo(ctx, o, "systemctl", "enable", "--now", "ssh.socket")
	case sshAlwaysOn:
		if err := remoteSudo(ctx, o, "systemctl", "disable", "--now", "ssh.socket"); err != nil {
			return err
		}
		if err := remoteSudo(ctx, o, "systemctl", "enable", "--now", "ssh.service"); err != nil {
			return err
		}
		if _, err := os.Stat(sshSocketMigrationDropIn); err == nil {
			fmt.Fprintf(o.Stdout, "  note: %s pulls ssh.socket back in, so SSH may still start on demand; to remove it, follow Ubuntu's revert steps at https://discourse.ubuntu.com/t/sshd-now-uses-socket-based-activation-ubuntu-22-10-and-later/30189\n", sshSocketMigrationDropIn)
		}
		return nil
	case sshOff:
		return remoteSudo(ctx, o, "systemctl", "disable", "--now", "ssh.socket", "ssh.service")
	}
	return fmt.Errorf("unknown SSH server mode %q", mode)
}

// EnsureSSHServer installs openssh-server and enables it at boot for
// `ctdev install ssh`. A server already running keeps its mode, so an
// always-on choice made in `configure ssh` survives; one that is off gets
// the setting's default.
func EnsureSSHServer(ctx context.Context, o sysutil.Opts) error {
	mode := detectSSHServer(ctx)
	if mode == sshOff {
		mode = sshOnDemand
	}
	return applySSHServer(ctx, o, mode)
}

// DisableSSHServer stops the OpenSSH server and keeps it from starting at
// boot. With openssh-server absent there are no units to disable, and
// systemctl would fail on them.
func DisableSSHServer(ctx context.Context, o sysutil.Opts) error {
	if _, err := os.Stat(sshdPath); err != nil {
		return nil
	}
	return applySSHServer(ctx, o, sshOff)
}

// SSHServerStartsAtBoot reports whether openssh-server is installed and one
// of its units is enabled.
func SSHServerStartsAtBoot(ctx context.Context) bool {
	if _, err := os.Stat(sshdPath); err != nil {
		return false
	}
	for _, unit := range []string{"ssh.socket", "ssh.service"} {
		if out, _ := remoteProbe(ctx, "systemctl", "is-enabled", unit); out == "enabled" {
			return true
		}
	}
	return false
}

// ── Key-only login ──────────────────────────────────────────────────────

// detectSSHKeyOnly reports "enabled" only when ctdev's drop-in exists and
// sshd's effective config has password login off, both read through the
// sudo probe (the drop-in is root-only). With no authorized key ctdev leaves
// passwords on, so that reads "disabled" and a re-run after adding a key
// finishes the job. When the probe can't run (--show and --dry-run skip
// sudo) it reads "unknown".
func detectSSHKeyOnly(ctx context.Context) string {
	if !hasAuthorizedKey() {
		return "disabled"
	}
	out, err := remoteSudoProbe(ctx, "sshd", "-T")
	if err != nil {
		return ValueUnknown
	}
	if _, err := remoteSudoProbe(ctx, "test", "-e", sshdDropIn); err != nil {
		return "disabled"
	}
	if !sshdPasswordAuthOff(out) {
		return "disabled"
	}
	return "enabled"
}

// sshdPasswordAuthOff reports whether `sshd -T` output has password login off.
func sshdPasswordAuthOff(effective string) bool {
	for _, line := range strings.Split(effective, "\n") {
		if strings.EqualFold(strings.TrimSpace(line), "passwordauthentication no") {
			return true
		}
	}
	return false
}

// applySSHKeyOnly writes or removes ctdev's sshd drop-in. Password auth is
// only disabled once an authorized key is present, so a key-less run can't
// lock the user out. The config is validated with `sshd -t` before reload,
// and the reload is skipped when sshd isn't running (socket activation
// starts it with the new config).
func applySSHKeyOnly(ctx context.Context, o sysutil.Opts, value string) error {
	if !remoteOn(value) {
		if err := remoteSudo(ctx, o, "rm", "-f", sshdDropIn, sshdLegacyDropIn); err != nil {
			return err
		}
		return remoteSudo(ctx, o, "systemctl", "try-reload-or-restart", "ssh.service")
	}
	// The SSH server setting runs first and installs openssh-server, so this
	// only trips when that setting was skipped or failed.
	if _, err := os.Stat(sshdPath); err != nil {
		fmt.Fprintln(o.Stdout, "  skipped: no sshd — turn on the SSH server first")
		return nil
	}
	hasKey := hasAuthorizedKey()
	content := sshHardeningConf
	if hasKey {
		content += "PasswordAuthentication no\n"
	} else {
		fmt.Fprintln(o.Stdout, "  note: no ~/.ssh/authorized_keys yet — leaving password auth enabled until a key is added; re-run after adding your key")
	}
	if err := remoteSudo(ctx, o, "rm", "-f", sshdLegacyDropIn); err != nil {
		return err
	}
	if err := remoteWriteFile(ctx, o, content, sshdDropIn); err != nil {
		return err
	}
	if err := remoteSudo(ctx, o, "sshd", "-t"); err != nil {
		return fmt.Errorf("sshd config validation failed: %w", err)
	}
	if err := remoteSudo(ctx, o, "systemctl", "try-reload-or-restart", "ssh.service"); err != nil {
		return err
	}
	if hasKey && !o.DryRun {
		if out, err := remoteSudoProbe(ctx, "sshd", "-T"); err == nil && !sshdPasswordAuthOff(out) {
			fmt.Fprintln(o.Stdout, "  warning: another sshd setting turns password login back on; check /etc/ssh/sshd_config and /etc/ssh/sshd_config.d/ (sudo sshd -T | grep -i passwordauthentication)")
		}
	}
	return nil
}

// hasAuthorizedKey reports whether ~/.ssh/authorized_keys has at least one key.
func hasAuthorizedKey() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return true
		}
	}
	return false
}

// ── Settings `configure ssh` no longer offers (ADR-0023) ────────────────

// retiredSSHSettings are the four settings CTD-178 removed from `configure
// ssh`. Each pairs a check that ctdev turned it on with the old setting's
// off path, which removes only what ctdev added. The tunnel comes before
// lingering, which it relied on.
var retiredSSHSettings = []struct {
	name  string
	added func(context.Context) (bool, error)
	off   func(context.Context, sysutil.Opts) error
}{
	{"Open this machine from vscode.dev", known(codeTunnelInstalled), uninstallCodeTunnel},
	{"Keep my programs running after I disconnect", known(lingering), disableLinger},
	{"Mosh support", known(moshLocaleAdded), removeMoshLocale},
	{"Only allow SSH and Mosh from my local network (firewall)", ctdevUFWOn, removeCtdevUFW},
}

// RetireRemovedSSHSettings turns off each retired setting ctdev turned on,
// so a setting removed from the menu doesn't stay on with no off switch. A
// machine where ctdev turned none of them on is left alone. It runs until a
// pass succeeds, then never again.
func RetireRemovedSSHSettings(ctx context.Context, o sysutil.Opts) error {
	done := filepath.Join(remoteStateDir(), sshSettingsRetiredMarker)
	if _, err := os.Stat(done); err == nil || !gateLinux() {
		return nil
	}
	var errs []error
	for _, s := range retiredSSHSettings {
		added, err := s.added(ctx)
		if err != nil {
			// --dry-run skips sudo, so it can't read root-only state; it changes
			// nothing and writes no marker anyway.
			if !o.DryRun {
				errs = append(errs, fmt.Errorf("check %q: %w", s.name, err))
			}
			continue
		}
		if !added {
			continue
		}
		fmt.Fprintf(o.Stdout, "  Turning off %q, which ctdev no longer offers\n", s.name)
		if err := s.off(ctx, o); err != nil {
			errs = append(errs, fmt.Errorf("turn off %q: %w", s.name, err))
		}
	}
	if len(errs) > 0 || o.DryRun {
		return errors.Join(errs...)
	}
	if err := os.MkdirAll(filepath.Dir(done), 0o755); err != nil {
		return err
	}
	return os.WriteFile(done, nil, 0o644)
}

// codeTunnelInstalled reports whether the VS Code tunnel user service exists
// and the `code` CLI that uninstalls it is on PATH. Nothing records who
// installed it; ctdev's vscode.dev setting defaulted to on wherever VS Code
// was installed, so on a ctdev machine it was ctdev.
func codeTunnelInstalled(_ context.Context) bool {
	home, err := os.UserHomeDir()
	if err != nil || !remoteCommandExists("code") {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".config", "systemd", "user", "code-tunnel.service"))
	return err == nil
}

func uninstallCodeTunnel(ctx context.Context, o sysutil.Opts) error {
	return remoteRun(ctx, o, "code", "tunnel", "service", "uninstall")
}

// lingering reports whether systemd lingering is on for the current user.
// Nothing records who turned it on; ctdev's "Keep my programs running"
// setting defaulted to on in every --batch run, so on a ctdev machine it was
// ctdev, and ctdev otherwise never turns lingering on (docs/backups.md).
func lingering(ctx context.Context) bool {
	name, err := remoteUsername()
	if err != nil {
		return false
	}
	out, err := remoteProbe(ctx, "loginctl", "show-user", name, "--property=Linger")
	return err == nil && out == "Linger=yes"
}

func disableLinger(ctx context.Context, o sysutil.Opts) error {
	name, err := remoteUsername()
	if err != nil {
		return err
	}
	return remoteSudo(ctx, o, "loginctl", "disable-linger", name)
}

// moshLocaleAdded reports whether ctdev generated en_US.UTF-8.
func moshLocaleAdded(_ context.Context) bool {
	_, err := os.Stat(filepath.Join(remoteStateDir(), moshLocaleMarker))
	return err == nil
}

// removeMoshLocale removes the en_US.UTF-8 locale ctdev generated, then the
// marker that says it did.
func removeMoshLocale(ctx context.Context, o sysutil.Opts) error {
	if err := remoteSudo(ctx, o, "sed", "-i", `s/^en_US\.UTF-8 UTF-8/# en_US.UTF-8 UTF-8/`, "/etc/locale.gen"); err != nil {
		return err
	}
	if err := remoteSudo(ctx, o, "localedef", "--delete-from-archive", "en_US.utf8"); err != nil {
		return err
	}
	if o.DryRun {
		return nil
	}
	return os.Remove(filepath.Join(remoteStateDir(), moshLocaleMarker))
}

// ctdevUFWOn reports whether UFW is on (ufw.conf's ENABLED=yes) with every
// allow rule ctdev added (`ufw status`, root-only). A probe that can't run is
// an error, not "ctdev didn't add it", so the cleanup isn't marked done
// without having looked.
func ctdevUFWOn(ctx context.Context) (bool, error) {
	if !ufwEnabled() {
		return false, nil
	}
	status, err := remoteSudoProbe(ctx, "ufw", "status")
	if err != nil {
		return false, fmt.Errorf("read UFW's rules (needs sudo): %w", err)
	}
	return ufwHasCtdevRules(status), nil
}

// known adapts a check that can't fail to the retired-settings table.
func known(added func(context.Context) bool) func(context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) { return added(ctx), nil }
}

// SSHSettingsRetirePending reports whether RetireRemovedSSHSettings still has
// to run here, so a caller that hasn't asked for sudo yet can ask first.
func SSHSettingsRetirePending() bool {
	if !gateLinux() {
		return false
	}
	_, err := os.Stat(filepath.Join(remoteStateDir(), sshSettingsRetiredMarker))
	return err != nil
}

// ufwEnabled reports whether ufw.conf, readable without root, has ENABLED=yes.
func ufwEnabled() bool {
	f, err := os.Open(ufwConfPath)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "ENABLED=yes" {
			return true
		}
	}
	return false
}

// ufwRule is one allow rule ctdev added: a port and protocol from a range.
type ufwRule struct{ port, proto, cidr string }

// ufwRules are the allow rules ctdev added, one SSH and one Mosh rule per
// RFC1918 private range.
func ufwRules() []ufwRule {
	var rules []ufwRule
	for _, cidr := range []string{"192.168.0.0/16", "10.0.0.0/8", "172.16.0.0/12"} {
		rules = append(rules,
			ufwRule{port: "22", proto: "tcp", cidr: cidr},
			ufwRule{port: "60000:61000", proto: "udp", cidr: cidr})
	}
	return rules
}

// ufwHasCtdevRules reports whether `ufw status` lists every rule ctdev added.
// UFW prints each as "22/tcp  ALLOW  192.168.0.0/16".
func ufwHasCtdevRules(status string) bool {
	listed := map[ufwRule]bool{}
	for _, line := range strings.Split(status, "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || f[1] != "ALLOW" {
			continue
		}
		port, proto, ok := strings.Cut(f[0], "/")
		if !ok {
			continue
		}
		listed[ufwRule{port: port, proto: proto, cidr: f[2]}] = true
	}
	for _, r := range ufwRules() {
		if !listed[r] {
			return false
		}
	}
	return true
}

// removeCtdevUFW deletes exactly the rules ctdev added, then turns UFW off
// only if no other rules are left, so a firewall you configured yourself
// stays up.
func removeCtdevUFW(ctx context.Context, o sysutil.Opts) error {
	// Best-effort: a rule someone already deleted by hand isn't an error.
	for _, r := range ufwRules() {
		_ = remoteSudo(ctx, o, "ufw", "delete", "allow", "from", r.cidr, "to", "any", "port", r.port, "proto", r.proto)
	}
	if !o.DryRun {
		status, err := remoteSudoProbe(ctx, "ufw", "status")
		if err != nil {
			return fmt.Errorf("ufw status: %w", err)
		}
		if ufwHasRules(status) {
			fmt.Fprintln(o.Stdout, "  note: UFW has other rules, so it stays on; run 'sudo ufw disable' to turn it off")
			return nil
		}
	}
	return remoteSudo(ctx, o, "ufw", "--force", "disable")
}

// ufwHasRules reports whether `ufw status` output lists any rule. Rules
// follow the "--" separator under the To/Action/From header.
func ufwHasRules(status string) bool {
	seenSeparator := false
	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "--") {
			seenSeparator = true
			continue
		}
		if seenSeparator && line != "" {
			return true
		}
	}
	return false
}
