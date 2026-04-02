package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func doctlInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

	if !opts.Force && sysutil.CommandExists("doctl") {
		fmt.Fprintln(opts.Stdout, "doctl already installed")
		return nil
	}

	if p.OS == platform.MacOS {
		fmt.Fprintln(opts.Stdout, "Installing doctl...")
		return sysutil.InstallPackage(o, "doctl")
	}

	fmt.Fprintln(opts.Stdout, "Installing doctl...")
	ver, err := sysutil.DownloadGitHubBinary(o, sysutil.GitHubBinarySpec{
		Repo: "digitalocean/doctl",
		ArchiveURL: func(ver, goos, goarch string) string {
			return fmt.Sprintf("https://github.com/digitalocean/doctl/releases/download/v%s/doctl-%s-%s-%s.tar.gz", ver, ver, goos, goarch)
		},
		ChecksumURL: func(ver, goos, goarch string) string {
			return fmt.Sprintf("https://github.com/digitalocean/doctl/releases/download/v%s/doctl-%s-checksums.sha256", ver, ver)
		},
		BinaryPath:  func(goos, goarch string) string { return "doctl" },
		InstallDest: "/usr/local/bin/doctl",
		ArchFormat:  "tar.gz",
	})
	if err != nil {
		return err
	}
	if !o.DryRun {
		fmt.Fprintf(opts.Stdout, "doctl %s installed\n", ver)
	}
	return nil
}

func doctlUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing doctl...")

	if p.OS == platform.MacOS {
		return sysutil.RemovePackage(o, "doctl")
	}
	return sysutil.SudoRun(o, "rm", "-f", "/usr/local/bin/doctl")
}
