package component

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// goInstallDir is where the official tarball lands on Linux. The matching
// PATH entry is added by configs/zsh/path.zsh.
const goInstallDir = "/usr/local/go"

type goFile struct {
	Filename string `json:"filename"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	SHA256   string `json:"sha256"`
	Kind     string `json:"kind"`
}

type goRelease struct {
	Version string   `json:"version"`
	Stable  bool     `json:"stable"`
	Files   []goFile `json:"files"`
}

func goInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && goInstalled() {
		fmt.Fprintln(opts.Stdout, "Go already installed")
		return nil
	}

	// macOS: let Homebrew manage Go (it already handles PATH + upgrades).
	if p.PackageManager == "brew" {
		fmt.Fprintln(opts.Stdout, "Installing Go via Homebrew...")
		return sysutil.Run(ctx, o, "brew", "install", "go")
	}

	// Linux: install the official tarball — apt's Go lags well behind upstream.
	ver, file, err := latestGoArchive(ctx, "linux", p.Arch)
	if err != nil {
		return err
	}
	fmt.Fprintf(opts.Stdout, "Installing Go %s (official tarball)...\n", ver)

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download https://go.dev/dl/%s, verify sha256, extract to %s\n", file.Filename, goInstallDir)
		return nil
	}

	tmp, err := os.CreateTemp("", "go-*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	if err := sysutil.DownloadFile(ctx, "https://go.dev/dl/"+file.Filename, tmp.Name()); err != nil {
		return fmt.Errorf("download go: %w", err)
	}
	if err := sysutil.VerifyChecksum(tmp.Name(), file.SHA256); err != nil {
		return fmt.Errorf("verify go tarball: %w", err)
	}

	// Replace any existing install so versions don't accumulate under /usr/local/go.
	if err := sysutil.SudoRun(ctx, o, "rm", "-rf", goInstallDir); err != nil {
		return fmt.Errorf("remove old go: %w", err)
	}
	if err := sysutil.SudoRun(ctx, o, "tar", "-C", "/usr/local", "-xzf", tmp.Name()); err != nil {
		return fmt.Errorf("extract go: %w", err)
	}

	fmt.Fprintf(opts.Stdout, "Go %s installed to %s (PATH wired via path.zsh)\n", ver, goInstallDir)
	return nil
}

// goInstalled reports whether the tarball install exists or `go` is on PATH.
func goInstalled() bool {
	if _, err := os.Stat(filepath.Join(goInstallDir, "bin", "go")); err == nil {
		return true
	}
	return sysutil.CommandExists("go")
}

// latestGoArchive returns the newest stable Go version and the archive file
// matching the given OS/arch, queried from the go.dev download manifest.
func latestGoArchive(ctx context.Context, goos, arch string) (string, goFile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://go.dev/dl/?mode=json", nil)
	if err != nil {
		return "", goFile{}, err
	}
	resp, err := sysutil.HTTPClient().Do(req)
	if err != nil {
		return "", goFile{}, fmt.Errorf("fetch go releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", goFile{}, fmt.Errorf("fetch go releases: HTTP %d", resp.StatusCode)
	}

	var releases []goRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", goFile{}, fmt.Errorf("parse go releases: %w", err)
	}
	for _, r := range releases {
		if !r.Stable {
			continue
		}
		for _, f := range r.Files {
			if f.OS == goos && f.Arch == arch && f.Kind == "archive" {
				return r.Version, f, nil
			}
		}
	}
	return "", goFile{}, fmt.Errorf("no stable go archive for %s/%s", goos, arch)
}

func goUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Go...")

	if p.PackageManager == "brew" {
		return sysutil.Run(ctx, o, "brew", "uninstall", "go")
	}
	return sysutil.SudoRun(ctx, o, "rm", "-rf", goInstallDir)
}
