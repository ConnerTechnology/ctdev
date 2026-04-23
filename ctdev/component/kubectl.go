package component

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func kubectlInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && sysutil.CommandExists("kubectl") {
		fmt.Fprintln(opts.Stdout, "kubectl already installed")
		return nil
	}

	if p.OS == platform.MacOS {
		fmt.Fprintln(opts.Stdout, "Installing kubectl...")
		return sysutil.InstallPackage(ctx, o, "kubectl")
	}

	// Fetch latest stable version
	ver, err := fetchKubectlStableVersion()
	if err != nil {
		return err
	}
	fmt.Fprintf(opts.Stdout, "Installing kubectl %s...\n", ver)

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download kubectl %s and install to /usr/local/bin\n", ver)
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "kubectl-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	binaryPath := filepath.Join(tmpDir, "kubectl")
	checksumPath := filepath.Join(tmpDir, "kubectl.sha256")

	dlURL := fmt.Sprintf("https://dl.k8s.io/release/%s/bin/linux/%s/kubectl", ver, p.Arch)
	csURL := fmt.Sprintf("https://dl.k8s.io/release/%s/bin/linux/%s/kubectl.sha256", ver, p.Arch)

	if err := sysutil.DownloadFile(ctx, dlURL, binaryPath); err != nil {
		return err
	}
	if err := sysutil.DownloadFile(ctx, csURL, checksumPath); err != nil {
		return err
	}

	// kubectl.sha256 contains just the hash, not "hash  filename" format
	expected, err := os.ReadFile(checksumPath)
	if err != nil {
		return err
	}
	if err := sysutil.VerifyChecksum(binaryPath, strings.TrimSpace(string(expected))); err != nil {
		return err
	}

	return sysutil.InstallBinary(ctx, o, binaryPath, "/usr/local/bin/kubectl")
}

func fetchKubectlStableVersion() (string, error) {
	resp, err := sysutil.HTTPClient().Get("https://dl.k8s.io/release/stable.txt")
	if err != nil {
		return "", fmt.Errorf("fetch kubectl stable version: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func kubectlUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing kubectl...")

	if p.OS == platform.MacOS {
		return sysutil.RemovePackage(ctx, o, "kubectl")
	}
	return sysutil.SudoRun(ctx, o, "rm", "-f", "/usr/local/bin/kubectl")
}
