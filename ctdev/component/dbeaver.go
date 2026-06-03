package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func dbeaverInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && (sysutil.CommandExists("dbeaver") || sysutil.CommandExists("dbeaver-ce")) {
		fmt.Fprintln(opts.Stdout, "DBeaver already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing DBeaver...")

	switch p.PackageManager {
	case "brew":
		return sysutil.BrewCaskInstall(ctx, o, "dbeaver-community")
	case "apt":
		keyring := "/usr/share/keyrings/dbeaver-archive-keyring.gpg"
		if err := sysutil.AddAPTKeyring(ctx, o, "https://dbeaver.io/debs/dbeaver.gpg.key", keyring); err != nil {
			return fmt.Errorf("add dbeaver GPG key: %w", err)
		}
		repoLine := fmt.Sprintf("deb [signed-by=%s] https://dbeaver.io/debs/dbeaver-ce /", keyring)
		if err := sysutil.AddAPTSource(ctx, o, repoLine, "dbeaver.list"); err != nil {
			return fmt.Errorf("add dbeaver repo: %w", err)
		}
		if err := sysutil.APTUpdate(ctx, o); err != nil {
			return err
		}
		return sysutil.InstallPackage(ctx, o, "dbeaver-ce")
	default:
		return unsupportedPMError("dbeaver", p.PackageManager)
	}
}

func dbeaverUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing DBeaver...")

	switch p.PackageManager {
	case "brew":
		return sysutil.BrewCaskRemove(ctx, o, "dbeaver-community")
	case "apt":
		return sysutil.RemovePackage(ctx, o, "dbeaver-ce")
	default:
		return ErrUnsupportedOS
	}
}
