package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// gwsFormula is the Homebrew core formula; it installs the `gws` binary.
const gwsFormula = "googleworkspace-cli"

// gwsLinuxArch maps Go arch names to the Rust target triples in the release
// asset names. Google publishes Linux builds for these two only.
var gwsLinuxArch = map[string]string{
	"amd64": "x86_64",
	"arm64": "aarch64",
}

func gwsInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && alreadyInstalled("gws") {
		fmt.Fprintln(opts.Stdout, "gws already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing gws...")

	if p.OS == platform.MacOS {
		return sysutil.InstallPackage(ctx, o, gwsFormula)
	}

	triple, ok := gwsLinuxArch[p.Arch]
	if !ok {
		return fmt.Errorf("gws Linux build only available for amd64/arm64 (got %s): %w", p.Arch, ErrUnsupportedOS)
	}

	// No APT repo: Linux gets the static musl build from the latest GitHub
	// release, checked against the SHA-256 file published beside each asset.
	asset := func(ver string) string {
		return fmt.Sprintf("https://github.com/googleworkspace/cli/releases/download/v%s/google-workspace-cli-%s-unknown-linux-musl.tar.gz", ver, triple)
	}
	ver, err := sysutil.DownloadGitHubBinary(ctx, o, sysutil.GitHubBinarySpec{
		Repo:        "googleworkspace/cli",
		ArchiveURL:  func(ver, goos, goarch string) string { return asset(ver) },
		ChecksumURL: func(ver, goos, goarch string) string { return asset(ver) + ".sha256" },
		BinaryPath:  func(goos, goarch string) string { return "gws" },
		InstallDest: "/usr/local/bin/gws",
		ArchFormat:  "tar.gz",
	})
	if err != nil {
		return err
	}
	if !o.DryRun {
		fmt.Fprintf(opts.Stdout, "gws %s installed\n", ver)
	}
	return nil
}

func gwsUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing gws...")

	if p.OS == platform.MacOS {
		return sysutil.RemovePackage(ctx, o, gwsFormula)
	}
	return sysutil.SudoRun(ctx, o, "rm", "-f", "/usr/local/bin/gws")
}
