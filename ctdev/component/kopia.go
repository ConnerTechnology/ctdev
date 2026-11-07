package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// kopia installs one pinned Kopia binary and nothing else: no config, no
// repository, no timer. Whatever uses it brings its own repository and
// schedule. A node's boot runs this unattended, as root, so the version is
// pinned rather than "latest": a new Kopia reaches a node only through a
// ctdev release (CTD-143).
const kopiaVersion = "0.23.1"

// kopiaBin is where the binary goes; a var so tests can point it elsewhere.
var kopiaBin = "/usr/local/bin/kopia"

// kopiaPin is one architecture's pinned release asset.
type kopiaPin struct {
	asset     string // the tarball's name, without .tar.gz; Kopia calls amd64 "x64"
	tarSHA256 string // of kopia-<version>-linux-<arch>.tar.gz
	binSHA256 string // of the kopia binary inside it
}

// The tarball hashes match the release's checksums.txt, and each binary is
// byte-identical to /bin/kopia in that platform's kopia/kopia:0.23.1 image
// (checked 2026-11-06). Recording them here means a tampered release asset
// or a rewritten checksums.txt fails the install instead of running as root.
// Update all four together with kopiaVersion.
var kopiaPins = map[string]kopiaPin{
	"amd64": {
		asset:     "kopia-" + kopiaVersion + "-linux-x64",
		tarSHA256: "416d0f84a3dbb321a8b2d8f0997b1a0a6e915babe79ee76fa6e4d2bd1e1c5178",
		binSHA256: "e306c3d48b47756912928738241baffe68d1a8a16ab804ab3fbabd725a61df7f",
	},
	"arm64": {
		asset:     "kopia-" + kopiaVersion + "-linux-arm64",
		tarSHA256: "a4ffbc019e0b0f932e2632054e73ec521dc1e80172a00095369c53ecf4e5a6cb",
		binSHA256: "7986754bdc38d8bbb1d03368c22f7a499171f3f0bec86b834a8153fb1b58ab06",
	},
}

func kopiaPinFor(arch string) (kopiaPin, error) {
	pin, ok := kopiaPins[arch]
	if !ok {
		return kopiaPin{}, fmt.Errorf("kopia: no pinned linux/%s build: %w", arch, ErrUnsupportedOS)
	}
	return pin, nil
}

// kopiaPinnedInstalled is the Detect hook. Only a binary that hashes to the
// pin counts, so a re-run replaces any other version without --force (and
// asks for root to do it). It hashes the file rather than running it, as
// install.sh does for ctdev: as root, whatever sits at that path could be
// anything.
func kopiaPinnedInstalled() bool {
	pin, err := kopiaPinFor(platform.Detect().Arch)
	if err != nil {
		return false
	}
	return sysutil.VerifyChecksum(kopiaBin, pin.binSHA256) == nil
}

func kopiaInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && alreadyInstalled("kopia") {
		fmt.Fprintf(opts.Stdout, "kopia %s already installed\n", kopiaVersion)
		return nil
	}

	pin, err := kopiaPinFor(p.Arch)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://github.com/kopia/kopia/releases/download/v%s/%s.tar.gz", kopiaVersion, pin.asset)

	fmt.Fprintf(opts.Stdout, "Installing kopia %s...\n", kopiaVersion)
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download %s, verify sha256 %s, install to %s\n", url, pin.tarSHA256, kopiaBin)
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "kopia-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archive := filepath.Join(tmpDir, pin.asset+".tar.gz")
	if err := sysutil.DownloadFile(ctx, url, archive); err != nil {
		return err
	}
	if err := sysutil.VerifyChecksum(archive, pin.tarSHA256); err != nil {
		return fmt.Errorf("kopia %s: %w", pin.asset, err)
	}
	if err := sysutil.Run(ctx, o, "tar", "-xzf", archive, "-C", tmpDir); err != nil {
		return err
	}
	bin := filepath.Join(tmpDir, pin.asset, "kopia")
	// The tarball checked out, but Detect trusts the binary's own hash, so
	// check that too: a mismatch here would reinstall on every boot.
	if err := sysutil.VerifyChecksum(bin, pin.binSHA256); err != nil {
		return fmt.Errorf("kopia binary in %s: %w", pin.asset, err)
	}
	if err := sysutil.InstallBinary(ctx, o, bin, kopiaBin); err != nil {
		return err
	}
	fmt.Fprintf(opts.Stdout, "kopia %s installed\n", kopiaVersion)
	return nil
}

// kopiaUninstall removes only the binary. Repositories, configs and caches
// belong to whatever used Kopia, and nothing here created them.
func kopiaUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing kopia...")
	return sysutil.SudoRun(ctx, o, "rm", "-f", kopiaBin)
}
