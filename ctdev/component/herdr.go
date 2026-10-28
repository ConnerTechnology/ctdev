package component

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// herdrFormula is the Homebrew core formula; it installs the `herdr` binary.
const herdrFormula = "herdr"

// herdrManifestURL is where herdr publishes the SHA-256 of every release asset,
// the same manifest its own installer and `herdr update` read. Its GitHub
// releases carry the raw binaries and no checksum file.
const herdrManifestURL = "https://herdr.dev/latest.json"

// herdrLinuxTarget maps Go arch names to the target names in herdr's asset
// names and manifest keys. herdr publishes Linux builds for these two only.
var herdrLinuxTarget = map[string]string{
	"amd64": "linux-x86_64",
	"arm64": "linux-aarch64",
}

func herdrTarget(arch string) (string, error) {
	target, ok := herdrLinuxTarget[arch]
	if !ok {
		return "", fmt.Errorf("herdr Linux build only available for amd64/arm64 (got %s): %w", arch, ErrUnsupportedOS)
	}
	return target, nil
}

// herdrChecksum returns the manifest's SHA-256 for target in release ver. It
// reads the entry for the version being downloaded rather than the manifest's
// top-level latest, so a GitHub release that is ahead of the manifest fails
// here instead of being checked against the wrong hash. Anything missing is an
// error; the download is never installed unchecked.
func herdrChecksum(ctx context.Context, manifestURL, ver, target string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := sysutil.HTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch herdr manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch herdr manifest: HTTP %d", resp.StatusCode)
	}

	var manifest struct {
		Releases map[string]struct {
			SHA256 map[string]string `json:"sha256"`
		} `json:"releases"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return "", fmt.Errorf("parse herdr manifest: %w", err)
	}
	sum := strings.TrimSpace(manifest.Releases[ver].SHA256[target])
	if sum == "" {
		return "", fmt.Errorf("herdr manifest has no %s checksum for v%s", target, ver)
	}
	return sum, nil
}

func herdrInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && alreadyInstalled("herdr") {
		fmt.Fprintln(opts.Stdout, "herdr already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing herdr...")

	if p.OS == platform.MacOS {
		return sysutil.InstallPackage(ctx, o, herdrFormula)
	}

	target, err := herdrTarget(p.Arch)
	if err != nil {
		return err
	}

	// No APT repo: Linux gets the raw binary from the latest GitHub release,
	// checked against the SHA-256 in herdr's manifest. It goes to
	// /usr/local/bin like the other release binaries, so it is on PATH for
	// non-interactive SSH, which `herdr --remote` runs through.
	ver, err := sysutil.DownloadGitHubBinary(ctx, o, sysutil.GitHubBinarySpec{
		Repo: "herdrdev/herdr",
		ArchiveURL: func(ver, goos, goarch string) string {
			return fmt.Sprintf("https://github.com/herdrdev/herdr/releases/download/v%s/herdr-%s", ver, target)
		},
		Checksum: func(ctx context.Context, ver, goos, goarch string) (string, error) {
			return herdrChecksum(ctx, herdrManifestURL, ver, target)
		},
		InstallDest: "/usr/local/bin/herdr",
	})
	if err != nil {
		return err
	}
	if !o.DryRun {
		fmt.Fprintf(opts.Stdout, "herdr %s installed\n", ver)
	}
	return nil
}

// herdrUninstall removes the binary only. ~/.config/herdr holds the user's
// settings and the session state lives beside it, so a reinstall keeps both.
func herdrUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing herdr...")

	if p.OS == platform.MacOS {
		return sysutil.RemovePackage(ctx, o, herdrFormula)
	}
	return sysutil.SudoRun(ctx, o, "rm", "-f", "/usr/local/bin/herdr")
}
