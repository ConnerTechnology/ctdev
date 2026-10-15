package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// endorctlFormula is the tap-qualified Homebrew formula, so `brew install`
// taps endorlabs/tap on its own.
const endorctlFormula = "endorlabs/tap/endorctl"

func endorctlInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && alreadyInstalled("endorctl") {
		fmt.Fprintln(opts.Stdout, "endorctl already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing endorctl...")

	if p.OS == platform.MacOS {
		return sysutil.InstallPackage(ctx, o, endorctlFormula)
	}

	// Endor Labs publishes no APT repo: Linux gets the latest binary, checked
	// against the SHA-256 they serve alongside it.
	asset := fmt.Sprintf("endorctl_linux_%s", p.Arch)
	dlURL := "https://api.endorlabs.com/download/latest/" + asset
	csURL := "https://api.endorlabs.com/sha/latest/" + asset

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download %s, verify SHA-256, install to /usr/local/bin/endorctl\n", dlURL)
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "endorctl-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	binaryPath := filepath.Join(tmpDir, "endorctl")
	checksumPath := filepath.Join(tmpDir, "endorctl.sha256")

	if err := sysutil.DownloadFile(ctx, dlURL, binaryPath); err != nil {
		return fmt.Errorf("download endorctl: %w", err)
	}
	if err := sysutil.DownloadFile(ctx, csURL, checksumPath); err != nil {
		return fmt.Errorf("download endorctl checksum: %w", err)
	}

	// The sha endpoint returns just the hash, not "hash  filename".
	expected, err := os.ReadFile(checksumPath)
	if err != nil {
		return err
	}
	if err := sysutil.VerifyChecksum(binaryPath, strings.TrimSpace(string(expected))); err != nil {
		return err
	}

	return sysutil.InstallBinary(ctx, o, binaryPath, "/usr/local/bin/endorctl")
}

func endorctlUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing endorctl...")

	if p.OS == platform.MacOS {
		return sysutil.RemovePackage(ctx, o, endorctlFormula)
	}
	return sysutil.SudoRun(ctx, o, "rm", "-f", "/usr/local/bin/endorctl")
}
