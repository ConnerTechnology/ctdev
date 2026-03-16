package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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

	ver, err := sysutil.GitHubLatestVersion("digitalocean/doctl")
	if err != nil {
		return err
	}
	fmt.Fprintf(opts.Stdout, "Installing doctl %s...\n", ver)

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download doctl v%s and install to /usr/local/bin\n", ver)
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "doctl-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archive := fmt.Sprintf("doctl-%s-linux-%s.tar.gz", ver, p.Arch)
	archivePath := filepath.Join(tmpDir, archive)
	checksumPath := filepath.Join(tmpDir, "checksums.txt")

	dlURL := fmt.Sprintf("https://github.com/digitalocean/doctl/releases/download/v%s/%s", ver, archive)
	csURL := fmt.Sprintf("https://github.com/digitalocean/doctl/releases/download/v%s/doctl-%s-checksums.sha256", ver, ver)

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
	return sysutil.InstallBinary(o, filepath.Join(tmpDir, "doctl"), "/usr/local/bin/doctl")
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
