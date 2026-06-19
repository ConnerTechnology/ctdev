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
		return sysutil.InstallAPTRepoPackage(ctx, o, sysutil.APTRepoPackage{
			KeyURL:      "https://dbeaver.io/debs/dbeaver.gpg.key",
			KeyringPath: keyring,
			RepoLine:    fmt.Sprintf("deb [signed-by=%s] https://dbeaver.io/debs/dbeaver-ce /", keyring),
			SourceFile:  "dbeaver.list",
			Packages:    []string{"dbeaver-ce"},
		})
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
		if err := sysutil.RemovePackage(ctx, o, "dbeaver-ce"); err != nil {
			return err
		}
		return sysutil.RemoveAPTRepo(ctx, o, "dbeaver.list", "/usr/share/keyrings/dbeaver-archive-keyring.gpg")
	default:
		return ErrUnsupportedOS
	}
}
