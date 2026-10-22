package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// restic installs only the restic binary. Backups run as the user, so there is
// nothing to deploy under /etc and nothing to schedule here: per-folder backup
// (CTD-87) drives restic itself.
func resticInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if p.PackageManager != "apt" && p.PackageManager != "brew" {
		return unsupportedPMError("restic", p.PackageManager)
	}
	if !opts.Force && alreadyInstalled("restic") {
		fmt.Fprintln(opts.Stdout, "restic already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing restic...")
	return sysutil.InstallPackage(ctx, o, "restic")
}

func resticUninstall(ctx context.Context, opts ExecOpts) error {
	fmt.Fprintln(opts.Stdout, "Removing restic...")
	return sysutil.RemovePackage(ctx, execOpts(opts), "restic")
}
