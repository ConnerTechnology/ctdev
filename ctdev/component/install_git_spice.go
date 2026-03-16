package component

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func gitSpiceInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

	if !opts.Force && isGitSpiceInstalled() {
		fmt.Fprintln(opts.Stdout, "git-spice already installed")
		return nil
	}

	if p.OS == platform.MacOS {
		fmt.Fprintln(opts.Stdout, "Installing git-spice...")
		return sysutil.InstallPackage(o, "git-spice")
	}

	ver, err := sysutil.GitHubLatestVersion("abhinav/git-spice")
	if err != nil {
		return err
	}
	fmt.Fprintf(opts.Stdout, "Installing git-spice %s...\n", ver)

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download git-spice v%s and install to /usr/local/bin\n", ver)
		return nil
	}

	// Map architecture to git-spice naming
	gsArch := p.Arch
	switch p.Arch {
	case "amd64":
		gsArch = "x86_64"
	case "arm64":
		gsArch = "aarch64"
	}

	tmpDir, err := os.MkdirTemp("", "git-spice-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archive := fmt.Sprintf("git-spice.Linux-%s.tar.gz", gsArch)
	archivePath := filepath.Join(tmpDir, archive)
	checksumPath := filepath.Join(tmpDir, "checksums.txt")

	dlURL := fmt.Sprintf("https://github.com/abhinav/git-spice/releases/download/v%s/%s", ver, archive)
	csURL := fmt.Sprintf("https://github.com/abhinav/git-spice/releases/download/v%s/checksums.txt", ver)

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
	return sysutil.InstallBinary(o, filepath.Join(tmpDir, "gs"), "/usr/local/bin/gs")
}

// isGitSpiceInstalled checks if gs is git-spice (not Ghostscript).
func isGitSpiceInstalled() bool {
	out, err := exec.Command("gs", "--version").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "git-spice")
}

func gitSpiceUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing git-spice...")

	if p.OS == platform.MacOS {
		return sysutil.RemovePackage(o, "git-spice")
	}
	// Could be in /usr/local/bin or ~/go/bin
	home, _ := os.UserHomeDir()
	_ = sysutil.Run(o, "rm", "-f", filepath.Join(home, "go", "bin", "gs"))
	return sysutil.SudoRun(o, "rm", "-f", "/usr/local/bin/gs")
}
