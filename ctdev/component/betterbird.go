package component

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	if err := betterbirdCheckArch(p.Arch); err != nil {
		return err
	}

	fmt.Fprintln(opts.Stdout, "Installing Betterbird...")

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download betterbird tarball, verify sha256, extract to %s, write %s\n", betterbirdInstallDir, betterbirdDesktop)
		return nil
	}

	// Replacing the tree under a running instance crashes it mid-session.
	running, err := betterbirdRunning(ctx)
	if err != nil {
		return err
	}
	if running {
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

	// Staging sits next to the install dir so the swap is a rename on one
	// filesystem; tmpDir's random suffix keeps the name unique.
	staging := filepath.Join(filepath.Dir(betterbirdInstallDir), ".ctdev-"+filepath.Base(tmpDir))
	if err := betterbirdReplace(ctx, o, tarPath, staging, betterbirdInstallDir); err != nil {
		return err
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

// betterbirdCheckArch rejects every arch but amd64, the only one upstream
// ships a Linux tarball for. It wraps ErrUnsupportedOS so the executor reports
// Skipped rather than Failed.
func betterbirdCheckArch(arch string) error {
	if arch != "amd64" {
		return fmt.Errorf("betterbird tarball only available on amd64 (got %s): %w", arch, ErrUnsupportedOS)
	}
	return nil
}

// betterbirdRunning asks pgrep whether a Betterbird process is alive.
func betterbirdRunning(ctx context.Context) (bool, error) {
	return pgrepMatched(sysutil.Run(ctx, sysutil.Opts{Stdout: io.Discard}, "pgrep", "-f", "betterbird-bin"))
}

// pgrepMatched maps pgrep's result: exit 0 (nil) is a match and exit 1 is no
// match. Exit 2/3 or a missing pgrep says nothing about the process, so those
// are errors rather than a green light.
func pgrepMatched(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("check whether betterbird is running: %w", err)
}

// betterbirdReplace swaps a new release into installDir without ever leaving
// the machine with no Betterbird: it extracts into staging, checks the result,
// moves the old tree into staging, moves the new tree into place, and only
// then removes staging (and the old tree with it). A failure before the swap
// leaves installDir untouched; a failure during it puts the old tree back.
func betterbirdReplace(ctx context.Context, o sysutil.Opts, tarPath, staging, installDir string) error {
	if err := sysutil.SudoRun(ctx, o, "mkdir", staging); err != nil {
		return fmt.Errorf("create betterbird staging dir: %w", err)
	}
	// WithoutCancel: after a Ctrl-C the staging dir still has to go.
	keepStaging := false
	defer func() {
		if keepStaging {
			return
		}
		if err := sysutil.SudoRun(context.WithoutCancel(ctx), o, "rm", "-rf", staging); err != nil {
			fmt.Fprintf(o.Stdout, "warning: could not remove %s: %v; remove it by hand\n", staging, err)
		}
	}()

	if err := sysutil.SudoRun(ctx, o, "tar", "-C", staging, "-xJf", tarPath); err != nil {
		return fmt.Errorf("extract betterbird: %w", err)
	}
	newTree := filepath.Join(staging, "betterbird")
	if err := betterbirdCheckTree(newTree); err != nil {
		return err
	}

	oldTree := filepath.Join(staging, "old")
	_, statErr := os.Lstat(installDir)
	hadOld := statErr == nil
	if hadOld {
		if err := sysutil.SudoRun(ctx, o, "mv", "-T", installDir, oldTree); err != nil {
			// The rename may have landed before the error (a Ctrl-C right
			// after it), so look at the disk rather than trust the error.
			switch betterbirdAfterFailedAside(pathMayExist(installDir), pathMayExist(oldTree)) {
			case asideRestore:
				if rerr := sysutil.SudoRun(context.WithoutCancel(ctx), o, "mv", "-T", oldTree, installDir); rerr != nil {
					keepStaging = true
					return fmt.Errorf("move old betterbird aside: %w (putting it back also failed: %v; it is in %s)", err, rerr, oldTree)
				}
			case asideKeep:
				keepStaging = true
				return fmt.Errorf("move old betterbird aside: %w (both %s and %s exist; left both in place)", err, installDir, oldTree)
			}
			return fmt.Errorf("move old betterbird aside: %w", err)
		}
	}
	if err := sysutil.SudoRun(ctx, o, "mv", "-T", newTree, installDir); err != nil {
		if hadOld {
			if rerr := sysutil.SudoRun(context.WithoutCancel(ctx), o, "mv", "-T", oldTree, installDir); rerr != nil {
				// Staging now holds the only copy of the old install: keep it.
				keepStaging = true
				return fmt.Errorf("move new betterbird into place: %w (restoring the old one also failed: %v; it is in %s)", err, rerr, oldTree)
			}
		}
		return fmt.Errorf("move new betterbird into place: %w", err)
	}
	return nil
}

// asideRecovery is what to do after moving the old install aside reported an
// error.
type asideRecovery int

const (
	asideClean   asideRecovery = iota // staging holds no old tree: removing it is safe
	asideRestore                      // the old tree moved: put it back
	asideKeep                         // unexpected state: remove nothing
)

// betterbirdAfterFailedAside decides, from what is on disk, whether a failed
// move-aside actually moved the old install. Staging is only safe to remove
// when it holds no old tree.
func betterbirdAfterFailedAside(installExists, oldTreeExists bool) asideRecovery {
	switch {
	case !oldTreeExists:
		return asideClean
	case !installExists:
		return asideRestore
	default:
		return asideKeep
	}
}

// pathMayExist reports whether p exists, counting any Lstat error other than
// "not exist" as present so a doubt never leads to deleting it.
func pathMayExist(p string) bool {
	_, err := os.Lstat(p)
	return err == nil || !errors.Is(err, fs.ErrNotExist)
}

// betterbirdCheckTree confirms an extracted tarball holds the binary the
// desktop entry launches, so a truncated or reshaped tarball never replaces a
// working install.
func betterbirdCheckTree(tree string) error {
	bin := filepath.Join(tree, "betterbird")
	info, err := os.Stat(bin)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("extracted betterbird tarball has no %s; keeping the existing install", bin)
	}
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
