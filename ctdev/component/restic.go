package component

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
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

// resticInstalled is the registry's Detect. On Linux only ctdev's own binary
// counts: apt's restic on PATH is too old, and counting it would make install
// say "already installed" and never put 0.19.1 in place. On macOS Homebrew's
// restic is current, so any restic on PATH counts.
func resticInstalled() bool {
	if platform.Detect().OS != platform.Linux {
		return sysutil.CommandExists("restic")
	}
	ours, err := resticUserBin()
	if err != nil {
		return false
	}
	_, err = os.Stat(ours)
	return err == nil
}

// otherResticOnPath returns the first restic on PATH that isn't ours, or "".
func otherResticOnPath(ours string) string {
	oursReal, err := filepath.EvalSymlinks(ours)
	if err != nil {
		oursReal = filepath.Clean(ours)
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		cand := filepath.Join(dir, "restic")
		fi, err := os.Stat(cand)
		if err != nil || fi.IsDir() || fi.Mode().Perm()&0o111 == 0 {
			continue
		}
		resolved, err := filepath.EvalSymlinks(cand)
		if err != nil {
			resolved = filepath.Clean(cand)
		}
		if resolved != oursReal {
			return cand
		}
	}
	return ""
}

// hintOtherRestic prints one line when another restic (usually apt's) is on
// PATH beside ours. It never removes it: that needs sudo, and it's the user's.
func hintOtherRestic(w io.Writer, ours string) {
	if other := otherResticOnPath(ours); other != "" {
		fmt.Fprintf(w, "Another restic is at %s; ctdev uses %s. Remove apt's with: sudo apt remove restic\n", other, ours)
	}
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
	hintOtherRestic(opts.Stdout, dest)
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
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] rm -f %s\n", dest)
	} else if err := os.Remove(dest); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	hintOtherRestic(opts.Stdout, dest)
	return nil
}
