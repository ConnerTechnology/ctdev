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
		return sysutil.InstallPackage(o, "terraform")
	case "apt":
		if p.Codename == "" {
			return fmt.Errorf("could not determine distribution codename for APT repo")
		}
		keyring := "/usr/share/keyrings/hashicorp-archive-keyring.gpg"
		if err := sysutil.AddAPTKeyring(o, "https://apt.releases.hashicorp.com/gpg", keyring); err != nil {
			return fmt.Errorf("add hashicorp GPG key: %w", err)
		}
		repoLine := fmt.Sprintf("deb [signed-by=%s] https://apt.releases.hashicorp.com %s main", keyring, p.Codename)
		if err := sysutil.AddAPTSource(o, repoLine, "hashicorp.list"); err != nil {
			return fmt.Errorf("add hashicorp repo: %w", err)
		}
		if err := sysutil.APTUpdate(o); err != nil {
			return err
		}
		return sysutil.InstallPackage(o, "terraform")
	case "dnf":
		if err := sysutil.SudoRun(o, "dnf", "install", "-y", "dnf-plugins-core"); err != nil {
			return err
		}
		if err := sysutil.SudoRun(o, "dnf", "config-manager", "--add-repo", "https://rpm.releases.hashicorp.com/fedora/hashicorp.repo"); err != nil {
			return err
		}
		return sysutil.SudoRun(o, "dnf", "install", "-y", "terraform")
	case "pacman":
		return sysutil.InstallPackage(o, "terraform")
	default:
		return fmt.Errorf("terraform install not supported for package manager: %s", p.PackageManager)
	}
}

func terraformUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing terraform...")

	if p.OS == platform.MacOS {
		return sysutil.RemovePackage(o, "terraform")
	}
	// On Linux, terraform may have been installed via apt/dnf or as a standalone binary
	if sysutil.IsPackageInstalled("terraform") {
		return sysutil.RemovePackage(o, "terraform")
	}
	// Fallback: remove standalone binary
	return sysutil.SudoRun(o, "rm", "-f", "/usr/local/bin/terraform")
}
