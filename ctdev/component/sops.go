package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func sopsInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && sysutil.CommandExists("sops") {
		fmt.Fprintln(opts.Stdout, "sops already installed")
		return nil
	}

	if p.OS == platform.MacOS {
		fmt.Fprintln(opts.Stdout, "Installing sops...")
		return sysutil.InstallPackage(ctx, o, "sops")
	}

	fmt.Fprintln(opts.Stdout, "Installing sops...")
	ver, err := sysutil.DownloadGitHubBinary(ctx, o, sysutil.GitHubBinarySpec{
		Repo: "getsops/sops",
		ArchiveURL: func(ver, goos, goarch string) string {
			return fmt.Sprintf("https://github.com/getsops/sops/releases/download/v%s/sops-v%s.%s.%s", ver, ver, goos, goarch)
		},
		ChecksumURL: func(ver, goos, goarch string) string {
			return fmt.Sprintf("https://github.com/getsops/sops/releases/download/v%s/sops-v%s.checksums.txt", ver, ver)
		},
		BinaryPath:  func(goos, goarch string) string { return "" },
		InstallDest: "/usr/local/bin/sops",
		ArchFormat:  "", // raw binary
	})
	if err != nil {
		return err
	}
	if !o.DryRun {
		fmt.Fprintf(opts.Stdout, "sops %s installed\n", ver)
	}
	return nil
}

func sopsUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing sops...")

	if p.OS == platform.MacOS {
		return sysutil.RemovePackage(ctx, o, "sops")
	}
	return sysutil.SudoRun(ctx, o, "rm", "-f", "/usr/local/bin/sops")
}
