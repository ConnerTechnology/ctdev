package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func ageInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

	if !opts.Force && sysutil.CommandExists("age") {
		fmt.Fprintln(opts.Stdout, "age already installed")
		return nil
	}

	if p.OS == platform.MacOS {
		fmt.Fprintln(opts.Stdout, "Installing age...")
		return sysutil.InstallPackage(o, "age")
	}

	ver, err := sysutil.GitHubLatestVersion("FiloSottile/age")
	if err != nil {
		return err
	}
	fmt.Fprintf(opts.Stdout, "Installing age %s...\n", ver)

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download age v%s and install to /usr/local/bin\n", ver)
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "age-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archive := fmt.Sprintf("age-v%s-linux-%s.tar.gz", ver, p.Arch)
	archivePath := filepath.Join(tmpDir, archive)
	url := fmt.Sprintf("https://github.com/FiloSottile/age/releases/download/v%s/%s", ver, archive)

	if err := sysutil.DownloadFile(url, archivePath); err != nil {
		return err
	}
	if err := sysutil.Run(o, "tar", "-xzf", archivePath, "-C", tmpDir); err != nil {
		return err
	}
	if err := sysutil.InstallBinary(o, filepath.Join(tmpDir, "age", "age"), "/usr/local/bin/age"); err != nil {
		return err
	}
	return sysutil.InstallBinary(o, filepath.Join(tmpDir, "age", "age-keygen"), "/usr/local/bin/age-keygen")
}

func ageUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing age...")

	if p.OS == platform.MacOS {
		return sysutil.RemovePackage(o, "age")
	}
	_ = sysutil.SudoRun(o, "rm", "-f", "/usr/local/bin/age")
	return sysutil.SudoRun(o, "rm", "-f", "/usr/local/bin/age-keygen")
}
