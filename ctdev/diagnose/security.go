package diagnose

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
)

// The security checks report; they never change anything. On a machine you're
// visiting, "your disk isn't encrypted" is information for the owner to act
// on, not a setting for you to flip.

// checkFirewall reports whether a host firewall is active. It's deliberately
// not a failure: a desktop behind a NAT router with no listening services is
// fine without one, and saying otherwise would be crying wolf.
func checkFirewall(ctx context.Context, f Facts) Result {
	switch f.Platform.OS {
	case platform.Linux:
		switch {
		case commandExists("ufw"):
			// Reading status needs root; without it we can still tell whether
			// the service is running.
			if out, ok := sudoCapture(ctx, "ufw", "status"); ok {
				if strings.Contains(out, "Status: active") {
					return okf("ufw active")
				}
				return infof("ufw installed but inactive")
			}
			return needsRootSkip("to read ufw status")
		case commandExists("firewall-cmd"):
			if strings.TrimSpace(capture(ctx, "firewall-cmd", "--state")) == "running" {
				return okf("firewalld running")
			}
			return infof("firewalld installed but not running")
		}
		return skipf("no recognized firewall installed")

	case platform.MacOS:
		out := capture(ctx, "defaults", "read", "/Library/Preferences/com.apple.alf", "globalstate")
		switch strings.TrimSpace(out) {
		case "1", "2":
			return okf("application firewall on")
		case "0":
			return infof("application firewall off")
		}
		return skipf("could not read the firewall state")

	case platform.Windows:
		out := powershell(ctx,
			`(Get-NetFirewallProfile -ErrorAction SilentlyContinue |`+
				` Where-Object {$_.Enabled -eq $true}).Count`)
		n, err := strconv.Atoi(firstLine(out))
		if err != nil {
			return skipf("could not read the firewall state")
		}
		if n == 0 {
			return warnf("Turn Windows Firewall back on unless something specifically requires it off.",
				"every firewall profile is disabled")
		}
		return okf("%d of 3 firewall profiles enabled", n)
	}
	return skipf("not checked on this platform")
}

// checkDiskEncryption reports whether the disk is encrypted at rest. For a
// laptop that leaves the house this is the difference between a lost device
// and a data breach, and it's invisible until someone asks.
func checkDiskEncryption(ctx context.Context, f Facts) Result {
	switch f.Platform.OS {
	case platform.Linux:
		if !commandExists("lsblk") {
			return skipf("lsblk is not installed")
		}
		out := capture(ctx, "lsblk", "-n", "-o", "TYPE")
		if strings.Contains(out, "crypt") {
			return okf("LUKS encryption in use")
		}
		return infof("no encrypted volumes found")

	case platform.MacOS:
		out := capture(ctx, "fdesetup", "status")
		switch {
		case strings.Contains(out, "FileVault is On"):
			return okf("FileVault on")
		case strings.Contains(out, "FileVault is Off"):
			return infof("FileVault off")
		}
		return skipf("could not read the FileVault state")

	case platform.Windows:
		out := powershell(ctx,
			`(Get-BitLockerVolume -ErrorAction SilentlyContinue |`+
				` Where-Object {$_.VolumeType -eq 'OperatingSystem'}).ProtectionStatus`)
		switch strings.TrimSpace(firstLine(out)) {
		case "On", "1":
			return okf("BitLocker on")
		case "Off", "0":
			return infof("BitLocker off")
		}
		return skipf("BitLocker state needs an elevated shell")
	}
	return skipf("not checked on this platform")
}

