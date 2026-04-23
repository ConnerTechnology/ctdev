package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func helmInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && sysutil.CommandExists("helm") {
		fmt.Fprintln(opts.Stdout, "helm already installed")
		return nil
	}

	if p.OS == platform.MacOS {
		fmt.Fprintln(opts.Stdout, "Installing helm...")
		return sysutil.InstallPackage(ctx, o, "helm")
	}

	fmt.Fprintln(opts.Stdout, "Installing helm...")
	ver, err := sysutil.DownloadGitHubBinary(ctx, o, sysutil.GitHubBinarySpec{
		Repo: "helm/helm",
		ArchiveURL: func(ver, goos, goarch string) string {
			return fmt.Sprintf("https://get.helm.sh/helm-v%s-%s-%s.tar.gz", ver, goos, goarch)
		},
		ChecksumURL: func(ver, goos, goarch string) string {
			return fmt.Sprintf("https://get.helm.sh/helm-v%s-%s-%s.tar.gz.sha256sum", ver, goos, goarch)
		},
		BinaryPath: func(goos, goarch string) string {
			return fmt.Sprintf("%s-%s/helm", goos, goarch)
		},
		InstallDest: "/usr/local/bin/helm",
		ArchFormat:  "tar.gz",
	})
	if err != nil {
		return err
	}
	if !o.DryRun {
		fmt.Fprintf(opts.Stdout, "helm %s installed\n", ver)
	}
	return nil
}

func helmUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing helm...")

	if p.OS == platform.MacOS {
		return sysutil.RemovePackage(ctx, o, "helm")
	}
	return sysutil.SudoRun(ctx, o, "rm", "-f", "/usr/local/bin/helm")
}
