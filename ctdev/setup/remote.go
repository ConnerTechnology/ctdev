package setup

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
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
	// remoteSudo runs a root command (systemctl, ufw, loginctl, locale-gen).
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
	// remoteUsername is the login lingering is turned on and off for.
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
	// remoteInstall installs apt packages (openssh-server, ufw).
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

// lanCIDRs are the RFC1918 private ranges UFW rules are scoped to. Inter-VLAN
// enforcement is delegated to the gateway firewall (see remote-access design).
var lanCIDRs = []string{"192.168.0.0/16", "10.0.0.0/8", "172.16.0.0/12"}

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

// moshLocale is the locale Mosh needs. moshLocaleMarker, in ctdev's state
// dir, records that ctdev generated it, so turning Mosh support off never
// removes a locale that was already there.
const (
	moshLocale       = "en_US.UTF-8"
	moshLocaleMarker = "mosh-locale-added"
)

// remoteOn reports whether a toggle value asks for the setting to be on.
func remoteOn(value string) bool { return value == "enabled" }

// gateMosh reports whether mosh-server is on PATH on this Linux host; the
// UTF-8 locale only matters to Mosh.
func gateMosh() bool {
	if !gateLinux() {
		return false
	}
	_, err := exec.LookPath("mosh-server")
	return err == nil
}

// gateVSCode reports whether VS Code's `code` CLI is on PATH on this Linux
// host; the tunnel is a systemd user service it installs.
func gateVSCode() bool {
	if !gateLinux() {
		return false
	}
	_, err := exec.LookPath("code")
	return err == nil
}

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

// ── Key-only login ──────────────────────────────────────────────────────

// detectSSHKeyOnly reports "enabled" only when sshd's effective config has
// password login off, read from `sshd -T` (the drop-in is root-only). With no
// authorized key ctdev leaves passwords on, so that reads "disabled" and a
// re-run after adding a key finishes the job.
func detectSSHKeyOnly(ctx context.Context) string {
	if !hasAuthorizedKey() {
		return "disabled"
	}
	out, err := remoteSudoProbe(ctx, "sshd", "-T")
	if err != nil || !sshdPasswordAuthOff(out) {
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

// ── Keep my programs running (lingering) ────────────────────────────────

// detectLinger reports whether systemd lingering is enabled for the current user.
func detectLinger(ctx context.Context) string {
	name, err := remoteUsername()
	if err != nil {
		return "disabled"
	}
	out, err := remoteProbe(ctx, "loginctl", "show-user", name, "--property=Linger")
	if err == nil && out == "Linger=yes" {
		return "enabled"
	}
	return "disabled"
}

// applyLinger turns systemd lingering on or off for the current user, so
// user services (VS Code tunnel, tmux) survive without a login session.
func applyLinger(ctx context.Context, o sysutil.Opts, value string) error {
	name, err := remoteUsername()
	if err != nil {
		return err
	}
	if remoteOn(value) {
		return remoteSudo(ctx, o, "loginctl", "enable-linger", name)
	}
	if detectCodeTunnel(ctx) == "enabled" {
		fmt.Fprintln(o.Stdout, "  warning: the vscode.dev tunnel is on and stops when you log out without this; turn \"Open this machine from vscode.dev\" off too, or keep this on")
	}
	return remoteSudo(ctx, o, "loginctl", "disable-linger", name)
}

// ── Mosh support (UTF-8 locale) ─────────────────────────────────────────

// detectMoshLocale reports "enabled" if an en_US UTF-8 locale is generated.
func detectMoshLocale(ctx context.Context) string {
	out, _ := remoteProbe(ctx, "locale", "-a")
	low := strings.ToLower(out)
	if strings.Contains(low, "en_us.utf-8") || strings.Contains(low, "en_us.utf8") {
		return "enabled"
	}
	return "disabled"
}

// applyMoshLocale generates en_US.UTF-8, which mosh-server needs, and
// remembers that ctdev added it. Off removes the locale only when ctdev was
// the one that generated it.
func applyMoshLocale(ctx context.Context, o sysutil.Opts, value string) error {
	marker := filepath.Join(remoteStateDir(), moshLocaleMarker)
	if remoteOn(value) {
		if detectMoshLocale(ctx) == "enabled" {
			return nil
		}
		if err := remoteSudo(ctx, o, "locale-gen", moshLocale); err != nil {
			return fmt.Errorf("locale-gen: %w", err)
		}
		if o.DryRun {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
			return err
		}
		return os.WriteFile(marker, []byte(moshLocale+"\n"), 0o644)
	}
	if _, err := os.Stat(marker); err != nil {
		fmt.Fprintf(o.Stdout, "  note: %s was here before ctdev, so it stays\n", moshLocale)
		return nil
	}
	if err := remoteSudo(ctx, o, "sed", "-i", `s/^en_US\.UTF-8 UTF-8/# en_US.UTF-8 UTF-8/`, "/etc/locale.gen"); err != nil {
		return err
	}
	if err := remoteSudo(ctx, o, "localedef", "--delete-from-archive", "en_US.utf8"); err != nil {
		return err
	}
	if o.DryRun {
		return nil
	}
	return os.Remove(marker)
}

// ── Open this machine from vscode.dev (VS Code tunnel) ──────────────────

// detectCodeTunnel reports whether the VS Code tunnel user service exists.
func detectCodeTunnel(_ context.Context) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "disabled"
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "systemd", "user", "code-tunnel.service")); err == nil {
		return "enabled"
	}
	return "disabled"
}

