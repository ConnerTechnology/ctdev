package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func terraformInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && sysutil.CommandExists("terraform") {
		fmt.Fprintln(opts.Stdout, "terraform already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing terraform...")

	switch p.PackageManager {
	case "brew":
		return sysutil.InstallPackage(ctx, o, "terraform")
	case "apt":
		if p.Codename == "" {
			return fmt.Errorf("could not determine distribution codename for APT repo")
		}
		keyring := "/usr/share/keyrings/hashicorp-archive-keyring.gpg"
		return sysutil.InstallAPTRepoPackage(ctx, o, sysutil.APTRepoPackage{
			KeyURL:      "https://apt.releases.hashicorp.com/gpg",
			KeyringPath: keyring,
			RepoLine:    fmt.Sprintf("deb [signed-by=%s] https://apt.releases.hashicorp.com %s main", keyring, p.Codename),
			SourceFile:  "hashicorp.list",
			Packages:    []string{"terraform"},
		})
	default:
		return unsupportedPMError("terraform", p.PackageManager)
	}
}

func terraformUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing terraform...")

	switch p.PackageManager {
	case "brew", "apt":
		if sysutil.IsPackageInstalled("terraform") {
			return sysutil.RemovePackage(ctx, o, "terraform")
		}
		fmt.Fprintln(opts.Stdout, "terraform package not installed")
		return nil
	default:
		// Fallback: remove a hand-placed standalone binary at the conventional path.
		return sysutil.SudoRun(ctx, o, "rm", "-f", "/usr/local/bin/terraform")
	}
}
