package component

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// T3 Code (https://t3.codes) ships a `t3` CLI and an Electron desktop app from
// the same GitHub release. Its release workflow marks only plain stable
// releases as latest, so /releases/latest is the stable channel.
const (
	T3CodeRepo        = "pingdotgg/t3code"
	t3codeInstallURL  = "https://t3.codes/install.sh"
	t3codeDocsURL     = "https://github.com/pingdotgg/t3code/blob/main/docs/user/install.md"
	t3codeCask        = "t3-code"
	t3codeDebPackage  = "t3code"
	t3codeReleaseBase = "https://github.com/pingdotgg/t3code/releases/download"
)

// The cask installs "T3 Code (Beta).app"; the plain name is checked too so
// detection survives the app leaving beta.
var t3codeMacApps = []string{"/Applications/T3 Code (Beta).app", "/Applications/T3 Code.app"}

// t3codeLauncher is where install.sh links `t3` (T3CODE_INSTALL_BIN_DIR's
// default). Detection and every command go through this path rather than
// whatever `t3` is first on PATH: the name is short enough that an unrelated
// tool could own it, and `t3 uninstall --yes` must never reach that one.
func t3codeLauncher() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin", "t3")
}

func t3codeInstalled() bool {
	_, err := os.Stat(t3codeLauncher())
	return err == nil
}

// t3codeNextSteps is printed after the CLI installs. ctdev doesn't set up the
// background service itself: `t3 up` asks how other devices reach the machine
// and signs in, which needs a person.
const t3codeNextSteps = `T3 Code is installed. Next steps:
  - Getting started: ` + t3codeDocsURL + `
  - Run ` + "`t3`" + ` to start T3 Code and open it in your browser.
  - To use it from your iPhone, iPad or another computer, run ` + "`t3 up`" + `
    (or ` + "`t3 service install`" + ` to only keep it running in the background).`

func t3codeInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if p.OS == platform.MacOS && p.Arch != "arm64" {
		return fmt.Errorf("the t3 CLI has no Intel Mac build: %w", ErrUnsupportedOS)
	}
	if !opts.Force && alreadyInstalled("t3code") {
		fmt.Fprintln(opts.Stdout, "T3 Code CLI already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing T3 Code CLI...")
	// install.sh checks the archive against the release's SHA256SUMS and lays
	// it out under ~/.t3/runtime, the layout `t3 service install` and
	// `t3 update` reuse, so a download of our own would gain nothing.
	if err := sysutil.Run(ctx, o, "bash", "-c", "curl -fsSL "+t3codeInstallURL+" | sh"); err != nil {
		return fmt.Errorf("t3 installer: %w", err)
	}
	fmt.Fprintln(opts.Stdout, t3codeNextSteps)
	return nil
}

// t3codeUninstall uses the CLI's own uninstall: it removes the background
// service, the launcher and every downloaded version, and keeps the user's
// projects and threads.
func t3codeUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing T3 Code CLI...")
	if !t3codeInstalled() {
		return nil
	}
	return sysutil.Run(ctx, o, t3codeLauncher(), "uninstall", "--yes")
}

// T3CodeVersion returns the installed CLI's version ("0.1.7"), or "" when it
// isn't installed or doesn't answer.
func T3CodeVersion(ctx context.Context) string {
	if !t3codeInstalled() {
		return ""
	}
	out, err := exec.CommandContext(ctx, t3codeLauncher(), "--version").Output()
	if err != nil {
		return ""
	}
	return parseT3CodeVersion(string(out))
}

// parseT3CodeVersion reads `t3 --version` output, "t3 v0.1.7".
func parseT3CodeVersion(out string) string {
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimPrefix(fields[len(fields)-1], "v")
}

// T3CodeUpdate moves the CLI to ver with `t3 update`, which also restarts the
// background service when one is installed (--yes lets it do so without a
// prompt). ver is pinned so the update installs the release ctdev listed.
func T3CodeUpdate(ctx context.Context, o sysutil.Opts, ver string) error {
	return sysutil.Run(ctx, o, t3codeLauncher(), "update", "--yes", ver)
}

