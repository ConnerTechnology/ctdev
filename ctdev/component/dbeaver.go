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
		return sysutil.BrewCaskInstall(o, "dbeaver-community")
	case "apt":
		keyring := "/etc/apt/trusted.gpg.d/dbeaver.gpg"
		if err := sysutil.AddAPTKeyring(o, "https://dbeaver.io/debs/dbeaver.gpg.key", keyring); err != nil {
			return fmt.Errorf("add dbeaver GPG key: %w", err)
		}
		repoLine := fmt.Sprintf("deb [signed-by=%s] https://dbeaver.io/debs/dbeaver-ce /", keyring)
		if err := sysutil.AddAPTSource(o, repoLine, "dbeaver.list"); err != nil {
			return fmt.Errorf("add dbeaver repo: %w", err)
		}
		if err := sysutil.APTUpdate(o); err != nil {
			return err
		}
		return sysutil.InstallPackage(o, "dbeaver-ce")
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
		if err := sysutil.DownloadFile(url, tmp.Name()); err != nil {
			return fmt.Errorf("download dbeaver RPM: %w", err)
		}
		return sysutil.SudoRun(o, "dnf", "install", "-y", tmp.Name())
	case "pacman":
		return sysutil.SudoRun(o, "pacman", "-S", "--noconfirm", "dbeaver")
	default:
		return fmt.Errorf("dbeaver install not supported for package manager: %s", p.PackageManager)
	}
}

func dbeaverUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing DBeaver...")

	switch p.PackageManager {
	case "brew":
		return sysutil.BrewCaskRemove(o, "dbeaver-community")
	case "apt":
		return sysutil.RemovePackage(o, "dbeaver-ce")
	case "dnf":
		return sysutil.RemovePackage(o, "dbeaver-ce")
	case "pacman":
		return sysutil.RemovePackage(o, "dbeaver")
	default:
		return ErrUnsupportedOS
	}
}
