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
		return sysutil.InstallPackage(ctx, o, "gh")
	case "apt":
		keyring := "/usr/share/keyrings/githubcli-archive-keyring.gpg"
		return sysutil.InstallAPTRepoPackage(ctx, o, sysutil.APTRepoPackage{
			KeyURL:      "https://cli.github.com/packages/githubcli-archive-keyring.gpg",
			KeyringPath: keyring,
			RepoLine:    fmt.Sprintf("deb [arch=%s signed-by=%s] https://cli.github.com/packages stable main", p.Arch, keyring),
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
		return sysutil.RemoveAPTRepo(ctx, o, "github-cli.list", "/usr/share/keyrings/githubcli-archive-keyring.gpg")
	default:
		return ErrUnsupportedOS
	}
}
