package component

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

var kubectlStableURL = "https://dl.k8s.io/release/stable.txt"

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
	ver, err := fetchKubectlStableVersion(ctx)
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

func fetchKubectlStableVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, kubectlStableURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := sysutil.HTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch kubectl stable version: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch kubectl stable version: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
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
