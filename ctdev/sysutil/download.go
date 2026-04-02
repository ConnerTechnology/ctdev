package sysutil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 60 * time.Second}

// DownloadFile downloads a URL to a local file path.
func DownloadFile(url, dest string) error {
	resp, err := httpClient.Get(url)
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
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// GitHubLatestVersion fetches the latest release version from a GitHub repo.
// Returns the version without the "v" prefix.
func GitHubLatestVersion(repo string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	resp, err := httpClient.Get(url)
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
		if len(fields) == 2 && fields[1] == target {
			if fields[0] == actual {
				return nil
			}
			return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", target, fields[0], actual)
		}
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
func InstallBinary(o Opts, src, dest string) error {
	return SudoRun(o, "install", "-o", "root", "-g", "root", "-m", "0755", src, dest)
}

// GitHubBinarySpec describes how to download and install a binary from a GitHub release.
type GitHubBinarySpec struct {
	Repo        string                                     // e.g. "helm/helm"
	ArchiveURL  func(version, goos, goarch string) string  // returns download URL
	ChecksumURL func(version, goos, goarch string) string  // optional, returns checksum URL
	BinaryPath  func(goos, goarch string) string           // path within extracted archive
	InstallDest string                                      // e.g. "/usr/local/bin/helm"
	ArchFormat  string                                      // "tar.gz", "zip", or "" for raw binary
}

// DownloadGitHubBinary fetches the latest release, downloads, verifies, extracts, and installs.
// Returns the version string and any error.
func DownloadGitHubBinary(o Opts, spec GitHubBinarySpec) (string, error) {
	ver, err := GitHubLatestVersion(spec.Repo)
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

	if err := DownloadFile(dlURL, archivePath); err != nil {
		return ver, fmt.Errorf("download %s: %w", spec.Repo, err)
	}

	if spec.ChecksumURL != nil {
		csURL := spec.ChecksumURL(ver, goos, goarch)
		csPath := filepath.Join(tmpDir, "checksums.txt")
		if err := DownloadFile(csURL, csPath); err != nil {
			return ver, fmt.Errorf("download checksums for %s: %w", spec.Repo, err)
		}
		if err := VerifyChecksumFile(archivePath, csPath); err != nil {
			return ver, err
		}
	}

	switch spec.ArchFormat {
	case "tar.gz":
		if err := Run(o, "tar", "-xzf", archivePath, "-C", tmpDir); err != nil {
			return ver, fmt.Errorf("extract %s: %w", spec.Repo, err)
		}
	case "zip":
		if err := Run(o, "unzip", "-o", archivePath, "-d", tmpDir); err != nil {
			return ver, fmt.Errorf("extract %s: %w", spec.Repo, err)
		}
	case "":
		// Raw binary, no extraction needed
		return ver, InstallBinary(o, archivePath, spec.InstallDest)
	}

	binaryPath := filepath.Join(tmpDir, spec.BinaryPath(goos, goarch))
	return ver, InstallBinary(o, binaryPath, spec.InstallDest)
}
