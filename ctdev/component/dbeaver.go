package component

import (
	"context"
	"fmt"
	"os"

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
	case "dnf":
		archRPM := ""
		switch p.Arch {
		case "amd64":
			archRPM = "x86_64"
		case "arm64":
			archRPM = "aarch64"
		default:
			return fmt.Errorf("dbeaver RPM not available for architecture: %s", p.Arch)
		}
		if o.DryRun {
			fmt.Fprintf(o.Stdout, "[dry-run] download and install dbeaver-ce RPM for %s\n", archRPM)
			return nil
		}
		tmp, err := os.CreateTemp("", "dbeaver-ce-*.rpm")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		tmp.Close()

		url := fmt.Sprintf("https://dbeaver.io/files/dbeaver-ce-latest-stable.%s.rpm", archRPM)
		if err := sysutil.DownloadFile(ctx, url, tmp.Name()); err != nil {
			return fmt.Errorf("download dbeaver RPM: %w", err)
		}
		return sysutil.SudoRun(ctx, o, "dnf", "install", "-y", tmp.Name())
	case "pacman":
		return sysutil.SudoRun(ctx, o, "pacman", "-S", "--noconfirm", "dbeaver")
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
	case "dnf":
		return sysutil.RemovePackage(ctx, o, "dbeaver-ce")
	case "pacman":
		return sysutil.RemovePackage(ctx, o, "dbeaver")
	default:
		return ErrUnsupportedOS
	}
}
