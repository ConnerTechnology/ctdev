package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func tailscaleInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && sysutil.CommandExists("tailscale") {
		fmt.Fprintln(opts.Stdout, "Tailscale already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing Tailscale...")

	switch p.PackageManager {
	case "brew":
		return sysutil.BrewCaskInstall(o, "tailscale")
	case "apt":
		codename := p.Codename
		if codename == "" {
			codename = "noble"
		}
		distro := p.Distro
		if distro == "" {
			distro = "ubuntu"
		}
		// Tailscale only publishes repos for ubuntu and debian
		switch distro {
		case "ubuntu", "linuxmint", "pop":
			distro = "ubuntu"
		case "debian":
			distro = "debian"
		default:
			distro = "ubuntu"
		}

		keyring := "/usr/share/keyrings/tailscale-archive-keyring.gpg"
		keyURL := fmt.Sprintf("https://pkgs.tailscale.com/stable/%s/%s.noarmor.gpg", distro, codename)
		if err := sysutil.AddAPTKeyring(o, keyURL, keyring); err != nil {
			return fmt.Errorf("add tailscale GPG key: %w", err)
		}

		repoLine := fmt.Sprintf("deb [signed-by=%s] https://pkgs.tailscale.com/stable/%s %s main", keyring, distro, codename)
		if err := sysutil.AddAPTSource(o, repoLine, "tailscale.list"); err != nil {
			return fmt.Errorf("add tailscale repo: %w", err)
		}

		if err := sysutil.APTUpdate(o); err != nil {
			return err
		}
		if err := sysutil.InstallPackage(o, "tailscale"); err != nil {
			return err
		}

		_ = sysutil.ServiceEnable(o, "tailscaled")
		_ = sysutil.ServiceStart(o, "tailscaled")

		fmt.Fprintln(opts.Stdout, "Run 'sudo tailscale up' to authenticate")
		return nil
	default:
		return fmt.Errorf("tailscale install not supported for package manager: %s", p.PackageManager)
	}
}

func tailscaleUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Tailscale...")

	switch p.PackageManager {
	case "brew":
		return sysutil.BrewCaskRemove(o, "tailscale")
	case "apt":
		_ = sysutil.SudoRun(o, "systemctl", "stop", "tailscaled")
		_ = sysutil.SudoRun(o, "systemctl", "disable", "tailscaled")
		if err := sysutil.RemovePackage(o, "tailscale"); err != nil {
			return err
		}
		_ = sysutil.SudoRun(o, "rm", "-f", "/etc/apt/sources.list.d/tailscale.list")
		_ = sysutil.SudoRun(o, "rm", "-f", "/usr/share/keyrings/tailscale-archive-keyring.gpg")
		return nil
	default:
		return ErrUnsupportedOS
	}
}
