// Latest-version lookups (GitHub, go.dev, dl.k8s.io, nodenv/ruby-build) and
// version-comparison helpers.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// fetchGitHubReleaseTags returns release tags from newest to oldest (excludes pre-releases).
func fetchGitHubReleaseTags(ctx context.Context, repo string) ([]string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=50", repo)
	resp, err := httpGetJSON(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var releases []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	var tags []string
	for _, r := range releases {
		if !r.Prerelease {
			tags = append(tags, r.TagName)
		}
	}
	return tags, nil
}

func majorVersion(ver string) int {
	parts := strings.SplitN(ver, ".", 2)
	if len(parts) == 0 {
		return 0
	}
	var major int
	fmt.Sscanf(parts[0], "%d", &major)
	return major
}

// versionParts splits a dotted version into its leading numeric components,
// ignoring a leading "v" and any pre-release/build suffix.
func versionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	fields := strings.FieldsFunc(v, func(r rune) bool {
		return r == '.' || r == '-' || r == '+' || r == '_'
	})
	var parts []int
	for _, f := range fields {
		num, got := 0, false
		for _, r := range f {
			if r < '0' || r > '9' {
				break
			}
			num, got = num*10+int(r-'0'), true
		}
		if !got {
			break
		}
		parts = append(parts, num)
	}
	return parts
}

// versionNewer reports whether candidate is a strictly higher dotted-numeric
// version than current. It's used to avoid offering a "downgrade" when the
// locally installed tool is newer than the registry's notion of latest (e.g. a
// manually built Go or a pre-release). Equal versions return false.
func versionNewer(candidate, current string) bool {
	c, cur := versionParts(candidate), versionParts(current)
	n := len(c)
	if len(cur) > n {
		n = len(cur)
	}
	for i := 0; i < n; i++ {
		a, b := 0, 0
		if i < len(c) {
			a = c[i]
		}
		if i < len(cur) {
			b = cur[i]
		}
		if a != b {
			return a > b
		}
	}
	return false
}

func fetchLatestNodeLTS(ctx context.Context) string {
	// Use a simpler endpoint that returns just the LTS schedule
	// The full index.json is huge, so we use nodenv's definitions if available
	if _, err := exec.LookPath("nodenv"); err == nil {
		out, err := exec.CommandContext(ctx, "nodenv", "install", "--list").Output()
		if err == nil {
			// Find latest even-numbered major version (LTS)
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			for i := len(lines) - 1; i >= 0; i-- {
				v := strings.TrimSpace(lines[i])
				if v == "" || strings.Contains(v, "-") {
					continue
				}
				parts := strings.SplitN(v, ".", 2)
				if len(parts) < 2 {
					continue
				}
				major := 0
				fmt.Sscanf(parts[0], "%d", &major)
				if major > 0 && major%2 == 0 {
					return v
				}
			}
		}
	}
	return ""
}

// goRelease mirrors the subset of https://go.dev/dl/?mode=json we consume.
type goRelease struct {
	Version string `json:"version"`
	Files   []struct {
		Filename string `json:"filename"`
		OS       string `json:"os"`
		Arch     string `json:"arch"`
		Kind     string `json:"kind"`
		SHA256   string `json:"sha256"`
	} `json:"files"`
}

// httpGetJSON issues a GET with ctx so Ctrl-C cancels in-flight requests.
// Callers are expected to close the response body.
func httpGetJSON(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return sysutil.HTTPClient().Do(req)
}

// fetchGoReleases returns the list of published Go releases (newest first).
// Each release includes per-OS/arch archive metadata with SHA256 hashes.
func fetchGoReleases(ctx context.Context) ([]goRelease, error) {
	resp, err := httpGetJSON(ctx, "https://go.dev/dl/?mode=json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var releases []goRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	return releases, nil
}

func fetchLatestGoVersion(ctx context.Context) (string, error) {
	releases, err := fetchGoReleases(ctx)
	if err != nil {
		return "", err
	}
	if len(releases) == 0 {
		return "", nil
	}
	return strings.TrimPrefix(releases[0].Version, "go"), nil
}

// goArchiveSHA256 looks up the published sha256 for the archive tarball
// matching ver/goos/goarch. Returns "" if not found.
func goArchiveSHA256(releases []goRelease, ver, goos, goarch string) string {
	target := fmt.Sprintf("go%s.%s-%s.tar.gz", ver, goos, goarch)
	for _, r := range releases {
		if strings.TrimPrefix(r.Version, "go") != ver {
			continue
		}
		for _, f := range r.Files {
			if f.Filename == target {
				return f.SHA256
			}
		}
	}
	return ""
}

func fetchLatestRubyVersion(ctx context.Context) string {
	// Use ruby-build's definitions if available (from rbenv)
	if _, err := exec.LookPath("ruby-build"); err == nil {
		out, err := exec.CommandContext(ctx, "ruby-build", "--definitions").Output()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			// Walk backwards to find latest stable (numeric only, no -dev/-preview/-rc)
			for i := len(lines) - 1; i >= 0; i-- {
				v := strings.TrimSpace(lines[i])
				if v == "" {
					continue
				}
				// Skip non-release versions
				if strings.ContainsAny(v, "-") {
					continue
				}
				// Must start with a digit
				if v[0] >= '0' && v[0] <= '9' {
					return v
				}
			}
		}
	}
	return ""
}

func fetchLatestKubectlVersion(ctx context.Context) (string, error) {
	resp, err := httpGetJSON(ctx, "https://dl.k8s.io/release/stable.txt")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(strings.TrimSpace(string(body)), "v"), nil
}