// checkSSHExposure flags an SSH server accepting passwords. Every scanner on
// the internet tries password auth against port 22 continuously, so this is
// the difference between "exposed" and "exposed and guessable".
func checkSSHExposure(ctx context.Context, f Facts) Result {
	if f.Platform.OS == platform.Windows {
		return skipf("not checked on Windows")
	}
	if !pathExists("/etc/ssh/sshd_config") {
		return skipf("no SSH server installed")
	}
	// macOS ships sshd_config on every Mac whether or not Remote Login is on,
	// so the file alone says nothing about exposure. Only a definite "not
	// running" skips; an unanswerable probe falls through to the config.
	if sshdServing(ctx, f.Platform.OS) == serviceStopped {
		return skipf("SSH server not running")
	}

	// `sshd -T` prints the resolved configuration, which is the only
	// authoritative answer once drop-ins, Match blocks, and compiled-in
	// defaults are in play. It needs root, so it's the preferred path rather
	// than the only one.
	if out, gotRoot := sudoCapture(ctx, "sshd", "-T"); gotRoot {
		switch parsePasswordAuth(out) {
		case authYes:
			return warnf("Switch to key-based login and set PasswordAuthentication no, especially if port 22 is reachable from outside.",
				"the SSH server accepts password logins")
		case authNo:
			return okf("SSH accepts keys only")
		}
	}

	config, complete := sshdReadableConfig()
	state := parsePasswordAuth(config)
	// Refusing to guess matters here. The drop-ins in sshd_config.d are
	// commonly root-only, and an unreadable file that happens to disable
	// password auth would otherwise be reported as the opposite.
	if !complete && state == authUnset {
		return needsRootSkip("to read the whole SSH config")
	}
	if state == authYes || state == authUnset {
		return warnf("Switch to key-based login and set PasswordAuthentication no, especially if port 22 is reachable from outside.",
			"the SSH server accepts password logins")
	}
	return okf("SSH accepts keys only")
}

type serviceState int

const (
	serviceUnknown serviceState = iota
	serviceRunning
	serviceStopped
)

// launchdNoSuchService is launchctl's exit status for "Could not find
// service": the job isn't loaded, which for sshd means Remote Login is off.
const launchdNoSuchService = 113

// sshdUnits are every name sshd runs under on systemd hosts: Debian calls it
// ssh, Fedora sshd, and newer Ubuntu starts it from a socket unit.
var sshdUnits = []string{"ssh.service", "sshd.service", "ssh.socket", "sshd.socket"}

// sshdServing asks the service manager whether sshd is up, without root. It
// asks the service manager rather than dialing port 22 because sshd may listen
// elsewhere, and status promises no network calls.
func sshdServing(ctx context.Context, goos platform.OS) serviceState {
	switch goos {
	case platform.MacOS:
		_, err := captureErr(ctx, "launchctl", "print", "system/com.openssh.sshd")
		return launchdState(err)
	case platform.Linux:
		if !commandExists("systemctl") {
			return serviceUnknown
		}
		// is-active exits non-zero whenever any unit is down, so the output,
		// one state per unit, is the answer and the error is not.
		out, _ := captureErr(ctx, "systemctl", append([]string{"is-active"}, sshdUnits...)...)
		return systemdState(out, len(sshdUnits))
	}
	return serviceUnknown
}

// launchdState maps the result of `launchctl print` to a service state.
func launchdState(err error) serviceState {
	if err == nil {
		return serviceRunning
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == launchdNoSuchService {
		return serviceStopped
	}
	return serviceUnknown
}

// systemdState reads `systemctl is-active` output for want units. Any unit up
// means serving; it is stopped only when every unit answered and none is.
func systemdState(out string, want int) serviceState {
	lines := strings.Fields(out)
	for _, state := range lines {
		switch state {
		case "active", "activating", "reloading":
			return serviceRunning
		}
	}
	if len(lines) != want {
		return serviceUnknown
	}
	for _, state := range lines {
		if state != "inactive" && state != "failed" {
			return serviceUnknown
		}
	}
	return serviceStopped
}

// sshdReadableConfig concatenates the sshd configuration in the order sshd
// itself parses it, and reports whether every file could actually be read.
//
// Drop-ins come first because the shipped sshd_config puts its Include line at
// the top, and sshd takes the *first* occurrence of a keyword — so a drop-in
// beats the main file.
func sshdReadableConfig() (config string, complete bool) {
	complete = true
	var b strings.Builder

	for _, path := range globPaths("/etc/ssh/sshd_config.d/*.conf") {
		data, err := os.ReadFile(path)
		if err != nil {
			complete = false
			continue
		}
		b.Write(data)
		b.WriteString("\n")
	}

	data, err := os.ReadFile("/etc/ssh/sshd_config")
	if err != nil {
		complete = false
	} else {
		b.Write(data)
	}
	return b.String(), complete
}

type authState int

const (
	authUnset authState = iota
	authYes
	authNo
)

// parsePasswordAuth finds the effective PasswordAuthentication setting. It
// reads both sshd_config syntax ("PasswordAuthentication no") and the
// lowercased single-space form `sshd -T` emits ("passwordauthentication no").
func parsePasswordAuth(config string) authState {
	for _, line := range lines(config) {
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.EqualFold(fields[0], "PasswordAuthentication") {
			continue
		}
		if strings.EqualFold(fields[1], "yes") {
			return authYes
		}
		return authNo
	}
	return authUnset
}
