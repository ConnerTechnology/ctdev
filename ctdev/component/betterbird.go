package component

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// Betterbird ships for Linux only as an upstream tarball (x86_64), so this
// follows upstream's own install-on-linux/install-betterbird.sh: ask
// getloc.php for the current release, verify it against sha256-<major>.txt,
// replace /opt/betterbird, and register a desktop entry. The tarball has no
// updater; `ctdev install betterbird --force` pulls the latest release.
const (
	betterbirdInstallDir = "/opt/betterbird"
	betterbirdDownloads  = "https://www.betterbird.eu/downloads/"
	betterbirdLocURL     = betterbirdDownloads + "getloc.php?os=linux&lang=en-US&version=release"
	// /usr/share rather than /usr/local/share: the latter's applications/ is
	// often absent, and a desktop session only watches directories that existed
	// at login, so an entry there stays out of the menu until the next login.
	betterbirdDesktop = "/usr/share/applications/eu.betterbird.Betterbird.desktop"
)

func betterbirdInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && alreadyInstalled("betterbird") {
		fmt.Fprintln(opts.Stdout, "Betterbird already installed")
		return nil
	}
	if p.OS != platform.Linux {
		return unsupportedPMError("betterbird", p.PackageManager)
	}
	if p.Arch != "amd64" {
		return fmt.Errorf("betterbird tarball only available on amd64 (got %s)", p.Arch)
	}

	fmt.Fprintln(opts.Stdout, "Installing Betterbird...")

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download betterbird tarball, verify sha256, extract to %s, write %s\n", betterbirdInstallDir, betterbirdDesktop)
		return nil
	}

	// Replacing the tree under a running instance crashes it mid-session.
	if exec.CommandContext(ctx, "pgrep", "-f", "betterbird-bin").Run() == nil {
		return fmt.Errorf("betterbird is running; close it and re-run")
	}

	tarURL, err := betterbirdLatestURL(ctx)
	if err != nil {
		return err
	}
	file := path.Base(tarURL)
	major, err := betterbirdMajor(file)
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "betterbird-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	// The tarball keeps its upstream name so VerifyChecksumFile can find its line.
	tarPath := filepath.Join(tmpDir, file)
	sumsPath := filepath.Join(tmpDir, "sha256.txt")
	if err := sysutil.DownloadFile(ctx, tarURL, tarPath); err != nil {
		return fmt.Errorf("download betterbird: %w", err)
	}
	if err := sysutil.DownloadFile(ctx, betterbirdDownloads+"sha256-"+major+".txt", sumsPath); err != nil {
		return fmt.Errorf("download betterbird checksums: %w", err)
	}
	if err := sysutil.VerifyChecksumFile(tarPath, sumsPath); err != nil {
		return fmt.Errorf("verify betterbird tarball: %w", err)
	}

	if err := sysutil.SudoRun(ctx, o, "rm", "-rf", betterbirdInstallDir); err != nil {
		return fmt.Errorf("remove old betterbird: %w", err)
	}
	if err := sysutil.SudoRun(ctx, o, "tar", "-C", filepath.Dir(betterbirdInstallDir), "-xJf", tarPath); err != nil {
		return fmt.Errorf("extract betterbird: %w", err)
	}

	desktop, err := Configs.ReadFile("configs/betterbird/eu.betterbird.Betterbird.desktop")
	if err != nil {
		return err
	}
	if err := sysutil.SudoWriteFileMode(ctx, o, string(desktop), betterbirdDesktop, "0644"); err != nil {
		return fmt.Errorf("write betterbird desktop entry: %w", err)
	}

	fmt.Fprintf(opts.Stdout, "Betterbird installed to %s from %s\n", betterbirdInstallDir, file)
	return nil
}

// betterbirdLatestURL asks upstream which tarball is the current release.
// getloc.php answers with the bare URL that get.php would redirect to.
func betterbirdLatestURL(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, betterbirdLocURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := sysutil.HTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch betterbird release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch betterbird release: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", fmt.Errorf("fetch betterbird release: %w", err)
	}
	u := strings.TrimSpace(string(body))
	if !strings.HasPrefix(u, betterbirdDownloads) || !strings.HasSuffix(u, ".linux-x86_64.tar.xz") {
		return "", fmt.Errorf("unexpected betterbird release location %q", u)
	}
	return u, nil
}

// betterbirdMajor extracts the major version that names upstream's checksum
// file: "betterbird-153.4.0esr-bb10.en-US.linux-x86_64.tar.xz" -> "153".
func betterbirdMajor(file string) (string, error) {
	rest, ok := strings.CutPrefix(file, "betterbird-")
	if !ok {
		return "", fmt.Errorf("unexpected betterbird tarball name %q", file)
	}
	major, _, ok := strings.Cut(rest, ".")
	if !ok || major == "" || strings.Trim(major, "0123456789") != "" {
		return "", fmt.Errorf("unexpected betterbird tarball name %q", file)
	}
	return major, nil
}

func betterbirdUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	if p.OS != platform.Linux {
		return ErrUnsupportedOS
	}
	fmt.Fprintln(opts.Stdout, "Removing Betterbird...")
	if err := sysutil.SudoRun(ctx, o, "rm", "-f", betterbirdDesktop); err != nil {
		return err
	}
	// The mail profile lives in ~/.thunderbird (shared with Thunderbird) and is
	// left alone.
	return sysutil.SudoRun(ctx, o, "rm", "-rf", betterbirdInstallDir)
}
