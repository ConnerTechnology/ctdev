package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func terraformInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && sysutil.CommandExists("terraform") {
		fmt.Fprintln(opts.Stdout, "terraform already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing terraform...")

	switch p.PackageManager {
	case "brew":
		return sysutil.InstallPackage(ctx, o, "terraform")
	case "apt":
		if p.Codename == "" {
			return fmt.Errorf("could not determine distribution codename for APT repo")
		}
		keyring := "/usr/share/keyrings/hashicorp-archive-keyring.gpg"
		if err := sysutil.AddAPTKeyring(ctx, o, "https://apt.releases.hashicorp.com/gpg", keyring); err != nil {
			return fmt.Errorf("add hashicorp GPG key: %w", err)
		}
		repoLine := fmt.Sprintf("deb [signed-by=%s] https://apt.releases.hashicorp.com %s main", keyring, p.Codename)
		if err := sysutil.AddAPTSource(ctx, o, repoLine, "hashicorp.list"); err != nil {
			return fmt.Errorf("add hashicorp repo: %w", err)
		}
		if err := sysutil.APTUpdate(ctx, o); err != nil {
			return err
		}
		return sysutil.InstallPackage(ctx, o, "terraform")
	case "dnf":
		if err := sysutil.SudoRun(ctx, o, "dnf", "install", "-y", "dnf-plugins-core"); err != nil {
			return err
		}
		if err := sysutil.SudoRun(ctx, o, "dnf", "config-manager", "--add-repo", "https://rpm.releases.hashicorp.com/fedora/hashicorp.repo"); err != nil {
			return err
		}
		return sysutil.SudoRun(ctx, o, "dnf", "install", "-y", "terraform")
	case "pacman":
		return sysutil.InstallPackage(ctx, o, "terraform")
	default:
		return unsupportedPMError("terraform", p.PackageManager)
	}
}

func terraformUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing terraform...")

	// Mirror the install-side package-manager switch so we reach the right
	// removal path on every distro instead of probing with a dpkg-only
	// heuristic and falling through to a wrong binary path.
	switch p.PackageManager {
	case "brew", "apt", "dnf", "pacman":
		if sysutil.IsPackageInstalled("terraform") {
			return sysutil.RemovePackage(ctx, o, "terraform")
		}
		fmt.Fprintln(opts.Stdout, "terraform package not installed")
		return nil
	default:
		// Fallback: remove a hand-placed standalone binary at the conventional path.
		return sysutil.SudoRun(ctx, o, "rm", "-f", "/usr/local/bin/terraform")
	}
}
