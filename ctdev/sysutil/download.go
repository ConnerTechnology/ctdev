package sysutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var httpClient = &http.Client{
	Timeout: 60 * time.Second,
	// Every URL we fetch is https; refuse a redirect that downgrades to
	// plaintext so a compromised endpoint can't bounce a download onto http.
	// Loopback targets stay allowed for httptest-backed tests.
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if req.URL.Scheme != "https" && !isLoopbackHost(req.URL.Hostname()) {
			return fmt.Errorf("refusing redirect to non-https url %q", req.URL)
		}
		return nil
	},
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// HTTPClient returns the shared HTTP client with a 60-second timeout.
func HTTPClient() *http.Client { return httpClient }

// httpGet issues a GET with ctx so cancellation terminates in-flight requests.
func httpGet(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return httpClient.Do(req)
}

// DownloadFile downloads a URL to a local file path, honoring ctx for cancellation.
// On error the partial destination file is removed so retries see a clean slate.
func DownloadFile(ctx context.Context, url, dest string) error {
	resp, err := httpGet(ctx, url)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(dest)
		return fmt.Errorf("download %s: %w", url, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(dest)
		return fmt.Errorf("download %s: %w", url, err)
	}
	return nil
}

// GitHubLatestVersion fetches the latest release version from a GitHub repo.
// Returns the version without the "v" prefix.
func GitHubLatestVersion(ctx context.Context, repo string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	resp, err := httpGet(ctx, url)
	if err != nil {
		return "", fmt.Errorf("fetch latest version for %s: %w", repo, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch latest version for %s: HTTP %d", repo, resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("parse release for %s: %w", repo, err)
	}
	return strings.TrimPrefix(release.TagName, "v"), nil
}

// VerifyChecksumFile verifies a file's SHA256 hash against a checksums file.
// The checksums file should contain lines in the format: "<hash>  <filename>"
func VerifyChecksumFile(filePath, checksumsPath string) error {
	// Compute actual hash
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))

	// Read expected hash from checksums file
	data, err := os.ReadFile(checksumsPath)
	if err != nil {
		return err
	}

	target := filepath.Base(filePath)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// Tolerate the `sha256sum -b` binary marker ("hash *file"), uppercase
		// hashes, and trailing columns by matching on the filename field.
		name := strings.TrimPrefix(fields[1], "*")
		if name != target {
			continue
		}
		if strings.EqualFold(fields[0], actual) {
			return nil
		}
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", target, fields[0], actual)
	}
	return fmt.Errorf("no checksum found for %s in checksums file", target)
}

// VerifyChecksum verifies a file's SHA256 hash against an expected hash string.
func VerifyChecksum(filePath, expected string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))

	expected = strings.TrimSpace(expected)
	if actual != expected {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}

// InstallBinary copies a binary to a destination path with sudo.
func InstallBinary(ctx context.Context, o Opts, src, dest string) error {
	return SudoRun(ctx, o, "install", "-o", "root", "-g", "root", "-m", "0755", src, dest)
}

// GitHubBinarySpec describes how to download and install a binary from a GitHub release.
type GitHubBinarySpec struct {
	Repo        string                                    // e.g. "helm/helm"
	ArchiveURL  func(version, goos, goarch string) string // returns download URL
	ChecksumURL func(version, goos, goarch string) string // optional, returns checksum URL
	BinaryPath  func(goos, goarch string) string          // path within extracted archive
	InstallDest string                                    // e.g. "/usr/local/bin/helm"
	ArchFormat  string                                    // "tar.gz", "zip", or "" for raw binary
}

// DownloadGitHubBinary fetches the latest release, downloads, verifies, extracts, and installs.
// Returns the version string and any error.
func DownloadGitHubBinary(ctx context.Context, o Opts, spec GitHubBinarySpec) (string, error) {
	ver, err := GitHubLatestVersion(ctx, spec.Repo)
	if err != nil {
		return "", err
	}

	goos := runtime.GOOS
	goarch := runtime.GOARCH

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download %s v%s and install to %s\n", spec.Repo, ver, spec.InstallDest)
		return ver, nil
	}

	tmpDir, err := os.MkdirTemp("", "ctdev-dl-*")
	if err != nil {
		return ver, err
	}
	defer os.RemoveAll(tmpDir)

	dlURL := spec.ArchiveURL(ver, goos, goarch)
	archiveName := filepath.Base(dlURL)
	archivePath := filepath.Join(tmpDir, archiveName)

	if err := DownloadFile(ctx, dlURL, archivePath); err != nil {
		return ver, fmt.Errorf("download %s: %w", spec.Repo, err)
	}

	if spec.ChecksumURL != nil {
		csURL := spec.ChecksumURL(ver, goos, goarch)
		csPath := filepath.Join(tmpDir, "checksums.txt")
		if err := DownloadFile(ctx, csURL, csPath); err != nil {
			return ver, fmt.Errorf("download checksums for %s: %w", spec.Repo, err)
		}
		if err := VerifyChecksumFile(archivePath, csPath); err != nil {
			return ver, err
		}
	}

	switch spec.ArchFormat {
	case "tar.gz":
		if err := Run(ctx, o, "tar", "-xzf", archivePath, "-C", tmpDir); err != nil {
			return ver, fmt.Errorf("extract %s: %w", spec.Repo, err)
		}
	case "zip":
		if err := Run(ctx, o, "unzip", "-o", archivePath, "-d", tmpDir); err != nil {
			return ver, fmt.Errorf("extract %s: %w", spec.Repo, err)
		}
	case "":
		// Raw binary, no extraction needed
		return ver, InstallBinary(ctx, o, archivePath, spec.InstallDest)
	}

	binaryPath := filepath.Join(tmpDir, spec.BinaryPath(goos, goarch))
	return ver, InstallBinary(ctx, o, binaryPath, spec.InstallDest)
}
