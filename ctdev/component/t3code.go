package component

import (
	"bufio"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
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
// default). It isn't on PATH until the shell config adds ~/.local/bin, so
// detection checks the file as well as PATH.
func t3codeLauncher() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin", "t3")
}

func t3codeCommand() string {
	if sysutil.CommandExists("t3") {
		return "t3"
	}
	return t3codeLauncher()
}

func t3codeInstalled() bool {
	if sysutil.CommandExists("t3") {
		return true
	}
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
	return sysutil.Run(ctx, o, t3codeCommand(), "uninstall", "--yes")
}

// T3CodeVersion returns the installed CLI's version ("0.1.7"), or "" when it
// isn't installed or doesn't answer.
func T3CodeVersion(ctx context.Context) string {
	if !t3codeInstalled() {
		return ""
	}
	out, err := exec.CommandContext(ctx, t3codeCommand(), "--version").Output()
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

// T3CodeUpdate moves the CLI to the newest stable release with `t3 update`,
// which also restarts the background service when one is installed (--yes
// lets it do so without a prompt).
func T3CodeUpdate(ctx context.Context, o sysutil.Opts) error {
	return sysutil.Run(ctx, o, t3codeCommand(), "update", "--yes")
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
		return exec.Command("dpkg", "-s", t3codeDebPackage).Run() == nil
	}
	return false
}

// T3CodeDesktopDebVersion returns the installed Linux desktop package's
// version, or "" when the package isn't installed. The macOS app updates
// itself, so it has no counterpart.
func T3CodeDesktopDebVersion(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "dpkg-query", "-W", "-f=${Version}", t3codeDebPackage).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
	return installT3CodeDeb(ctx, o, p.Arch)
}

// installT3CodeDeb installs the latest stable release's .deb, checked against
// the sha512 electron-builder publishes for it. It is also how `ctdev update`
// moves the Linux app forward, so it always installs, even over itself.
func installT3CodeDeb(ctx context.Context, o sysutil.Opts, arch string) error {
	ver, err := sysutil.GitHubLatestVersion(ctx, T3CodeRepo)
	if err != nil {
		return err
	}
	deb, manifest, err := t3codeDebAsset(arch, ver)
	if err != nil {
		return err
	}
	base := fmt.Sprintf("%s/v%s/", t3codeReleaseBase, ver)
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download %s, verify it against %s, and install it\n", base+deb, manifest)
		return nil
	}

	want, err := t3codeManifestSHA512(ctx, base+manifest, deb)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "t3code-*.deb")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	tmp.Close()
	if err := sysutil.DownloadFile(ctx, base+deb, tmp.Name()); err != nil {
		return fmt.Errorf("download %s: %w", deb, err)
	}
	if err := verifySHA512Base64(tmp.Name(), want); err != nil {
		return fmt.Errorf("%s: %w", deb, err)
	}
	return installDebWithDepFix(ctx, o, tmp.Name(), t3codeDebPackage)
}

// t3codeManifestSHA512 fetches an electron-builder latest-*.yml and returns
// the sha512 of the named file.
func t3codeManifestSHA512(ctx context.Context, url, file string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := sysutil.HTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
	}
	return parseElectronManifestSHA512(resp.Body, file)
}

// parseElectronManifestSHA512 reads the files list of an electron-builder
// update manifest:
//
//	files:
//	  - url: T3-Code-0.1.7-amd64.deb
//	    sha512: <base64>
//
// It fails closed: a file that isn't listed, or has no hash, is an error.
func parseElectronManifestSHA512(r io.Reader, file string) (string, error) {
	sc := bufio.NewScanner(r)
	inEntry := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if v, ok := strings.CutPrefix(line, "- url:"); ok {
			inEntry = strings.TrimSpace(v) == file
			continue
		}
		if !inEntry {
			continue
		}
		if v, ok := strings.CutPrefix(line, "sha512:"); ok {
			if sum := strings.TrimSpace(v); sum != "" {
				return sum, nil
			}
			break
		}
		if strings.HasPrefix(line, "- ") || !strings.HasPrefix(sc.Text(), " ") {
			inEntry = false // the next entry, or a top-level key
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("update manifest has no sha512 for %s", file)
}

func verifySHA512Base64(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := base64.StdEncoding.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("sha512 mismatch: got %s, want %s", got, want)
	}
	return nil
}

func t3codeDesktopUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing T3 Code desktop...")

	switch p.OS {
	case platform.MacOS:
		return sysutil.BrewCaskRemove(ctx, o, t3codeCask)
	case platform.Linux:
		if p.PackageManager == "apt" {
			return sysutil.RemovePackage(ctx, o, t3codeDebPackage)
		}
	}
	return ErrUnsupportedOS
}

// UpdateT3CodeDesktopDeb installs the latest stable .deb over the installed
// one, for `ctdev update`.
func UpdateT3CodeDesktopDeb(ctx context.Context, o sysutil.Opts) error {
	return installT3CodeDeb(ctx, o, platform.Detect().Arch)
}
