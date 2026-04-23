package component

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func gitSpiceInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && isGitSpiceInstalled() {
		fmt.Fprintln(opts.Stdout, "git-spice already installed")
		return nil
	}

	if p.OS == platform.MacOS {
		fmt.Fprintln(opts.Stdout, "Installing git-spice...")
		return sysutil.InstallPackage(ctx, o, "git-spice")
	}

	fmt.Fprintln(opts.Stdout, "Installing git-spice...")
	ver, err := sysutil.DownloadGitHubBinary(ctx, o, sysutil.GitHubBinarySpec{
		Repo: "abhinav/git-spice",
		ArchiveURL: func(ver, goos, goarch string) string {
			arch := goarch
			switch goarch {
			case "amd64":
				arch = "x86_64"
			case "arm64":
				arch = "aarch64"
			}
			return fmt.Sprintf("https://github.com/abhinav/git-spice/releases/download/v%s/git-spice_%s.%s_%s.tar.gz", ver, ver, goos, arch)
		},
		ChecksumURL: func(ver, goos, goarch string) string {
			return fmt.Sprintf("https://github.com/abhinav/git-spice/releases/download/v%s/checksums.txt", ver)
		},
		BinaryPath:  func(goos, goarch string) string { return "gs" },
		InstallDest: "/usr/local/bin/gs",
		ArchFormat:  "tar.gz",
	})
	if err != nil {
		return err
	}
	if !o.DryRun {
		fmt.Fprintf(opts.Stdout, "git-spice %s installed\n", ver)
	}
	return nil
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
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing git-spice...")

	if p.OS == platform.MacOS {
		return sysutil.RemovePackage(ctx, o, "git-spice")
	}
	// `gs` collides with Ghostscript's binary. Only remove when we can confirm
	// it is actually git-spice so we don't nuke an unrelated binary.
	if !isGitSpiceInstalled() {
		fmt.Fprintln(opts.Stdout, "git-spice not detected at /usr/local/bin/gs; leaving untouched")
		return nil
	}
	return sysutil.SudoRun(ctx, o, "rm", "-f", "/usr/local/bin/gs")
}
