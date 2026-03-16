package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func sopsInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

	if !opts.Force && sysutil.CommandExists("sops") {
		fmt.Fprintln(opts.Stdout, "sops already installed")
		return nil
	}

	if p.OS == platform.MacOS {
		fmt.Fprintln(opts.Stdout, "Installing sops...")
		return sysutil.InstallPackage(o, "sops")
	}

	ver, err := sysutil.GitHubLatestVersion("getsops/sops")
	if err != nil {
		return err
	}
	fmt.Fprintf(opts.Stdout, "Installing sops %s...\n", ver)

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download sops v%s and install to /usr/local/bin\n", ver)
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "sops-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	binaryName := fmt.Sprintf("sops-v%s.linux.%s", ver, p.Arch)
	binaryPath := filepath.Join(tmpDir, binaryName)
	checksumPath := filepath.Join(tmpDir, "checksums.txt")

	dlURL := fmt.Sprintf("https://github.com/getsops/sops/releases/download/v%s/%s", ver, binaryName)
	csURL := fmt.Sprintf("https://github.com/getsops/sops/releases/download/v%s/sops-v%s.checksums.txt", ver, ver)

	if err := sysutil.DownloadFile(dlURL, binaryPath); err != nil {
		return err
	}
	if err := sysutil.DownloadFile(csURL, checksumPath); err != nil {
		return err
	}
	if err := sysutil.VerifyChecksumFile(binaryPath, checksumPath); err != nil {
		return err
	}
	return sysutil.InstallBinary(o, binaryPath, "/usr/local/bin/sops")
}

func sopsUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing sops...")

	if p.OS == platform.MacOS {
		return sysutil.RemovePackage(o, "sops")
	}
	return sysutil.SudoRun(o, "rm", "-f", "/usr/local/bin/sops")
}
