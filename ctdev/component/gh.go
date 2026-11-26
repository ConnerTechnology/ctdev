package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
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
		return sysutil.InstallPackage(ctx, o, "gh")
	case "apt":
		k := APTRepoKeys["gh"]
		return sysutil.InstallAPTRepoPackage(ctx, o, sysutil.APTRepoPackage{
			KeyURL:      k.KeyURL,
			KeyringPath: k.KeyringPath,
			RepoLine:    fmt.Sprintf("deb [arch=%s signed-by=%s] %s stable main", p.Arch, k.KeyringPath, k.RepoURL),
			SourceFile:  "github-cli.list",
			Packages:    []string{"gh"},
		})
	default:
		return unsupportedPMError("gh", p.PackageManager)
	}
}

func ghUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing gh...")

	switch p.PackageManager {
	case "brew":
		return sysutil.RemovePackage(ctx, o, "gh")
	case "apt":
		if err := sysutil.RemovePackage(ctx, o, "gh"); err != nil {
			return err
		}
		return sysutil.RemoveAPTRepo(ctx, o, "github-cli.list", APTRepoKeys["gh"].KeyringPath)
	default:
		return ErrUnsupportedOS
	}
}
