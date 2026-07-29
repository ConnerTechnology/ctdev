package setup

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/gpu"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// grubVarArgs returns the sed command args to set a GRUB variable.
// Uses sudo sed -i to replace or append the variable in /etc/default/grub.
// Uses | as the sed delimiter to avoid conflicts with / in values.
func grubVarArgs(varName, value string) []string {
	return []string{
		"sed", "-i",
		fmt.Sprintf("s|^%s=.*|%s=%s|", sedEscape(varName), varName, value),
		"/etc/default/grub",
	}
}

// sedEscape escapes regex metacharacters for use in sed patterns.
func sedEscape(s string) string {
	replacer := strings.NewReplacer(
		`.`, `\.`,
		`*`, `\*`,
		`[`, `\[`,
		`]`, `\]`,
		`^`, `\^`,
		`$`, `\$`,
		`\`, `\\`,
	)
	return replacer.Replace(s)
}

// dconfWriteArgs returns the dconf write command args for a given path and value.
func dconfWriteArgs(path, value string) []string {
	return []string{"dconf", "write", path, value}
}

// applyGrubVar sets a GRUB variable in /etc/default/grub using sed.
// If the variable doesn't exist, it appends it.
func applyGrubVar(ctx context.Context, o sysutil.Opts, varName, value string) error {
	content, err := os.ReadFile("/etc/default/grub")
	if err != nil {
		return fmt.Errorf("read /etc/default/grub: %w", err)
	}

	// Backup before modification (-n = no clobber, preserves original across multiple calls)
	if err := sysutil.SudoRun(ctx, o, "cp", "-n", "/etc/default/grub", "/etc/default/grub.ctdev-backup"); err != nil {
		return fmt.Errorf("backup /etc/default/grub: %w", err)
	}

	// Check if variable already exists — if so, replace it; otherwise append.
	if containsGrubVar(string(content), varName) {
		args := grubVarArgs(varName, value)
		return sysutil.SudoRun(ctx, o, args[0], args[1:]...)
	}

	// Append the variable
	tmpFile, err := os.CreateTemp("", "grub-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(appendGrubLine(string(content), varName, value)); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	return sysutil.SudoRun(ctx, o, "cp", tmpFile.Name(), "/etc/default/grub")
}

// appendGrubLine returns content with `varName=value` appended as a new line.
// A missing trailing newline on content is inserted first so the new variable
// lands on its own line instead of merging with the last existing line.
func appendGrubLine(content, varName, value string) string {
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + fmt.Sprintf("%s=%s\n", varName, value)
}

// containsGrubVar returns true if the content has a non-commented line starting with varName=.
func containsGrubVar(content, varName string) bool {
	prefix := varName + "="
	for _, line := range splitLines(content) {
		if len(line) > 0 && line[0] != '#' && len(line) >= len(prefix) && line[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// applyDconf writes a value to a dconf path (no quoting — suitable for int, bool, double).
func applyDconf(ctx context.Context, o sysutil.Opts, path, value string) error {
	args := dconfWriteArgs(path, value)
	return sysutil.Run(ctx, o, args[0], args[1:]...)
}

// applyDconfString writes a string value to a dconf path, wrapping it in single quotes.
func applyDconfString(ctx context.Context, o sysutil.Opts, path, value string) error {
	quoted := fmt.Sprintf("'%s'", value)
	args := dconfWriteArgs(path, quoted)
	return sysutil.Run(ctx, o, args[0], args[1:]...)
}

// applyGsettings runs gsettings set <schema> <key> <value>.
func applyGsettings(ctx context.Context, o sysutil.Opts, schema, key, value string) error {
	return sysutil.Run(ctx, o, "gsettings", "set", schema, key, value)
}

// applyPowerProfile sets the system power profile via powerprofilesctl.
func applyPowerProfile(ctx context.Context, o sysutil.Opts, value string) error {
	return sysutil.Run(ctx, o, "powerprofilesctl", "set", value)
}

// applyKeyRepeat sets the keyboard repeat rate using xset and gsettings.
// delay is in ms, rate is in characters-per-second.
func applyKeyRepeat(ctx context.Context, o sysutil.Opts, delay, rate string) error {
	if err := sysutil.Run(ctx, o, "xset", "r", "rate", delay, rate); err != nil {
		return fmt.Errorf("xset r rate: %w", err)
	}
	if err := applyGsettings(ctx, o, "org.cinnamon.desktop.peripherals.keyboard", "delay", delay); err != nil {
		return fmt.Errorf("gsettings delay: %w", err)
	}
	// gsettings repeat-interval is in milliseconds between repeats, not cps.
	interval := cpsToIntervalMs(rate)
	if err := applyGsettings(ctx, o, "org.cinnamon.desktop.peripherals.keyboard", "repeat-interval", interval); err != nil {
		return fmt.Errorf("gsettings repeat-interval: %w", err)
	}
	return nil
}

// cpsToIntervalMs converts a characters-per-second rate string to a
// milliseconds-between-repeats interval string for gsettings.
func cpsToIntervalMs(rate string) string {
	rateCps, err := strconv.Atoi(rate)
	if err != nil || rateCps <= 0 {
		return rate
	}
	return strconv.Itoa(1000 / rateCps)
}

// applySystemdEnable enables and starts a systemd service.
func applySystemdEnable(ctx context.Context, o sysutil.Opts, service string) error {
	if err := sysutil.SudoRun(ctx, o, "systemctl", "enable", service); err != nil {
		return fmt.Errorf("systemctl enable %s: %w", service, err)
	}
	if err := sysutil.SudoRun(ctx, o, "systemctl", "start", service); err != nil {
		return fmt.Errorf("systemctl start %s: %w", service, err)
	}
	return nil
}

// applyPackages installs apt packages quietly.
func applyPackages(ctx context.Context, o sysutil.Opts, packages []string) error {
	args := append([]string{"apt-get", "install", "-y", "-qq"}, packages...)
	return sysutil.SudoRun(ctx, o, args[0], args[1:]...)
}

// autoUpgradesConf enables the apt periodic jobs that unattended-upgrades
// hooks into. The package's stock 50unattended-upgrades already restricts
// upgrades to the security pocket, so this stays security-only by default.
const autoUpgradesConf = `// Written by ctdev (configure autoupdate).
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
`

const autoUpgradesConfPath = "/etc/apt/apt.conf.d/20auto-upgrades"

// applyUnattendedUpgrades installs unattended-upgrades and turns on the
// daily security-update run.
func applyUnattendedUpgrades(ctx context.Context, o sysutil.Opts) error {
	if err := applyPackages(ctx, o, []string{"unattended-upgrades"}); err != nil {
		return err
	}
	return sysutil.SudoWriteFile(ctx, o, autoUpgradesConf, autoUpgradesConfPath)
}

// detectUnattendedUpgrades reports "enabled" only when both the package and
// the periodic config that actually triggers it are present.
func detectUnattendedUpgrades(ctx context.Context) string {
	if !detectPackageInstalled(ctx, "unattended-upgrades") {
		return "disabled"
	}
	b, err := os.ReadFile(autoUpgradesConfPath)
	if err != nil || !strings.Contains(string(b), `Unattended-Upgrade "1"`) {
		return "disabled"
	}
	return "enabled"
}

// aptDailyUnits are the two stock apt jobs that take /var/lib/apt/lists/lock.
// Both ship with the apt package and are driven by their own timers.
var aptDailyUnits = []string{"apt-daily.service", "apt-daily-upgrade.service"}

// aptDailyTimeoutConf caps a single unit's start timeout. %s is the timespan.
const aptDailyTimeoutConf = `# Written by ctdev (configure autoupdate).
# Both apt-daily units are Type=oneshot, which systemd gives an infinite start
# timeout. A fetch that stalls rather than fails then holds the apt lock for as
# long as the machine is up, and every later apt run — including ctdev update —
# can't lock. This cap lets systemd kill the run instead.
[Service]
TimeoutStartSec=%s
`

func aptDailyDropInPath(unit string) string {
	return filepath.Join("/etc/systemd/system", unit+".d", "ctdev-timeout.conf")
}

// applyAptDailyTimeout writes (or removes) the start-timeout drop-in for both
// apt-daily units. "infinity" removes the drop-in rather than writing it out,
// so turning the cap off restores stock apt behavior instead of leaving a
// ctdev file behind that merely restates the default.
func applyAptDailyTimeout(ctx context.Context, o sysutil.Opts, value string) error {
	for _, unit := range aptDailyUnits {
		path := aptDailyDropInPath(unit)
		if value == "infinity" {
			if err := sysutil.SudoRun(ctx, o, "rm", "-f", path); err != nil {
				return fmt.Errorf("remove %s: %w", path, err)
			}
			continue
		}
		if err := sysutil.SudoRun(ctx, o, "mkdir", "-p", filepath.Dir(path)); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
		}
		if err := sysutil.SudoWriteFile(ctx, o, fmt.Sprintf(aptDailyTimeoutConf, value), path); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return sysutil.SudoRun(ctx, o, "systemctl", "daemon-reload")
}

// applyNvidiaSigning runs the GPU signing setup via the gpu package.
func applyNvidiaSigning(ctx context.Context, o sysutil.Opts) error {
	return gpu.RunSetup(ctx, gpu.Opts{
		Stdout: o.Stdout,
		Stdin:  os.Stdin,
	})
}

// applyNvidiaSuspendServices enables NVIDIA suspend-related systemd services.
func applyNvidiaSuspendServices(ctx context.Context, o sysutil.Opts) error {
	services := []string{
		"nvidia-suspend.service",
		"nvidia-resume.service",
		"nvidia-hibernate.service",
	}
	for _, svc := range services {
		// Skip services that don't exist or are static (no [Install] section).
		// (systemctl is-enabled output read plainly; no ctx because this is a read-only probe.)
		out, err := runOutput(ctx, "systemctl", "is-enabled", svc)
		if err != nil {
			continue
		}
		if strings.TrimSpace(out) == "static" {
			continue
		}
		if err := sysutil.SudoRun(ctx, o, "systemctl", "enable", svc); err != nil {
			return fmt.Errorf("enable %s: %w", svc, err)
		}
	}
	return nil
}

// applyXbindkeys installs xbindkeys and xdotool, deploys the config, and sets up autostart.
func applyXbindkeys(ctx context.Context, o sysutil.Opts) error {
	if err := applyPackages(ctx, o, []string{"xbindkeys", "xdotool"}); err != nil {
		return fmt.Errorf("install xbindkeys/xdotool: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	configDst := filepath.Join(home, ".xbindkeysrc")

	// Use DeployFileFromFS so any pre-existing user-customized file gets
	// backed up on diff instead of silently overwritten.
	if err := sysutil.DeployFileFromFS(Configs, "configs/xbindkeys/.xbindkeysrc", configDst); err != nil {
		return fmt.Errorf("deploy .xbindkeysrc: %w", err)
	}

	autostartDir := filepath.Join(home, ".config", "autostart")
	if err := os.MkdirAll(autostartDir, 0o755); err != nil {
		return fmt.Errorf("mkdir autostart: %w", err)
	}

	desktopDst := filepath.Join(autostartDir, "xbindkeys.desktop")
	if err := sysutil.DeployFileFromFS(Configs, "configs/xbindkeys/xbindkeys.desktop", desktopDst); err != nil {
		return fmt.Errorf("deploy xbindkeys.desktop: %w", err)
	}

	// Restart xbindkeys to pick up new config; best-effort.
	_ = sysutil.Run(ctx, o, "killall", "xbindkeys")
	// Fire-and-forget: don't block on xbindkeys.
	_ = startDetached("xbindkeys")

	return nil
}

// applyWireplumberLDAC copies the WirePlumber LDAC config from the dotfiles repo.
func applyWireplumberLDAC(ctx context.Context, o sysutil.Opts) error {
	confDir := "/etc/wireplumber/wireplumber.conf.d"
	confDst := filepath.Join(confDir, "51-ldac-hq.conf")

	if err := sysutil.SudoRun(ctx, o, "mkdir", "-p", confDir); err != nil {
		return fmt.Errorf("mkdir wireplumber conf dir: %w", err)
	}

	content, err := Configs.ReadFile("configs/wireplumber/51-ldac-hq.conf")
	if err != nil {
		return fmt.Errorf("read embedded wireplumber config: %w", err)
	}
	if err := sysutil.SudoWriteFile(ctx, o, string(content), confDst); err != nil {
		return fmt.Errorf("copy wireplumber config: %w", err)
	}

	// Restart PipeWire stack to pick up new config; best-effort.
	_ = sysutil.Run(ctx, o, "systemctl", "--user", "restart",
		"pipewire", "pipewire-pulse", "wireplumber")

	return nil
}

// applyLogitechKVMFix installs a udev rule and systemd user service to restart Solaar
// when the Logi Bolt receiver reconnects after a KVM switch.
func applyLogitechKVMFix(ctx context.Context, o sysutil.Opts) error {
	// Deploy udev rule
	udevContent, err := Configs.ReadFile("configs/udev/99-logitech-kvm-fix.rules")
	if err != nil {
		return fmt.Errorf("read embedded udev rule: %w", err)
	}
	if err := sysutil.SudoWriteFile(ctx, o, string(udevContent), "/etc/udev/rules.d/99-logitech-kvm-fix.rules"); err != nil {
		return fmt.Errorf("copy udev rule: %w", err)
	}

	// Deploy systemd user service
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	serviceDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		return fmt.Errorf("mkdir systemd user dir: %w", err)
	}
	serviceContent, err := Configs.ReadFile("configs/udev/solaar-restart.service")
	if err != nil {
		return fmt.Errorf("read embedded service file: %w", err)
	}
	serviceDst := filepath.Join(serviceDir, "solaar-restart.service")
	if err := os.WriteFile(serviceDst, serviceContent, 0o644); err != nil {
		return fmt.Errorf("write service file: %w", err)
	}

	// Enable the user service and reload udev
	_ = sysutil.Run(ctx, o, "systemctl", "--user", "daemon-reload")
	_ = sysutil.Run(ctx, o, "systemctl", "--user", "enable", "solaar-restart.service")
	if err := sysutil.SudoRun(ctx, o, "udevadm", "control", "--reload-rules"); err != nil {
		return fmt.Errorf("reload udev rules: %w", err)
	}

	return nil
}

// applyHideDrives installs a udev rule to hide Windows/secondary partitions from the file manager.
func applyHideDrives(ctx context.Context, o sysutil.Opts) error {
	content, err := Configs.ReadFile("configs/udev/99-hide-drives.rules")
	if err != nil {
		return fmt.Errorf("read embedded hide-drives rule: %w", err)
	}
	if err := sysutil.SudoWriteFile(ctx, o, string(content), "/etc/udev/rules.d/99-hide-drives.rules"); err != nil {
		return fmt.Errorf("copy udev rule: %w", err)
	}
	return sysutil.SudoRun(ctx, o, "udevadm", "control", "--reload-rules")
}

// applySSDTrim enables the fstrim.timer systemd unit for periodic SSD TRIM.
func applySSDTrim(ctx context.Context, o sysutil.Opts) error {
	if err := sysutil.SudoRun(ctx, o, "systemctl", "enable", "fstrim.timer"); err != nil {
		return fmt.Errorf("enable fstrim.timer: %w", err)
	}
	if err := sysutil.SudoRun(ctx, o, "systemctl", "start", "fstrim.timer"); err != nil {
		return fmt.Errorf("start fstrim.timer: %w", err)
	}
	return nil
}

// applyUpdateGrub regenerates the GRUB configuration. Used as a post-apply hook.
func applyUpdateGrub(ctx context.Context, o sysutil.Opts) error {
	return sysutil.SudoRun(ctx, o, "update-grub")
}

// ── Remote access ────────────────────────────────────────────────────────

// lanCIDRs are the RFC1918 private ranges UFW rules are scoped to. Inter-VLAN
// enforcement is delegated to the gateway firewall (see remote-access design).
var lanCIDRs = []string{"192.168.0.0/16", "10.0.0.0/8", "172.16.0.0/12"}

// sleepTargets are the systemd targets masked to keep an always-on box awake.
var sleepTargets = []string{"sleep.target", "suspend.target", "hibernate.target", "hybrid-sleep.target"}

const sshdDropIn = "/etc/ssh/sshd_config.d/99-ctdev.conf"

const sshHardeningConf = `# Managed by ctdev — remote access (key-based SSH)
PubkeyAuthentication yes
KbdInteractiveAuthentication no
ClientAliveInterval 60
ClientAliveCountMax 3
`

const wifiPowersaveConf = `[connection]
# Managed by ctdev — keep the WiFi adapter from dropping off the network while idle.
wifi.powersave = 2
`

// applySSHServer enables and starts the OpenSSH server.
func applySSHServer(ctx context.Context, o sysutil.Opts) error {
	return sysutil.SudoRun(ctx, o, "systemctl", "enable", "--now", "ssh")
}

// applySSHKeyAuth writes an sshd drop-in for key-based login. Password auth is
// only disabled once an authorized key is present, so a key-less run can't lock
// the user out. The config is validated with `sshd -t` before reload.
func applySSHKeyAuth(ctx context.Context, o sysutil.Opts) error {
	content := sshHardeningConf
	if hasAuthorizedKey() {
		content += "PasswordAuthentication no\n"
	} else {
		fmt.Fprintln(o.Stdout, "  note: no ~/.ssh/authorized_keys yet — leaving password auth enabled until a key is added; re-run after adding your key")
	}
	if err := sysutil.SudoWriteFile(ctx, o, content, sshdDropIn); err != nil {
		return err
	}
	if err := sysutil.SudoRun(ctx, o, "sshd", "-t"); err != nil {
		return fmt.Errorf("sshd config validation failed: %w", err)
	}
	return sysutil.SudoRun(ctx, o, "systemctl", "reload", "ssh")
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

// applyUFWRemote opens SSH + Mosh from private LAN ranges and enables UFW.
func applyUFWRemote(ctx context.Context, o sysutil.Opts) error {
	if !sysutil.CommandExists("ufw") {
		if err := applyPackages(ctx, o, []string{"ufw"}); err != nil {
			return fmt.Errorf("install ufw: %w", err)
		}
	}
	for _, cidr := range lanCIDRs {
		if err := sysutil.SudoRun(ctx, o, "ufw", "allow", "from", cidr, "to", "any", "port", "22", "proto", "tcp"); err != nil {
			return fmt.Errorf("ufw allow ssh from %s: %w", cidr, err)
		}
		if err := sysutil.SudoRun(ctx, o, "ufw", "allow", "from", cidr, "to", "any", "port", "60000:61000", "proto", "udp"); err != nil {
			return fmt.Errorf("ufw allow mosh from %s: %w", cidr, err)
		}
	}
	return sysutil.SudoRun(ctx, o, "ufw", "--force", "enable")
}

// applyUTF8Locale generates en_US.UTF-8 (required by Mosh).
func applyUTF8Locale(ctx context.Context, o sysutil.Opts) error {
	if err := sysutil.SudoRun(ctx, o, "locale-gen", "en_US.UTF-8"); err != nil {
		return fmt.Errorf("locale-gen: %w", err)
	}
	return sysutil.SudoRun(ctx, o, "update-locale", "LANG=en_US.UTF-8")
}

// applySuspendMask masks the sleep/suspend/hibernate targets.
func applySuspendMask(ctx context.Context, o sysutil.Opts) error {
	return sysutil.SudoRun(ctx, o, "systemctl", append([]string{"mask"}, sleepTargets...)...)
}

// applyWifiPowersaveOff writes a NetworkManager drop-in disabling WiFi power
// save. It intentionally does NOT restart NetworkManager — doing so over a
// remote session could drop the connection; it takes effect on next restart.
func applyWifiPowersaveOff(ctx context.Context, o sysutil.Opts) error {
	return sysutil.SudoWriteFile(ctx, o, wifiPowersaveConf, "/etc/NetworkManager/conf.d/wifi-powersave-off.conf")
}

// applyLinger enables systemd lingering for the current user so user services
// (VS Code tunnel, tmux) survive without an active login session.
func applyLinger(ctx context.Context, o sysutil.Opts) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	return sysutil.SudoRun(ctx, o, "loginctl", "enable-linger", u.Username)
}

// applyCodeTunnelService installs the VS Code tunnel as a user service.
// Authentication (`code tunnel user login`) remains a one-time manual step.
func applyCodeTunnelService(ctx context.Context, o sysutil.Opts) error {
	if !sysutil.CommandExists("code") {
		return fmt.Errorf("VS Code (code) not found — install the vscode component first")
	}
	if err := sysutil.Run(ctx, o, "code", "tunnel", "service", "install", "--accept-server-license-terms"); err != nil {
		return fmt.Errorf("code tunnel service install: %w", err)
	}
	fmt.Fprintln(o.Stdout, "  run 'code tunnel user login' once to authenticate, then reach this machine at https://vscode.dev")
	return nil
}
