package component

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/setup"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// The shell-outs go through package vars so tests can record them.
var (
	sshSudo          = sysutil.SudoRun
	sshEnsureLinux   = setup.EnsureSSHServer
	sshDisableLinux  = setup.DisableSSHServer
	sshLinuxAtBoot   = setup.SSHServerStartsAtBoot
	sshRemovePackage = sysutil.RemovePackage
	sshRemoteLoginOn = remoteLoginOn
)

// remoteLoginHint is the way round systemsetup, which macOS refuses from a
// terminal without Full Disk Access.
const remoteLoginHint = "if macOS asked for Full Disk Access, use the Remote Login switch in System Settings > General > Sharing instead"

// remoteLoginOn reports whether macOS Remote Login is on: launchd has the
// sshd job loaded only while it is, and asking needs no root.
func remoteLoginOn(ctx context.Context) bool {
	return exec.CommandContext(ctx, "launchctl", "print", "system/com.openssh.sshd").Run() == nil
}

func sshServerStartsAtBoot() bool {
	ctx := context.Background()
	if platform.Detect().OS == platform.MacOS {
		return sshRemoteLoginOn(ctx)
	}
	return sshLinuxAtBoot(ctx)
}

func sshInstall(ctx context.Context, opts ExecOpts) error {
	return sshInstallWith(ctx, opts, platform.Detect().PackageManager)
}

func sshInstallWith(ctx context.Context, opts ExecOpts, pm string) error {
	o := execOpts(opts)
	switch pm {
	case "brew":
		if !opts.Force && sshRemoteLoginOn(ctx) {
			fmt.Fprintln(opts.Stdout, "Remote Login already on")
			return nil
		}
		fmt.Fprintln(opts.Stdout, "Turning on Remote Login...")
		// macOS keeps the setting across restarts. Since Ventura, systemsetup
		// refuses unless the terminal has Full Disk Access.
		// systemsetup can print its refusal and still exit 0, so the state
		// launchd reports afterwards is the answer.
		if err := sshSudo(ctx, o, "systemsetup", "-setremotelogin", "on"); err != nil {
			return fmt.Errorf("turn on Remote Login: %w; %s", err, remoteLoginHint)
		}
		if !opts.DryRun && !sshRemoteLoginOn(ctx) {
			return fmt.Errorf("turn on Remote Login: still off after systemsetup; %s", remoteLoginHint)
		}
		return nil
	case "apt":
		fmt.Fprintln(opts.Stdout, "Setting up the SSH server...")
		return sshEnsureLinux(ctx, o)
	default:
		return unsupportedPMError("ssh", pm)
	}
}

func sshUninstall(ctx context.Context, opts ExecOpts) error {
	return sshUninstallWith(ctx, opts, platform.Detect().PackageManager)
}

func sshUninstallWith(ctx context.Context, opts ExecOpts, pm string) error {
	o := execOpts(opts)
	switch pm {
	case "brew":
		fmt.Fprintln(opts.Stdout, "Turning off Remote Login...")
		// -f skips the prompt that warns about cutting off remote sessions.
		if err := sshSudo(ctx, o, "systemsetup", "-f", "-setremotelogin", "off"); err != nil {
			return fmt.Errorf("turn off Remote Login: %w; %s", err, remoteLoginHint)
		}
		return nil
	case "apt":
		fmt.Fprintln(opts.Stdout, "Removing the SSH server...")
		if err := sshDisableLinux(ctx, o); err != nil {
			return err
		}
		return sshRemovePackage(ctx, o, "openssh-server")
	default:
		return unsupportedPMError("ssh", pm)
	}
}