// applyCodeTunnel installs or uninstalls the VS Code tunnel user service.
// Turning it on also turns on lingering, which the tunnel needs to keep
// running after you log out. Authentication (`code tunnel user login`)
// remains a one-time manual step.
func applyCodeTunnel(ctx context.Context, o sysutil.Opts, value string) error {
	if !remoteCommandExists("code") {
		return fmt.Errorf("VS Code (code) not found — install the vscode component first")
	}
	if !remoteOn(value) {
		if err := remoteRun(ctx, o, "code", "tunnel", "service", "uninstall"); err != nil {
			return fmt.Errorf("code tunnel service uninstall: %w", err)
		}
		return nil
	}
	lingerWasOff := detectLinger(ctx) != "enabled"
	if err := applyLinger(ctx, o, "enabled"); err != nil {
		return fmt.Errorf("keep programs running (lingering): %w", err)
	}
	if lingerWasOff {
		fmt.Fprintln(o.Stdout, "  note: also turned on \"Keep my programs running after I disconnect\", which the tunnel needs (if you turned it off in this run, the tunnel turned it back on)")
	}
	if err := remoteRun(ctx, o, "code", "tunnel", "service", "install", "--accept-server-license-terms"); err != nil {
		return fmt.Errorf("code tunnel service install: %w", err)
	}
	fmt.Fprintln(o.Stdout, "  run 'code tunnel user login' once to authenticate, then reach this machine at https://vscode.dev")
	return nil
}

// ── Local-network-only firewall (UFW) ───────────────────────────────────

// detectUFW reports "enabled" when UFW is on (ufw.conf's ENABLED=yes) and
// every allow rule ctdev adds is present (`ufw status`, root-only).
func detectUFW(ctx context.Context) string {
	if !ufwEnabled() {
		return "disabled"
	}
	status, err := remoteSudoProbe(ctx, "ufw", "status")
	if err != nil || !ufwHasCtdevRules(status) {
		return "disabled"
	}
	return "enabled"
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

// ufwRule is one allow rule ctdev adds: a port and protocol from a range.
type ufwRule struct{ port, proto, cidr string }

// args returns the rule as `ufw allow` / `ufw delete allow` arguments.
func (r ufwRule) args() []string {
	return []string{"from", r.cidr, "to", "any", "port", r.port, "proto", r.proto}
}

// ufwRules are the allow rules ctdev adds, one SSH and one Mosh rule per
// private range.
func ufwRules() []ufwRule {
	var rules []ufwRule
	for _, cidr := range lanCIDRs {
		rules = append(rules,
			ufwRule{port: "22", proto: "tcp", cidr: cidr},
			ufwRule{port: "60000:61000", proto: "udp", cidr: cidr})
	}
	return rules
}

// ufwHasCtdevRules reports whether `ufw status` lists every rule ctdev adds.
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

// applyUFW turns the local-network-only firewall on or off. On opens SSH and
// Mosh from private LAN ranges and enables UFW. Off deletes exactly those
// rules, then turns UFW off only if no other rules are left, so a firewall
// you configured yourself stays up.
func applyUFW(ctx context.Context, o sysutil.Opts, value string) error {
	if remoteOn(value) {
		if !remoteCommandExists("ufw") {
			if err := remoteInstall(ctx, o, []string{"ufw"}); err != nil {
				return fmt.Errorf("install ufw: %w", err)
			}
		}
		for _, r := range ufwRules() {
			if err := remoteSudo(ctx, o, "ufw", append([]string{"allow"}, r.args()...)...); err != nil {
				return fmt.Errorf("ufw allow %s: %w", strings.Join(r.args(), " "), err)
			}
		}
		return remoteSudo(ctx, o, "ufw", "--force", "enable")
	}
	if !remoteCommandExists("ufw") {
		return nil
	}
	// Best-effort: a rule someone already deleted by hand isn't an error.
	for _, r := range ufwRules() {
		_ = remoteSudo(ctx, o, "ufw", append([]string{"delete", "allow"}, r.args()...)...)
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
