package component

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// Orca (https://orca.dev) is Stably AI's desktop app for running coding agents
// in parallel worktrees. Desktop builds are GitHub releases of stablyai/orca;
// its mobile apps publish pre-releases there too, so /releases/latest is the
// desktop channel.
const (
	OrcaRepo        = "stablyai/orca"
	orcaReleaseBase = "https://github.com/stablyai/orca/releases/download"
	orcaDebPackage  = "orca-ide"
	orcaMacApp      = "/Applications/Orca.app"
	// The core Homebrew "orca" cask is an unrelated Plotly tool, so the cask is
	// named through Stably's tap, which brew taps on first use.
	orcaCask = "stablyai/orca/orca"
)

func orcaInstalled() bool {
	switch runtime.GOOS {
	case "darwin":
		_, err := os.Stat(orcaMacApp)
		return err == nil
	case "linux":
		return sysutil.DebInstalled(context.Background(), orcaDebPackage)
	}
	return false
}

// OrcaVersion returns the installed Linux package's version, or "" when it
// isn't installed. The macOS app updates itself, so it has no counterpart.
func OrcaVersion(ctx context.Context) string {
	return sysutil.DebVersion(ctx, orcaDebPackage)
}

// orcaDebAsset names the release's .deb and the electron-builder manifest that
// carries its sha512, for a Debian architecture.
func orcaDebAsset(arch, ver string) (deb, manifest string, err error) {
	switch arch {
	case "amd64":
		return fmt.Sprintf("orca-ide_%s_amd64.deb", ver), "latest-linux.yml", nil
	case "arm64":
		return fmt.Sprintf("orca-ide_%s_arm64.deb", ver), "latest-linux-arm64.yml", nil
	}
	return "", "", fmt.Errorf("no Orca build for %s (requires amd64 or arm64): %w", arch, ErrUnsupportedOS)
}

func orcaInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	switch p.OS {
	case platform.MacOS:
	case platform.Linux:
		if p.PackageManager != "apt" {
			return unsupportedPMError("Orca", p.PackageManager)
		}
	default:
		return ErrUnsupportedOS
	}

	if !opts.Force && alreadyInstalled("orca") {
		fmt.Fprintln(opts.Stdout, "Orca already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing Orca...")

	if p.OS == platform.MacOS {
		// The cask is marked auto_updates: the app updates itself. It also
		// links the bundled `orca` CLI onto PATH.
		return sysutil.BrewCaskInstallVerified(ctx, o, orcaCask, orcaInstalled)
	}
	ver, err := sysutil.GitHubLatestVersion(ctx, OrcaRepo)
	if err != nil {
		return err
	}
	return installOrcaDeb(ctx, o, p.Arch, ver)
}

// installOrcaDeb installs release ver's .deb. It is also how `ctdev update`
// moves the Linux app forward, so it always installs, even over itself.
func installOrcaDeb(ctx context.Context, o sysutil.Opts, arch, ver string) error {
	deb, manifest, err := orcaDebAsset(arch, ver)
	if err != nil {
		return err
	}
	base := fmt.Sprintf("%s/v%s/", orcaReleaseBase, ver)
	return installElectronDeb(ctx, o, base, deb, manifest, orcaDebPackage)
}

func orcaUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Orca...")

	switch p.OS {
	case platform.MacOS:
		return sysutil.BrewCaskRemove(ctx, o, orcaCask)
	case platform.Linux:
		if p.PackageManager != "apt" {
			return unsupportedPMError("Orca", p.PackageManager)
		}
		return sysutil.RemovePackage(ctx, o, orcaDebPackage)
	}
	return ErrUnsupportedOS
}

// OrcaUpdate installs release ver's .deb over the installed one, for
// `ctdev update` on Linux.
func OrcaUpdate(ctx context.Context, o sysutil.Opts, ver string) error {
	return installOrcaDeb(ctx, o, platform.Detect().Arch, ver)
}
