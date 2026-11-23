package component

import (
	"bufio"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// installElectronDeb installs an Electron app's .deb from a GitHub release,
// checked against the sha512 electron-builder publishes for it in the
// release's update manifest. base is the release's download URL with a
// trailing slash; deb and manifest are asset names in it.
func installElectronDeb(ctx context.Context, o sysutil.Opts, base, deb, manifest, pkg string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download %s, verify it against %s, and install it\n", base+deb, manifest)
		return nil
	}

	want, err := electronManifestSHA512(ctx, base+manifest, deb)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", pkg+"-*.deb")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	tmp.Close()
	if err := sysutil.DownloadFile(ctx, base+deb, tmp.Name()); err != nil {
		return fmt.Errorf("download %s: %w", deb, err)
	}
	if err := verifySHA512Base64(tmp.Name(), want); err != nil {
		return fmt.Errorf("%s: %w", deb, err)
	}
	return installDebWithDepFix(ctx, o, tmp.Name(), pkg)
}

// electronManifestSHA512 fetches an electron-builder latest-*.yml and returns
// the sha512 of the named file.
func electronManifestSHA512(ctx context.Context, url, file string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := sysutil.HTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
	}
	return parseElectronManifestSHA512(resp.Body, file)
}

// parseElectronManifestSHA512 reads the files list of an electron-builder
// update manifest:
//
//	files:
//	  - url: T3-Code-0.1.7-amd64.deb
//	    sha512: <base64>
//
// It fails closed: a file that isn't listed, or has no hash, is an error.
func parseElectronManifestSHA512(r io.Reader, file string) (string, error) {
	sc := bufio.NewScanner(r)
	inEntry := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if v, ok := strings.CutPrefix(line, "- url:"); ok {
			inEntry = strings.TrimSpace(v) == file
			continue
		}
		if !inEntry {
			continue
		}
		if v, ok := strings.CutPrefix(line, "sha512:"); ok {
			if sum := strings.TrimSpace(v); sum != "" {
				return sum, nil
			}
			break
		}
		if strings.HasPrefix(line, "- ") || !strings.HasPrefix(sc.Text(), " ") {
			inEntry = false // the next entry, or a top-level key
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("update manifest has no sha512 for %s", file)
}

func verifySHA512Base64(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := base64.StdEncoding.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("sha512 mismatch: got %s, want %s", got, want)
	}
	return nil
}
