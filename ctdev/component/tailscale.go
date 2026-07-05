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
		return sysutil.BrewCaskInstall(ctx, o, "tailscale")
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
		if err := sysutil.AddAPTKeyring(ctx, o, keyURL, keyring); err != nil {
			return fmt.Errorf("add tailscale GPG key: %w", err)
		}

		repoLine := fmt.Sprintf("deb [signed-by=%s] https://pkgs.tailscale.com/stable/%s %s main", keyring, distro, codename)
		if err := sysutil.AddAPTSource(ctx, o, repoLine, "tailscale.list"); err != nil {
			return fmt.Errorf("add tailscale repo: %w", err)
		}

		if err := sysutil.APTUpdate(ctx, o); err != nil {
			return err
		}
		if err := sysutil.InstallPackage(ctx, o, "tailscale"); err != nil {
			return err
		}

		// tailscaled must be running before 'tailscale up' can work; surface a
		// broken daemon now instead of at authentication time.
		if err := sysutil.ServiceEnable(ctx, o, "tailscaled"); err != nil {
			return fmt.Errorf("enable tailscaled service: %w", err)
		}
		if err := sysutil.ServiceStart(ctx, o, "tailscaled"); err != nil {
			return fmt.Errorf("start tailscaled service: %w", err)
		}

		fmt.Fprintln(opts.Stdout, "Run 'sudo tailscale up' to authenticate")
		return nil
	default:
		return unsupportedPMError("tailscale", p.PackageManager)
	}
}

func tailscaleUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Tailscale...")

	switch p.PackageManager {
	case "brew":
		return sysutil.BrewCaskRemove(ctx, o, "tailscale")
	case "apt":
		_ = sysutil.SudoRun(ctx, o, "systemctl", "stop", "tailscaled")
		_ = sysutil.SudoRun(ctx, o, "systemctl", "disable", "tailscaled")
		if err := sysutil.RemovePackage(ctx, o, "tailscale"); err != nil {
			return err
		}
		return sysutil.RemoveAPTRepo(ctx, o, "tailscale.list", "/usr/share/keyrings/tailscale-archive-keyring.gpg")
	default:
		return ErrUnsupportedOS
	}
}
