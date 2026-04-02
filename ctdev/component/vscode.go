package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func vscodeInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

	if !opts.Force && sysutil.CommandExists("code") {
		fmt.Fprintln(opts.Stdout, "VS Code already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing VS Code...")

	switch p.PackageManager {
	case "brew":
		return sysutil.BrewCaskInstall(o, "visual-studio-code")
	case "apt":
		keyring := "/etc/apt/trusted.gpg.d/packages.microsoft.gpg"
		if err := sysutil.AddAPTKeyring(o, "https://packages.microsoft.com/keys/microsoft.asc", keyring); err != nil {
			return fmt.Errorf("add microsoft GPG key: %w", err)
		}
		repoLine := fmt.Sprintf("deb [arch=amd64,arm64,armhf signed-by=%s] https://packages.microsoft.com/repos/code stable main", keyring)
		if err := sysutil.AddAPTSource(o, repoLine, "vscode.list"); err != nil {
			return fmt.Errorf("add vscode repo: %w", err)
		}
		if err := sysutil.APTUpdate(o); err != nil {
			return err
		}
		return sysutil.InstallPackage(o, "code")
	case "dnf":
		// Add Microsoft repo
		if err := sysutil.SudoRun(o, "rpm", "--import", "https://packages.microsoft.com/keys/microsoft.asc"); err != nil {
			return fmt.Errorf("import microsoft GPG key: %w", err)
		}
		repoContent := "[code]\nname=Visual Studio Code\nbaseurl=https://packages.microsoft.com/yumrepos/vscode\nenabled=1\ngpgcheck=1\ngpgkey=https://packages.microsoft.com/keys/microsoft.asc"
		if err := sysutil.SudoRun(o, "bash", "-c", fmt.Sprintf("echo -e '%s' > /etc/yum.repos.d/vscode.repo", repoContent)); err != nil {
			return fmt.Errorf("add vscode repo: %w", err)
		}
		return sysutil.SudoRun(o, "dnf", "install", "-y", "code")
	default:
		return fmt.Errorf("VS Code install not supported for package manager: %s", p.PackageManager)
	}
}

func vscodeUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing VS Code...")

	if p.OS == platform.MacOS {
		return sysutil.BrewCaskRemove(o, "visual-studio-code")
	}
	if p.PackageManager == "apt" || p.PackageManager == "dnf" {
		return sysutil.RemovePackage(o, "code")
	}
	return ErrUnsupportedOS
}
