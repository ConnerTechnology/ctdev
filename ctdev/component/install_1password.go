package component

import (
	"context"
	"fmt"
	"os"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func onePasswordInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

	if p.OS == platform.MacOS {
		if !opts.Force && sysutil.CommandExists("1password") {
			fmt.Fprintln(opts.Stdout, "1Password already installed")
			return nil
		}
		fmt.Fprintln(opts.Stdout, "Installing 1Password...")
		return sysutil.BrewCaskInstall(o, "1password")
	}

	if p.PackageManager != "apt" {
		return ErrUnsupportedOS
	}

	if !opts.Force && sysutil.CommandExists("1password") {
		fmt.Fprintln(opts.Stdout, "1Password already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing 1Password...")

	keyURL := "https://downloads.1password.com/linux/keys/1password.asc"
	keyring := "/usr/share/keyrings/1password-archive-keyring.gpg"
	if err := sysutil.AddAPTKeyring(o, keyURL, keyring); err != nil {
		return fmt.Errorf("add 1password GPG key: %w", err)
	}

	repoLine := fmt.Sprintf("deb [arch=amd64 signed-by=%s] https://downloads.1password.com/linux/debian/amd64 stable main", keyring)
	if err := sysutil.AddAPTSource(o, repoLine, "1password.list"); err != nil {
		return fmt.Errorf("add 1password repo: %w", err)
	}

	// Debsig policies
	if !o.DryRun {
		policyDir := "/etc/debsig/policies/AC2D62742012EA22"
		os.MkdirAll(policyDir, 0755)
		if err := sysutil.DownloadFile("https://downloads.1password.com/linux/debian/debsig/1password.pol", policyDir+"/1password.pol"); err != nil {
			return fmt.Errorf("download debsig policy: %w", err)
		}
		debsigKeyDir := "/usr/share/debsig/keyrings/AC2D62742012EA22"
		os.MkdirAll(debsigKeyDir, 0755)
		if err := sysutil.AddAPTKeyring(o, keyURL, debsigKeyDir+"/debsig.gpg"); err != nil {
			return fmt.Errorf("add debsig keyring: %w", err)
		}
	}

	if err := sysutil.APTUpdate(o); err != nil {
		return err
	}
	return sysutil.InstallPackage(o, "1password")
}

func onePasswordUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing 1Password...")

	if p.OS == platform.MacOS {
		return sysutil.BrewCaskRemove(o, "1password")
	}
	if p.PackageManager == "apt" {
		return sysutil.RemovePackage(o, "1password")
	}
	return ErrUnsupportedOS
}
