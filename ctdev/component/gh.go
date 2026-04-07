package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func ghInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && sysutil.CommandExists("gh") {
		fmt.Fprintln(opts.Stdout, "gh already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing gh...")

	switch p.PackageManager {
	case "brew":
		return sysutil.InstallPackage(o, "gh")
	case "apt":
		keyring := "/usr/share/keyrings/githubcli-archive-keyring.gpg"
		if err := sysutil.AddAPTKeyring(o, "https://cli.github.com/packages/githubcli-archive-keyring.gpg", keyring); err != nil {
			return fmt.Errorf("add gh GPG key: %w", err)
		}
		repoLine := fmt.Sprintf("deb [arch=%s signed-by=%s] https://cli.github.com/packages stable main", p.Arch, keyring)
		if err := sysutil.AddAPTSource(o, repoLine, "github-cli.list"); err != nil {
			return fmt.Errorf("add gh repo: %w", err)
		}
		if err := sysutil.APTUpdate(o); err != nil {
			return err
		}
		return sysutil.InstallPackage(o, "gh")
	case "dnf":
		return sysutil.InstallPackage(o, "gh")
	case "pacman":
		return sysutil.InstallPackage(o, "github-cli")
	default:
		return fmt.Errorf("gh install not supported for package manager: %s", p.PackageManager)
	}
}

func ghUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing gh...")

	switch p.PackageManager {
	case "brew":
		return sysutil.RemovePackage(o, "gh")
	case "apt":
		return sysutil.RemovePackage(o, "gh")
	case "dnf":
		return sysutil.RemovePackage(o, "gh")
	case "pacman":
		return sysutil.RemovePackage(o, "github-cli")
	default:
		return ErrUnsupportedOS
	}
}
