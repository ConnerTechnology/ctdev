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
	"strings"
)

// DownloadFile downloads a URL to a local file path.
func DownloadFile(url, dest string) error {
	resp, err := http.Get(url)
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
	resp, err := http.Get(url)
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
