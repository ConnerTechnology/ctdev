package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// acliFormula is the tap-qualified Homebrew formula, so `brew install` taps
// atlassian/homebrew-acli on its own.
const acliFormula = "atlassian/acli/acli"

const acliKeyring = "/usr/share/keyrings/acli-archive-keyring.gpg"

func acliInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && alreadyInstalled("acli") {
		fmt.Fprintln(opts.Stdout, "acli already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing acli...")

	switch p.PackageManager {
	case "brew":
		return sysutil.InstallPackage(ctx, o, acliFormula)
	case "apt":
		return sysutil.InstallAPTRepoPackage(ctx, o, sysutil.APTRepoPackage{
			KeyURL:      "https://acli.atlassian.com/gpg/public-key.asc",
			KeyringPath: acliKeyring,
			RepoLine:    fmt.Sprintf("deb [arch=%s signed-by=%s] https://acli.atlassian.com/linux/deb stable main", p.Arch, acliKeyring),
			SourceFile:  "acli.list",
			Packages:    []string{"acli"},
		})
	default:
		return unsupportedPMError("acli", p.PackageManager)
	}
}

func acliUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing acli...")

	switch p.PackageManager {
	case "brew":
		return sysutil.RemovePackage(ctx, o, acliFormula)
	case "apt":
		if err := sysutil.RemovePackage(ctx, o, "acli"); err != nil {
			return err
		}
		return sysutil.RemoveAPTRepo(ctx, o, "acli.list", acliKeyring)
	default:
		return ErrUnsupportedOS
	}
}
