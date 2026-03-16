package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func helmInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

	if !opts.Force && sysutil.CommandExists("helm") {
		fmt.Fprintln(opts.Stdout, "helm already installed")
		return nil
	}

	if p.OS == platform.MacOS {
		fmt.Fprintln(opts.Stdout, "Installing helm...")
		return sysutil.InstallPackage(o, "helm")
	}

	ver, err := sysutil.GitHubLatestVersion("helm/helm")
	if err != nil {
		return err
	}
	fmt.Fprintf(opts.Stdout, "Installing helm %s...\n", ver)

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download helm v%s and install to /usr/local/bin\n", ver)
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "helm-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archive := fmt.Sprintf("helm-v%s-linux-%s.tar.gz", ver, p.Arch)
	archivePath := filepath.Join(tmpDir, archive)
	checksumPath := filepath.Join(tmpDir, archive+".sha256sum")

	dlURL := fmt.Sprintf("https://get.helm.sh/%s", archive)
	csURL := fmt.Sprintf("https://get.helm.sh/%s.sha256sum", archive)

	if err := sysutil.DownloadFile(dlURL, archivePath); err != nil {
		return err
	}
	if err := sysutil.DownloadFile(csURL, checksumPath); err != nil {
		return err
	}
	if err := sysutil.VerifyChecksumFile(archivePath, checksumPath); err != nil {
		return err
	}
	if err := sysutil.Run(o, "tar", "-xzf", archivePath, "-C", tmpDir); err != nil {
		return err
	}
	return sysutil.InstallBinary(o, filepath.Join(tmpDir, fmt.Sprintf("linux-%s", p.Arch), "helm"), "/usr/local/bin/helm")
}

func helmUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing helm...")

	if p.OS == platform.MacOS {
		return sysutil.RemovePackage(o, "helm")
	}
	return sysutil.SudoRun(o, "rm", "-f", "/usr/local/bin/helm")
}