func t3codeDesktopInstalled() bool {
	switch runtime.GOOS {
	case "darwin":
		for _, app := range t3codeMacApps {
			if _, err := os.Stat(app); err == nil {
				return true
			}
		}
		return false
	case "linux":
		// The .deb puts nothing on PATH and installs under "/opt/T3 Code (Beta)",
		// so the package database is the stable signal.
		return sysutil.DebInstalled(context.Background(), t3codeDebPackage)
	}
	return false
}

// T3CodeDesktopVersion returns the installed Linux desktop package's
// version, or "" when the package isn't installed. The macOS app updates
// itself, so it has no counterpart.
func T3CodeDesktopVersion(ctx context.Context) string {
	return sysutil.DebVersion(ctx, t3codeDebPackage)
}

// t3codeDebAsset names the release's .deb and the electron-builder manifest
// that carries its sha512, for a Debian architecture.
func t3codeDebAsset(arch, ver string) (deb, manifest string, err error) {
	switch arch {
	case "amd64":
		return fmt.Sprintf("T3-Code-%s-amd64.deb", ver), "latest-linux.yml", nil
	case "arm64":
		return fmt.Sprintf("T3-Code-%s-arm64.deb", ver), "latest-linux-arm64.yml", nil
	}
	return "", "", fmt.Errorf("T3 Code desktop only available for amd64/arm64 (got %s): %w", arch, ErrUnsupportedOS)
}

func t3codeDesktopInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	switch p.OS {
	case platform.MacOS:
		if p.Arch != "arm64" {
			return fmt.Errorf("the T3 Code cask is Apple Silicon only: %w", ErrUnsupportedOS)
		}
	case platform.Linux:
		if p.PackageManager != "apt" {
			return unsupportedPMError("T3 Code desktop", p.PackageManager)
		}
	default:
		return ErrUnsupportedOS
	}

	if !opts.Force && alreadyInstalled("t3code-desktop") {
		fmt.Fprintln(opts.Stdout, "T3 Code desktop already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing T3 Code desktop...")

	if p.OS == platform.MacOS {
		// The cask is marked auto_updates: the app updates itself.
		return sysutil.BrewCaskInstallVerified(ctx, o, t3codeCask, t3codeDesktopInstalled)
	}
	ver, err := sysutil.GitHubLatestVersion(ctx, T3CodeRepo)
	if err != nil {
		return err
	}
	return installT3CodeDeb(ctx, o, p.Arch, ver)
}

// installT3CodeDeb installs release ver's .deb. It is also how `ctdev update`
// moves the Linux app forward, so it always installs, even over itself.
func installT3CodeDeb(ctx context.Context, o sysutil.Opts, arch, ver string) error {
	deb, manifest, err := t3codeDebAsset(arch, ver)
	if err != nil {
		return err
	}
	base := fmt.Sprintf("%s/v%s/", t3codeReleaseBase, ver)
	return installElectronDeb(ctx, o, base, deb, manifest, t3codeDebPackage)
}

func t3codeDesktopUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing T3 Code desktop...")

	switch p.OS {
	case platform.MacOS:
		return sysutil.BrewCaskRemove(ctx, o, t3codeCask)
	case platform.Linux:
		if p.PackageManager != "apt" {
			return unsupportedPMError("T3 Code desktop", p.PackageManager)
		}
		return sysutil.RemovePackage(ctx, o, t3codeDebPackage)
	}
	return ErrUnsupportedOS
}

// T3CodeDesktopUpdate installs release ver's .deb over the installed one,
// for `ctdev update` on Linux.
func T3CodeDesktopUpdate(ctx context.Context, o sysutil.Opts, ver string) error {
	return installT3CodeDeb(ctx, o, platform.Detect().Arch, ver)
}
