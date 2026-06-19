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
	o := execOpts(opts)

	if p.OS == platform.MacOS {
		if !opts.Force && sysutil.CommandExists("1password") {
			fmt.Fprintln(opts.Stdout, "1Password already installed")
			return nil
		}
		fmt.Fprintln(opts.Stdout, "Installing 1Password...")
		return sysutil.BrewCaskInstall(ctx, o, "1password")
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
	if err := sysutil.AddAPTKeyring(ctx, o, keyURL, keyring); err != nil {
		return fmt.Errorf("add 1password GPG key: %w", err)
	}

	repoLine := fmt.Sprintf("deb [arch=amd64 signed-by=%s] https://downloads.1password.com/linux/debian/amd64 stable main", keyring)
	if err := sysutil.AddAPTSource(ctx, o, repoLine, "1password.list"); err != nil {
		return fmt.Errorf("add 1password repo: %w", err)
	}

	// Debsig policies
	if !o.DryRun {
		policyDir := "/etc/debsig/policies/AC2D62742012EA22"
		if err := sysutil.SudoRun(ctx, o, "mkdir", "-p", policyDir); err != nil {
			return fmt.Errorf("create debsig policy dir: %w", err)
		}
		polTmp, err := os.CreateTemp("", "1password-pol-*")
		if err != nil {
			return err
		}
		defer os.Remove(polTmp.Name())
		polTmp.Close()
		if err := sysutil.DownloadFile(ctx, "https://downloads.1password.com/linux/debian/debsig/1password.pol", polTmp.Name()); err != nil {
			return fmt.Errorf("download debsig policy: %w", err)
		}
		if err := sysutil.SudoRun(ctx, o, "cp", polTmp.Name(), policyDir+"/1password.pol"); err != nil {
			return fmt.Errorf("install debsig policy: %w", err)
		}
		debsigKeyDir := "/usr/share/debsig/keyrings/AC2D62742012EA22"
		if err := sysutil.SudoRun(ctx, o, "mkdir", "-p", debsigKeyDir); err != nil {
			return fmt.Errorf("create debsig keyring dir: %w", err)
		}
		if err := sysutil.AddAPTKeyring(ctx, o, keyURL, debsigKeyDir+"/debsig.gpg"); err != nil {
			return fmt.Errorf("add debsig keyring: %w", err)
		}
	}

	if err := sysutil.APTUpdate(ctx, o); err != nil {
		return err
	}
	return sysutil.InstallPackage(ctx, o, "1password")
}

func onePasswordUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing 1Password...")

	if p.OS == platform.MacOS {
		return sysutil.BrewCaskRemove(ctx, o, "1password")
	}
	if p.PackageManager == "apt" {
		if err := sysutil.RemovePackage(ctx, o, "1password"); err != nil {
			return err
		}
		// Remove the debsig policy + keyring dirs the installer created.
		_ = sysutil.SudoRun(ctx, o, "rm", "-rf", "/etc/debsig/policies/AC2D62742012EA22")
		_ = sysutil.SudoRun(ctx, o, "rm", "-rf", "/usr/share/debsig/keyrings/AC2D62742012EA22")
		return sysutil.RemoveAPTRepo(ctx, o, "1password.list", "/usr/share/keyrings/1password-archive-keyring.gpg")
	}
	return ErrUnsupportedOS
}
