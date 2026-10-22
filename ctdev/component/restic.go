package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// resticVersion is the official release installed on Linux. Per-folder backup
// reads JSON fields restic added in 0.17, and Ubuntu's apt ships 0.12 (22.04)
// and 0.16 (24.04), so Linux takes the upstream binary instead of apt's.
const resticVersion = "0.19.1"

// resticUserBin is where the Linux binary goes: ~/.local/bin, beside ctdev
// itself, so neither install nor uninstall needs root.
func resticUserBin() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin", "restic"), nil
}

func resticInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	if !opts.Force && alreadyInstalled("restic") {
		fmt.Fprintln(opts.Stdout, "restic already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing restic...")

	if p.OS == platform.MacOS {
		return sysutil.InstallPackage(ctx, o, "restic")
	}
	if p.OS != platform.Linux {
		return ErrUnsupportedOS
	}

	dest, err := resticUserBin()
	if err != nil {
		return err
	}
	ver, err := sysutil.DownloadGitHubBinary(ctx, o, sysutil.GitHubBinarySpec{
		Repo:    "restic/restic",
		Version: resticVersion,
		ArchiveURL: func(ver, goos, goarch string) string {
			return fmt.Sprintf("https://github.com/restic/restic/releases/download/v%s/restic_%s_%s_%s.bz2", ver, ver, goos, goarch)
		},
		ChecksumURL: func(ver, goos, goarch string) string {
			return fmt.Sprintf("https://github.com/restic/restic/releases/download/v%s/SHA256SUMS", ver)
		},
		InstallDest: dest,
		ArchFormat:  "bz2",
		UserInstall: true,
	})
	if err != nil {
		return err
	}
	if !o.DryRun {
		fmt.Fprintf(opts.Stdout, "restic %s installed to %s\n", ver, dest)
	}
	return nil
}

func resticUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing restic...")
	if p.OS == platform.MacOS {
		return sysutil.RemovePackage(ctx, o, "restic")
	}
	dest, err := resticUserBin()
	if err != nil {
		return err
	}
	return sysutil.Run(ctx, o, "rm", "-f", dest)
}
