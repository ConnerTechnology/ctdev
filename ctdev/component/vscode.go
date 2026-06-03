package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func vscodeInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && sysutil.CommandExists("code") {
		fmt.Fprintln(opts.Stdout, "VS Code already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing VS Code...")

	switch p.PackageManager {
	case "brew":
		return sysutil.BrewCaskInstall(ctx, o, "visual-studio-code")
	case "apt":
		keyring := "/usr/share/keyrings/microsoft-archive-keyring.gpg"
		if err := sysutil.AddAPTKeyring(ctx, o, "https://packages.microsoft.com/keys/microsoft.asc", keyring); err != nil {
			return fmt.Errorf("add microsoft GPG key: %w", err)
		}
		repoLine := fmt.Sprintf("deb [arch=amd64,arm64,armhf signed-by=%s] https://packages.microsoft.com/repos/code stable main", keyring)
		if err := sysutil.AddAPTSource(ctx, o, repoLine, "vscode.list"); err != nil {
			return fmt.Errorf("add vscode repo: %w", err)
		}
		if err := sysutil.APTUpdate(ctx, o); err != nil {
			return err
		}
		return sysutil.InstallPackage(ctx, o, "code")
	default:
		return unsupportedPMError("VS Code", p.PackageManager)
	}
}

func vscodeUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing VS Code...")

	if p.OS == platform.MacOS {
		return sysutil.BrewCaskRemove(ctx, o, "visual-studio-code")
	}
	if p.PackageManager == "apt" {
		return sysutil.RemovePackage(ctx, o, "code")
	}
	return ErrUnsupportedOS
}
