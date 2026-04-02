# Go CLI Rewrite Completion — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the ctdev Go CLI rewrite — fix bugs, add helpers, port 13 remaining components, add test coverage, polish TUI.

**Architecture:** Phased approach building on existing Cobra + Bubble Tea v2 architecture. Phase 1 adds foundation helpers that Phase 2 consumes. Phase 3 adds tests. Phase 4 polishes UX. Phase 5 cleans up.

**Tech Stack:** Go 1.24, Cobra, Bubble Tea v2, Bubbles v2, Lip Gloss v2

**Spec:** `docs/superpowers/specs/2026-04-02-go-rewrite-completion-design.md`

---

## Phase 1: Foundation Fixes & Helpers

### Task 1: Cache platform.Detect() with sync.Once

**Files:**
- Modify: `ctdev/platform/detect.go`
- Modify: `ctdev/platform/detect_test.go`

- [ ] **Step 1: Write test for cached Detect()**

In `ctdev/platform/detect_test.go`, add:

```go
func TestDetectReturnsSameInstance(t *testing.T) {
	a := Detect()
	b := Detect()
	if a.OS != b.OS || a.Arch != b.Arch || a.Distro != b.Distro {
		t.Error("expected Detect() to return consistent results")
	}
}
```

- [ ] **Step 2: Run test**

Run: `cd ctdev && go test ./platform/ -run TestDetectReturnsSameInstance -v`
Expected: PASS (returns consistent results even without caching, but proves the contract)

- [ ] **Step 3: Add sync.Once caching to Detect()**

In `ctdev/platform/detect.go`, rename the existing `Detect()` to `detect()` (lowercase) and wrap with cache:

```go
import "sync"

var (
	cachedInfo Info
	detectOnce sync.Once
)

func Detect() Info {
	detectOnce.Do(func() {
		cachedInfo = detect()
	})
	return cachedInfo
}

func detect() Info {
	// ... existing Detect() body, unchanged ...
}
```

Also rename the existing internal helpers called by `Detect()` — no changes needed since `detectOS()`, `detectArch()`, etc. are already lowercase.

- [ ] **Step 4: Run all platform tests**

Run: `cd ctdev && go test ./platform/ -v`
Expected: All PASS

- [ ] **Step 5: Build to verify no regressions**

Run: `cd ctdev && go build ./...`
Expected: Clean build

- [ ] **Step 6: Commit**

```bash
git add ctdev/platform/detect.go ctdev/platform/detect_test.go
git commit -m "perf: cache platform.Detect() with sync.Once"
```

---

### Task 2: SimplePackageInstaller / SimplePackageUninstaller helpers

**Files:**
- Create: `ctdev/component/helpers.go`
- Create: `ctdev/component/helpers_test.go`
- Modify: `ctdev/component/registry.go` (update btop, jq, shellcheck, tmux entries)
- Delete: `ctdev/component/install_btop.go`
- Delete: `ctdev/component/install_jq.go`
- Delete: `ctdev/component/install_shellcheck.go`
- Delete: `ctdev/component/install_tmux.go`

- [ ] **Step 1: Write tests for helpers**

Create `ctdev/component/helpers_test.go`:

```go
package component

import (
	"bytes"
	"context"
	"testing"
)

func TestSimplePackageInstallerSkipsIfInstalled(t *testing.T) {
	// "go" is guaranteed to exist in test environment
	installer := SimplePackageInstaller("go")
	var buf bytes.Buffer
	err := installer(context.Background(), ExecOpts{
		Stdout: &buf,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("already installed")) {
		t.Errorf("expected 'already installed' message, got: %s", buf.String())
	}
}

func TestSimplePackageInstallerForceBypass(t *testing.T) {
	installer := SimplePackageInstaller("go")
	var buf bytes.Buffer
	err := installer(context.Background(), ExecOpts{
		Stdout: &buf,
		DryRun: true,
		Force:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bytes.Contains(buf.Bytes(), []byte("already installed")) {
		t.Error("expected force to bypass installed check")
	}
}
```

- [ ] **Step 2: Run tests — expect failure**

Run: `cd ctdev && go test ./component/ -run TestSimplePackage -v`
Expected: FAIL — `SimplePackageInstaller` not defined

- [ ] **Step 3: Create helpers.go**

Create `ctdev/component/helpers.go`:

```go
package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func SimplePackageInstaller(name string) func(context.Context, ExecOpts) error {
	return func(ctx context.Context, opts ExecOpts) error {
		o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
		if !opts.Force && sysutil.CommandExists(name) {
			fmt.Fprintf(opts.Stdout, "%s already installed\n", name)
			return nil
		}
		fmt.Fprintf(opts.Stdout, "Installing %s...\n", name)
		return sysutil.InstallPackage(o, name)
	}
}

func SimplePackageUninstaller(name string) func(context.Context, ExecOpts) error {
	return func(ctx context.Context, opts ExecOpts) error {
		o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
		fmt.Fprintf(opts.Stdout, "Removing %s...\n", name)
		return sysutil.RemovePackage(o, name)
	}
}
```

- [ ] **Step 4: Run tests — expect pass**

Run: `cd ctdev && go test ./component/ -run TestSimplePackage -v`
Expected: PASS

- [ ] **Step 5: Update registry and delete old files**

In `ctdev/component/registry.go`, update btop, jq, shellcheck, tmux entries to use helpers:

```go
{Name: "btop", Description: "Resource monitor", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: SimplePackageInstaller("btop"), GoUninstall: SimplePackageUninstaller("btop"), Tags: []string{"monitor", "htop"}},
{Name: "jq", Description: "JSON processor", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: SimplePackageInstaller("jq"), GoUninstall: SimplePackageUninstaller("jq"), Tags: []string{"json", "parser"}},
{Name: "shellcheck", Description: "Shell script linter", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: SimplePackageInstaller("shellcheck"), GoUninstall: SimplePackageUninstaller("shellcheck"), Tags: []string{"lint", "bash"}},
{Name: "tmux", Description: "Terminal multiplexer", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: SimplePackageInstaller("tmux"), GoUninstall: SimplePackageUninstaller("tmux"), Tags: []string{"terminal", "session"}},
```

Delete: `ctdev/component/install_btop.go`, `install_jq.go`, `install_shellcheck.go`, `install_tmux.go`

- [ ] **Step 6: Run all tests**

Run: `cd ctdev && go build ./... && go test ./...`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git rm ctdev/component/install_btop.go ctdev/component/install_jq.go ctdev/component/install_shellcheck.go ctdev/component/install_tmux.go
git add ctdev/component/helpers.go ctdev/component/helpers_test.go ctdev/component/registry.go
git commit -m "refactor: replace 4 trivial install files with SimplePackageInstaller helper"
```

---

### Task 3: DownloadGitHubBinary helper

**Files:**
- Modify: `ctdev/sysutil/download.go`
- Create: `ctdev/sysutil/download_test.go`

- [ ] **Step 1: Write test for checksum verification (existing but untested)**

Create `ctdev/sysutil/download_test.go`:

```go
package sysutil

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.bin")
	content := []byte("hello world")
	if err := os.WriteFile(file, content, 0644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(content)
	expected := hex.EncodeToString(h[:])

	if err := VerifyChecksum(file, expected); err != nil {
		t.Errorf("expected valid checksum, got: %v", err)
	}
	if err := VerifyChecksum(file, "0000badchecksum"); err == nil {
		t.Error("expected checksum mismatch error")
	}
}

func TestVerifyChecksumFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.bin")
	content := []byte("test data")
	if err := os.WriteFile(file, content, 0644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(content)
	hash := hex.EncodeToString(h[:])

	checksums := filepath.Join(dir, "SHA256SUMS")
	csContent := hash + "  test.bin\ndeadbeef  other.bin\n"
	if err := os.WriteFile(checksums, []byte(csContent), 0644); err != nil {
		t.Fatal(err)
	}

	if err := VerifyChecksumFile(file, checksums); err != nil {
		t.Errorf("expected valid checksum file match, got: %v", err)
	}
}

func TestVerifyChecksumFileMissing(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.bin")
	if err := os.WriteFile(file, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	checksums := filepath.Join(dir, "SHA256SUMS")
	if err := os.WriteFile(checksums, []byte("abc123  other.bin\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := VerifyChecksumFile(file, checksums)
	if err == nil {
		t.Error("expected error for missing file in checksums")
	}
}
```

- [ ] **Step 2: Run tests**

Run: `cd ctdev && go test ./sysutil/ -run TestVerifyChecksum -v`
Expected: PASS (tests existing functionality)

- [ ] **Step 3: Add DownloadGitHubBinary to download.go**

In `ctdev/sysutil/download.go`, add:

```go
// GitHubBinarySpec describes how to download and install a binary from a GitHub release.
type GitHubBinarySpec struct {
	Repo        string                                    // e.g. "helm/helm"
	ArchiveURL  func(version, goos, goarch string) string // returns download URL
	ChecksumURL func(version, goos, goarch string) string // optional, returns checksum URL
	BinaryPath  func(goos, goarch string) string          // path within extracted archive
	InstallDest string                                     // e.g. "/usr/local/bin/helm"
	ArchFormat  string                                     // "tar.gz", "zip", or "" for raw binary
}

// DownloadGitHubBinary fetches the latest release, downloads, verifies, extracts, and installs.
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

	// Verify checksum if provided
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

	// Extract if archived
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
		// Raw binary, no extraction needed — archivePath IS the binary
		return ver, InstallBinary(o, archivePath, spec.InstallDest)
	}

	binaryPath := filepath.Join(tmpDir, spec.BinaryPath(goos, goarch))
	return ver, InstallBinary(o, binaryPath, spec.InstallDest)
}
```

Add `"runtime"` to the imports.

- [ ] **Step 4: Build to verify**

Run: `cd ctdev && go build ./...`
Expected: Clean build

- [ ] **Step 5: Commit**

```bash
git add ctdev/sysutil/download.go ctdev/sysutil/download_test.go
git commit -m "feat: add DownloadGitHubBinary helper and checksum tests"
```

---

### Task 4: Refactor existing binary installers to use DownloadGitHubBinary

**Files:**
- Modify: `ctdev/component/install_helm.go`
- Modify: `ctdev/component/install_doctl.go`
- Modify: `ctdev/component/install_kubectl.go`
- Modify: `ctdev/component/install_age.go`
- Modify: `ctdev/component/install_sops.go`
- Modify: `ctdev/component/install_git_spice.go`

- [ ] **Step 1: Refactor helm installer**

Replace `ctdev/component/install_helm.go` body of `helmInstall` (after the macOS/force checks):

```go
func helmInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

	if !opts.Force && sysutil.CommandExists("helm") {
		fmt.Fprintln(opts.Stdout, "helm already installed")
		return nil
	}

	if p.OS == platform.MacOS {
		fmt.Fprintln(opts.Stdout, "Installing helm...")
		return sysutil.InstallPackage(o, "helm")
	}

	fmt.Fprintln(opts.Stdout, "Installing helm...")
	ver, err := sysutil.DownloadGitHubBinary(o, sysutil.GitHubBinarySpec{
		Repo: "helm/helm",
		ArchiveURL: func(ver, goos, goarch string) string {
			return fmt.Sprintf("https://get.helm.sh/helm-v%s-%s-%s.tar.gz", ver, goos, goarch)
		},
		ChecksumURL: func(ver, goos, goarch string) string {
			return fmt.Sprintf("https://get.helm.sh/helm-v%s-%s-%s.tar.gz.sha256sum", ver, goos, goarch)
		},
		BinaryPath: func(goos, goarch string) string {
			return fmt.Sprintf("%s-%s/helm", goos, goarch)
		},
		InstallDest: "/usr/local/bin/helm",
		ArchFormat:  "tar.gz",
	})
	if err != nil {
		return err
	}
	if !o.DryRun {
		fmt.Fprintf(opts.Stdout, "helm %s installed\n", ver)
	}
	return nil
}
```

- [ ] **Step 2: Refactor doctl, kubectl, age, sops, git-spice similarly**

Apply the same pattern to each file. Each installer shrinks to: force/installed check → macOS path → `DownloadGitHubBinary()` call with the spec struct. The uninstall functions stay unchanged.

For `install_kubectl.go` (raw binary, no archive):
```go
ArchFormat: "",  // raw binary
ArchiveURL: func(ver, goos, goarch string) string {
    return fmt.Sprintf("https://dl.k8s.io/release/v%s/bin/%s/%s/kubectl", ver, goos, goarch)
},
```

Note: kubectl uses its own version endpoint, not GitHub releases. Keep the custom version fetch but use `DownloadFile` + `InstallBinary` directly (don't use `DownloadGitHubBinary` for kubectl since it doesn't use GitHub releases for the binary).

For `install_age.go` — age has two binaries (age, age-keygen) in the archive. Keep it using the manual pattern since `DownloadGitHubBinary` only handles one binary. Just simplify by extracting the tmpdir+download+extract logic into shared code where possible.

- [ ] **Step 3: Build and test**

Run: `cd ctdev && go build ./... && go test ./...`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add ctdev/component/install_helm.go ctdev/component/install_doctl.go ctdev/component/install_sops.go ctdev/component/install_git_spice.go
git commit -m "refactor: use DownloadGitHubBinary helper in binary installers"
```

---

### Task 5: Replace hand-rolled JSON in update.go

**Files:**
- Modify: `ctdev/cmd/update.go`

- [ ] **Step 1: Replace fetchLatestGitHubTag with sysutil.GitHubLatestVersion**

In `ctdev/cmd/update.go`:
- Add import: `"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"`
- Replace all calls to `fetchLatestGitHubTag(ctx, "repo")` with `sysutil.GitHubLatestVersion("repo")` (drop the `ctx` param, add error handling)
- Delete `fetchLatestGitHubTag()` function

Callers: `scanCtdev`, `scanTerraform`, `scanBun` — each needs the return value adapted from `(string)` to `(string, error)`.

- [ ] **Step 2: Replace fetchGitHubReleaseTags with encoding/json**

Replace `fetchGitHubReleaseTags()` with:

```go
func fetchGitHubReleaseTags(repo string) []string {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=50", repo)
	resp, err := sysutil.HTTPClient().Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var releases []struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil
	}
	tags := make([]string, len(releases))
	for i, r := range releases {
		tags[i] = r.TagName
	}
	return tags
}
```

Add `"encoding/json"` to imports. Expose `sysutil.httpClient` via `sysutil.HTTPClient()` getter, or just create a local client with timeout.

- [ ] **Step 3: Replace parseNPMOutdated with encoding/json**

Replace `parseNPMOutdated()` and `extractJSONValue()`:

```go
func parseNPMOutdatedJSON(content string) ([]checklist.UpdateItem, error) {
	var packages map[string]struct {
		Current string `json:"current"`
		Latest  string `json:"latest"`
	}
	if err := json.Unmarshal([]byte(content), &packages); err != nil {
		return nil, err
	}
	var items []checklist.UpdateItem
	for name, pkg := range packages {
		if pkg.Current != pkg.Latest {
			items = append(items, checklist.UpdateItem{
				Name:       name,
				Source:     "npm",
				CurrentVer: pkg.Current,
				NewVer:     pkg.Latest,
			})
		}
	}
	return items, nil
}
```

Update `scanNPMGlobals` to call `parseNPMOutdatedJSON` instead of `parseNPMOutdated`.

- [ ] **Step 4: Replace kubectl/terraform JSON parsing**

In `scanKubectl`, replace the line-by-line extraction with:

```go
var kubectlVer struct {
	ClientVersion struct {
		GitVersion string `json:"gitVersion"`
	} `json:"clientVersion"`
}
if err := json.Unmarshal(out, &kubectlVer); err != nil {
	return nil, nil
}
current := strings.TrimPrefix(kubectlVer.ClientVersion.GitVersion, "v")
```

In `scanTerraform`:

```go
var tfVer struct {
	TerraformVersion string `json:"terraform_version"`
}
if err := json.Unmarshal(out, &tfVer); err != nil {
	return nil, nil
}
current := tfVer.TerraformVersion
```

- [ ] **Step 5: Delete extractJSONValue**

Remove `extractJSONValue()` and verify no remaining callers.

- [ ] **Step 6: Build**

Run: `cd ctdev && go build ./...`
Expected: Clean build

- [ ] **Step 7: Commit**

```bash
git add ctdev/cmd/update.go
git commit -m "refactor: replace hand-rolled JSON parsing with encoding/json"
```

---

### Task 6: Fix refreshAPTKeys

**Files:**
- Modify: `ctdev/cmd/update.go`

- [ ] **Step 1: Implement refreshAPTKeys**

Replace the empty `refreshAPTKeys` function:

```go
var aptKeyRefreshers = map[string]struct {
	KeyURL     string
	KeyringPath string
}{
	"gh":        {KeyURL: "https://cli.github.com/packages/githubcli-archive-keyring.gpg", KeyringPath: "/usr/share/keyrings/githubcli-archive-keyring.gpg"},
	"vscode":    {KeyURL: "https://packages.microsoft.com/keys/microsoft.asc", KeyringPath: "/usr/share/keyrings/packages.microsoft.gpg"},
	"1password": {KeyURL: "https://downloads.1password.com/linux/keys/1password.asc", KeyringPath: "/usr/share/keyrings/1password-archive-keyring.gpg"},
	"terraform": {KeyURL: "https://apt.releases.hashicorp.com/gpg", KeyringPath: "/usr/share/keyrings/hashicorp-archive-keyring.gpg"},
	"tailscale": {KeyURL: "https://pkgs.tailscale.com/stable/ubuntu/noble.noarmor.gpg", KeyringPath: "/usr/share/keyrings/tailscale-archive-keyring.gpg"},
}

func refreshAPTKeys(components []string) {
	if _, err := exec.LookPath("apt"); err != nil {
		return
	}
	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}
	targets := aptKeyRefreshers
	if len(components) > 0 {
		targets = make(map[string]struct{ KeyURL, KeyringPath string })
		for _, name := range components {
			if r, ok := aptKeyRefreshers[name]; ok {
				targets[name] = r
			}
		}
	}
	for name, r := range targets {
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("  Refreshing %s key...", name)))
		if err := sysutil.AddAPTKeyring(o, r.KeyURL, r.KeyringPath); err != nil {
			fmt.Printf("  %s\n", styles.Warning.Render(fmt.Sprintf("Warning: %s key refresh failed: %v", name, err)))
		}
	}
}
```

Add import for `"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"` and `"os"` if not already present.

- [ ] **Step 2: Build and verify**

Run: `cd ctdev && go build ./...`
Expected: Clean build

- [ ] **Step 3: Commit**

```bash
git add ctdev/cmd/update.go
git commit -m "fix: implement refreshAPTKeys to re-download GPG keyrings"
```

---

### Task 7: Fix cleanup audit no-op

**Files:**
- Modify: `ctdev/cmd/cleanup.go`
- Create: `ctdev/cmd/cleanup_test.go`

- [ ] **Step 1: Write test for duplicate detection**

Create `ctdev/cmd/cleanup_test.go`:

```go
package cmd

import (
	"testing"
)

func TestFindDuplicateAPTSources(t *testing.T) {
	files := map[string]string{
		"github-cli.list": "deb [arch=amd64] https://cli.github.com/packages stable main\n",
		"github-cli-dup.list": "deb [arch=amd64] https://cli.github.com/packages stable main\n",
		"vscode.list": "deb [arch=amd64] https://packages.microsoft.com/repos/code stable main\n",
	}
	dups := findDuplicateSourceLines(files)
	if len(dups) != 1 {
		t.Errorf("expected 1 duplicate, got %d", len(dups))
	}
}

func TestFindDuplicateAPTSourcesNoDups(t *testing.T) {
	files := map[string]string{
		"github-cli.list": "deb [arch=amd64] https://cli.github.com/packages stable main\n",
		"vscode.list": "deb [arch=amd64] https://packages.microsoft.com/repos/code stable main\n",
	}
	dups := findDuplicateSourceLines(files)
	if len(dups) != 0 {
		t.Errorf("expected 0 duplicates, got %d", len(dups))
	}
}
```

- [ ] **Step 2: Run test — expect failure**

Run: `cd ctdev && go test ./cmd/ -run TestFindDuplicate -v`
Expected: FAIL — `findDuplicateSourceLines` not defined

- [ ] **Step 3: Implement findDuplicateSourceLines**

In `ctdev/cmd/cleanup.go`, add:

```go
type duplicateSource struct {
	Line  string
	Files []string
}

func findDuplicateSourceLines(fileContents map[string]string) []duplicateSource {
	seen := make(map[string][]string) // normalized line → list of files
	for filename, content := range fileContents {
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			seen[line] = append(seen[line], filename)
		}
	}
	var dups []duplicateSource
	for line, files := range seen {
		if len(files) > 1 {
			dups = append(dups, duplicateSource{Line: line, Files: files})
		}
	}
	return dups
}
```

- [ ] **Step 4: Run test — expect pass**

Run: `cd ctdev && go test ./cmd/ -run TestFindDuplicate -v`
Expected: PASS

- [ ] **Step 5: Wire into cleanup execute handler**

Replace the audit task's `execute` function:

```go
execute: func() error {
	files := readAPTSourceFiles()
	dups := findDuplicateSourceLines(files)
	if len(dups) == 0 {
		fmt.Println("  No duplicate repositories found.")
		return nil
	}
	for _, d := range dups {
		fmt.Printf("  Duplicate: %s\n    In: %s\n", d.Line, strings.Join(d.Files, ", "))
	}
	return nil
},
```

Add `readAPTSourceFiles`:

```go
func readAPTSourceFiles() map[string]string {
	dir := "/etc/apt/sources.list.d"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	files := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() || (!strings.HasSuffix(e.Name(), ".list") && !strings.HasSuffix(e.Name(), ".sources")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		files[e.Name()] = string(data)
	}
	return files
}
```

Add `"os"` and `"path/filepath"` to imports.

- [ ] **Step 6: Build and test**

Run: `cd ctdev && go build ./... && go test ./cmd/ -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add ctdev/cmd/cleanup.go ctdev/cmd/cleanup_test.go
git commit -m "fix: implement APT duplicate repository detection in cleanup"
```

---

## Phase 2: Port Remaining Components

### Task 8: Port Tier 1 components (chrome, ghostty, tailscale, claude-code, codex, bun, dbeaver)

**Files:**
- Create: `ctdev/component/install_chrome.go`
- Create: `ctdev/component/install_ghostty.go`
- Create: `ctdev/component/install_tailscale.go`
- Create: `ctdev/component/install_claude_code.go`
- Create: `ctdev/component/install_codex.go`
- Create: `ctdev/component/install_bun.go`
- Create: `ctdev/component/install_dbeaver.go`
- Modify: `ctdev/component/registry.go`

Each component follows the same pattern. I'll show chrome as the full example; the rest follow the same structure.

- [ ] **Step 1: Create install_chrome.go**

```go
package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func chromeInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

	if !opts.Force && sysutil.CommandExists("google-chrome") {
		fmt.Fprintln(opts.Stdout, "Chrome already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing Google Chrome...")

	switch p.PackageManager {
	case "brew":
		return sysutil.BrewCaskInstall(o, "google-chrome")
	case "apt":
		if p.Arch != "amd64" {
			return fmt.Errorf("Chrome .deb only available for amd64")
		}
		if o.DryRun {
			fmt.Fprintln(o.Stdout, "[dry-run] download chrome .deb and install")
			return nil
		}
		tmpDir, err := os.MkdirTemp("", "chrome-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)
		debPath := filepath.Join(tmpDir, "chrome.deb")
		if err := sysutil.DownloadFile("https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb", debPath); err != nil {
			return err
		}
		if err := sysutil.SudoRun(o, "dpkg", "-i", debPath); err != nil {
			// Fix broken deps
			_ = sysutil.SudoRun(o, "apt-get", "install", "-f", "-y")
		}
		return nil
	case "dnf":
		if err := sysutil.SudoRun(o, "dnf", "install", "-y", "fedora-workstation-repositories"); err != nil {
			return err
		}
		if err := sysutil.SudoRun(o, "dnf", "config-manager", "--set-enabled", "google-chrome"); err != nil {
			return err
		}
		return sysutil.InstallPackage(o, "google-chrome-stable")
	default:
		return fmt.Errorf("Chrome install not supported for: %s", p.PackageManager)
	}
}

func chromeUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing Chrome...")

	if p.PackageManager == "brew" {
		return sysutil.BrewCaskRemove(o, "google-chrome")
	}
	return sysutil.RemovePackage(o, "google-chrome-stable")
}
```

- [ ] **Step 2: Create remaining Tier 1 installers**

Create each file following the same pattern. Key details for each:

**install_ghostty.go:** brew cask `ghostty` on macOS; on apt, use `curl -fsSL https://raw.githubusercontent.com/mkasberg/ghostty-ubuntu/HEAD/install.sh | bash` via `sysutil.Run`; on pacman, `pacman -S ghostty`. Symlink config from dotfiles root.

**install_tailscale.go:** brew cask `tailscale` on macOS; on apt, add keyring from `https://pkgs.tailscale.com/stable/ubuntu/{codename}.noarmor.gpg`, add source, apt install `tailscale`, enable `tailscaled` service.

**install_claude_code.go:** Run `curl -fsSL https://claude.ai/install.sh | bash` via `sysutil.Run`. Symlink config files using `sysutil.SafeSymlink` from `components/claude-code/` to `~/.claude/`.

**install_codex.go:** brew cask `codex` on macOS; on Linux check Node.js exists, then `npm install -g @openai/codex`.

**install_bun.go:** brew `oven-sh/bun/bun` on macOS; on Linux `curl -fsSL https://bun.sh/install | bash`.

**install_dbeaver.go:** brew cask `dbeaver-community` on macOS; on apt, add keyring + source + install `dbeaver-ce`; on dnf, download RPM; on pacman, `pacman -S dbeaver`.

- [ ] **Step 3: Update registry.go**

Replace bash entries with Go functions for all 7 components:

```go
{Name: "chrome", ..., GoInstall: chromeInstall, GoUninstall: chromeUninstall, ...},
{Name: "ghostty", ..., GoInstall: ghosttyInstall, GoUninstall: ghosttyUninstall, ...},
// etc.
```

Remove `BashInstall`/`BashUninstall` fields from these entries.

- [ ] **Step 4: Build and test**

Run: `cd ctdev && go build ./... && go test ./...`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add ctdev/component/install_chrome.go ctdev/component/install_ghostty.go ctdev/component/install_tailscale.go ctdev/component/install_claude_code.go ctdev/component/install_codex.go ctdev/component/install_bun.go ctdev/component/install_dbeaver.go ctdev/component/registry.go
git commit -m "feat: port chrome, ghostty, tailscale, claude-code, codex, bun, dbeaver to Go"
```

---

### Task 9: Port Tier 2 components (fonts, docker, git, node, ruby, zsh)

**Files:**
- Create: `ctdev/component/install_fonts.go`
- Create: `ctdev/component/install_docker.go`
- Create: `ctdev/component/install_git.go`
- Create: `ctdev/component/install_node.go`
- Create: `ctdev/component/install_ruby.go`
- Create: `ctdev/component/install_zsh.go`
- Modify: `ctdev/component/registry.go`

**Stop criterion:** If any of these takes disproportionate effort, leave it as bash and move on.

- [ ] **Step 1: Port fonts**

`install_fonts.go`: Download Nerd Fonts (FiraCode, JetBrainsMono) from GitHub releases. Create `~/.local/share/fonts/` if needed. Unzip into font dir. Run `fc-cache -f` on Linux. On macOS, download to `~/Library/Fonts/`.

- [ ] **Step 2: Port docker**

`install_docker.go`: On macOS, brew cask `docker`. On apt: add Docker GPG key, add source (DEB822 format), apt install `docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin`, enable+start service, add user to docker group. On dnf: add docker-ce repo, dnf install, enable+start. On pacman: `pacman -S docker docker-compose`, enable+start.

- [ ] **Step 3: Port git**

`install_git.go`: Copy `.gitconfig` from `components/git/.gitconfig` to `~/.gitconfig` (copy, not symlink — so user changes stay local). Run configure.sh subscript via bash for name/email prompt.

- [ ] **Step 4: Port node**

`install_node.go`: On macOS, `brew install nodenv node-build`. On Linux, git clone nodenv + node-build plugin to `~/.nodenv`. Install Node 24.0.0 via `nodenv install`, set global, install npm globals.

- [ ] **Step 5: Port ruby**

`install_ruby.go`: On macOS, `brew install rbenv ruby-build` + build deps. On Linux, git clone rbenv + ruby-build + install build deps per PM. Install Ruby 3.4.1 via `rbenv install`, set global.

- [ ] **Step 6: Port zsh**

`install_zsh.go`: Install zsh package, run Oh My Zsh installer, git clone Pure prompt + plugins, symlink config files from `components/zsh/` to home dir.

- [ ] **Step 7: Update registry.go**

Replace bash entries with Go functions. Remove `BashInstall`/`BashUninstall` fields.

- [ ] **Step 8: Build and test**

Run: `cd ctdev && go build ./... && go test ./...`
Expected: All PASS

- [ ] **Step 9: Commit**

```bash
git add ctdev/component/install_fonts.go ctdev/component/install_docker.go ctdev/component/install_git.go ctdev/component/install_node.go ctdev/component/install_ruby.go ctdev/component/install_zsh.go ctdev/component/registry.go
git commit -m "feat: port fonts, docker, git, node, ruby, zsh to Go"
```

---

## Phase 3: Test Coverage

### Task 10: Extract and test update scanner parsers

**Files:**
- Modify: `ctdev/cmd/update.go` (extract parsing into named functions)
- Create: `ctdev/cmd/update_parse_test.go`

- [ ] **Step 1: Extract parseAPTUpgradable**

In `ctdev/cmd/update.go`, extract the line-parsing from `scanAPT` into:

```go
func parseAPTUpgradable(output string) []checklist.UpdateItem {
	var items []checklist.UpdateItem
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "[upgradable") {
			continue
		}
		parts := strings.SplitN(line, "/", 2)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		rest := parts[1]
		fields := strings.Fields(rest)
		newVer := ""
		currentVer := ""
		if len(fields) >= 2 {
			newVer = fields[1]
		}
		fromIdx := strings.Index(rest, "from: ")
		if fromIdx >= 0 {
			currentVer = strings.TrimRight(rest[fromIdx+6:], "]")
		}
		item := checklist.UpdateItem{
			Name:       name,
			Source:     "apt",
			CurrentVer: currentVer,
			NewVer:     newVer,
		}
		if strings.HasPrefix(name, "linux-") {
			item.IsKernel = true
		}
		items = append(items, item)
	}
	return items
}
```

Update `scanAPT` to call `parseAPTUpgradable(string(out))`.

Similarly extract: `parseBrewOutdated`, `parseFlatpakUpdates`.

- [ ] **Step 2: Write tests**

Create `ctdev/cmd/update_parse_test.go`:

```go
package cmd

import (
	"testing"
)

func TestParseAPTUpgradable(t *testing.T) {
	output := `Listing...
libglib2.0-0/noble-updates 2.80.5-1ubuntu1 amd64 [upgradable from: 2.80.0-6ubuntu3.1]
linux-image-6.8.0-50-generic/noble-updates 6.8.0-50.51 amd64 [upgradable from: 6.8.0-49.49]`

	items := parseAPTUpgradable(output)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "libglib2.0-0" {
		t.Errorf("expected libglib2.0-0, got %s", items[0].Name)
	}
	if items[0].Source != "apt" {
		t.Errorf("expected source apt, got %s", items[0].Source)
	}
	if !items[1].IsKernel {
		t.Error("expected linux-image to be flagged as kernel")
	}
}

func TestParseAPTUpgradableEmpty(t *testing.T) {
	items := parseAPTUpgradable("Listing...\n")
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestParseBrewOutdated(t *testing.T) {
	output := `node (21.0.0) < 22.0.0
python (3.12.0) < 3.13.0`

	items := parseBrewOutdated(output)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "node" {
		t.Errorf("expected node, got %s", items[0].Name)
	}
	if items[0].CurrentVer != "21.0.0" {
		t.Errorf("expected 21.0.0, got %s", items[0].CurrentVer)
	}
	if items[0].NewVer != "22.0.0" {
		t.Errorf("expected 22.0.0, got %s", items[0].NewVer)
	}
}

func TestParseBrewOutdatedEmpty(t *testing.T) {
	items := parseBrewOutdated("")
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestParseFlatpakUpdates(t *testing.T) {
	output := "org.mozilla.firefox\t132.0\norg.signal.Signal\t7.30.0"

	items := parseFlatpakUpdates(output)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "org.mozilla.firefox" {
		t.Errorf("expected firefox, got %s", items[0].Name)
	}
}

func TestParseNPMOutdatedJSON(t *testing.T) {
	content := `{
  "typescript": {
    "current": "5.3.0",
    "wanted": "5.4.0",
    "latest": "5.4.0",
    "dependent": "global",
    "location": "/usr/lib"
  }
}`
	items, err := parseNPMOutdatedJSON(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Name != "typescript" {
		t.Errorf("expected typescript, got %s", items[0].Name)
	}
	if items[0].CurrentVer != "5.3.0" {
		t.Errorf("expected 5.3.0, got %s", items[0].CurrentVer)
	}
}

func TestParseNPMOutdatedJSONEmpty(t *testing.T) {
	items, err := parseNPMOutdatedJSON("{}")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}
```

- [ ] **Step 3: Run tests**

Run: `cd ctdev && go test ./cmd/ -run TestParse -v`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add ctdev/cmd/update.go ctdev/cmd/update_parse_test.go
git commit -m "test: extract and test update scanner parsers"
```

---

### Task 11: Add progress uninstall mode test

**Files:**
- Modify: `ctdev/tui/progress/progress_test.go`

- [ ] **Step 1: Add uninstall mode test**

In `ctdev/tui/progress/progress_test.go`, add:

```go
func TestProgressModelUninstallMode(t *testing.T) {
	val := New([]string{"docker"}, ModeUninstall)
	m := &val

	view := m.viewProgress()
	if !strings.Contains(view, "Uninstalling") {
		t.Errorf("expected 'Uninstalling' in view, got: %s", view)
	}

	// Simulate completion
	updated, _ := m.Update(InstallDoneMsg{Name: "docker", Duration: time.Second})
	m = updated.(*Model)
	updated, _ = m.Update(AllDoneMsg{})
	m = updated.(*Model)

	summary := m.viewSummary()
	if !strings.Contains(summary, "Uninstall complete") {
		t.Errorf("expected 'Uninstall complete' in summary, got: %s", summary)
	}
}
```

Add `"strings"` to imports.

- [ ] **Step 2: Run test**

Run: `cd ctdev && go test ./tui/progress/ -run TestProgressModelUninstall -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add ctdev/tui/progress/progress_test.go
git commit -m "test: add progress uninstall mode test"
```

---

## Phase 4: UX/UI Improvements

### Task 12: Picker platform availability badges

**Files:**
- Modify: `ctdev/tui/picker/picker.go`
- Modify: `ctdev/cmd/install.go`

- [ ] **Step 1: Update picker to show all components**

In `ctdev/tui/picker/picker.go`, add `unsupported` field to `item`:

```go
type item struct {
	component   component.Component
	isCategory  bool
	category    component.Category
	collapsed   bool
	installed   bool
	unsupported bool
}
```

Update `New()` to accept full registry and mark unsupported items:

```go
func New(components []component.Component, installed map[string]bool, os component.OS, mode Mode) Model {
	groups := component.GroupByCategory(components)
	// ... build items, marking unsupported:
	for _, c := range comps {
		items = append(items, item{
			component:   c,
			installed:   installed[c.Name],
			unsupported: !c.SupportsOS(os),
		})
	}
}
```

Remove the `FilterByOS` call from `New()` — show all components.

- [ ] **Step 2: Update toggleSelected to skip unsupported**

In `toggleSelected()`:

```go
func (inst *Model) toggleSelected() {
	if inst.cursor >= 0 && inst.cursor < len(inst.items) {
		it := inst.items[inst.cursor]
		if !it.isCategory && !it.unsupported {
			// ... existing toggle logic
		}
	}
}
```

Also update `selectAll()` to skip unsupported items.

- [ ] **Step 3: Update View to dim unsupported items**

In the View function, when rendering a non-category item:

```go
if it.unsupported {
	osLabel := ""
	for _, s := range it.component.SupportedOS {
		osLabel = string(s)
	}
	indicator := styles.Dimmed.Render("⊘")
	name := lipgloss.NewStyle().Foreground(styles.Subtle).Width(14).Render(it.component.Name)
	desc := styles.Dimmed.Render(it.component.Description + " (" + osLabel + " only)")
	b.WriteString(fmt.Sprintf("  %s  %s %s\n", indicator, name, desc))
	continue
}
```

- [ ] **Step 4: Update cmd/install.go**

Pass full `comp.Registry` instead of filtering:

```go
m := picker.New(comp.Registry, installed, osType, picker.ModeInstall)
```

- [ ] **Step 5: Build and test**

Run: `cd ctdev && go build ./... && go test ./...`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add ctdev/tui/picker/picker.go ctdev/cmd/install.go
git commit -m "feat: show unsupported components as dimmed in picker"
```

---

### Task 13: Progress elapsed time during install

**Files:**
- Modify: `ctdev/tui/progress/progress.go`

- [ ] **Step 1: Add tick message and startTime per component**

In `ctdev/tui/progress/progress.go`, add to `ComponentState`:

```go
type ComponentState struct {
	Name      string
	Status    ComponentStatus
	Output    []string
	Error     string
	Duration  time.Duration
	StartedAt time.Time
}
```

Add a tick message type:

```go
type tickMsg time.Time
```

- [ ] **Step 2: Update Init to send ticks**

```go
func (inst *Model) Init() tea.Cmd {
	return tea.Batch(inst.spinner.Tick, tickEvery())
}

func tickEvery() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
```

- [ ] **Step 3: Handle tick in Update**

In the `Update` switch, add:

```go
case tickMsg:
	if !inst.done {
		return inst, tickEvery()
	}
```

Also in `InstallStartMsg` handler, set `StartedAt`:

```go
case InstallStartMsg:
	for i := range inst.components {
		if inst.components[i].Name == msg.Name {
			inst.components[i].Status = StatusRunning
			inst.components[i].StartedAt = time.Now()
			inst.current = i
			break
		}
	}
```

- [ ] **Step 4: Show elapsed time in viewProgress**

In `viewProgress`, for `StatusRunning`:

```go
case StatusRunning:
	elapsed := time.Since(c.StartedAt).Truncate(time.Second)
	b.WriteString(fmt.Sprintf("  %s %s %s\n",
		inst.spinner.View(),
		lipgloss.NewStyle().Bold(true).Foreground(styles.Bright).Render(c.Name),
		styles.Dimmed.Render(fmt.Sprintf("(%s)", elapsed)),
	))
```

- [ ] **Step 5: Build and test**

Run: `cd ctdev && go build ./... && go test ./tui/progress/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add ctdev/tui/progress/progress.go
git commit -m "feat: show elapsed time during component install"
```

---

### Task 14: Update scan progress indicator

**Files:**
- Modify: `ctdev/cmd/update.go`

- [ ] **Step 1: Add scan progress counter**

Replace the static "Scanning for updates..." in `runUpdate` with a live counter using a simple channel:

```go
func runUpdate(cmd *cobra.Command, args []string) error {
	if flagRefreshKeys {
		fmt.Println(styles.Dimmed.Render("Refreshing APT GPG keys..."))
		refreshAPTKeys(args)
	}

	items := scanAllWithProgress(context.Background())
	// ... rest unchanged
}

func scanAllWithProgress(ctx context.Context) []checklist.UpdateItem {
	var mu sync.Mutex
	var allItems []checklist.UpdateItem

	type scanner func(context.Context) ([]checklist.UpdateItem, error)
	scanners := []scanner{
		scanAPT, scanFlatpak, scanBrew, scanOhMyZsh, scanBun,
		scanNodeEnv, scanNPMGlobals, scanCtdev, scanGo, scanRuby,
		scanHelm, scanKubectl, scanTerraform,
	}
	total := len(scanners)
	done := make(chan struct{}, total)

	// Progress display goroutine
	go func() {
		count := 0
		for range done {
			count++
			fmt.Printf("\r%s", styles.Dimmed.Render(fmt.Sprintf("  Scanning for updates... (%d/%d sources checked)", count, total)))
		}
		fmt.Println()
	}()

	var wg sync.WaitGroup
	for _, fn := range scanners {
		fn := fn
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { done <- struct{}{} }()
			items, err := fn(ctx)
			if err == nil && len(items) > 0 {
				mu.Lock()
				allItems = append(allItems, items...)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	close(done)

	// Sort (same as before)
	sourceOrder := map[string]int{
		"apt": 0, "brew": 1, "flatpak": 2,
		"git": 3, "runtime": 4, "npm": 5,
		"cli": 6, "ctdev": 7,
	}
	sort.Slice(allItems, func(i, j int) bool {
		oi, oj := sourceOrder[allItems[i].Source], sourceOrder[allItems[j].Source]
		if oi != oj {
			return oi < oj
		}
		return allItems[i].Name < allItems[j].Name
	})

	return allItems
}
```

- [ ] **Step 2: Remove old scanAll function**

Delete the old `scanAll` function.

- [ ] **Step 3: Build**

Run: `cd ctdev && go build ./...`
Expected: Clean build

- [ ] **Step 4: Commit**

```bash
git add ctdev/cmd/update.go
git commit -m "feat: show scan progress counter during update check"
```

---

### Task 15: Info terminal-width-aware layout

**Files:**
- Modify: `ctdev/tui/info/info.go`

- [ ] **Step 1: Update Render to accept width**

Change `Render` signature:

```go
func Render(sysInfo platform.SystemInfo, version string, components []ComponentInfo, termWidth int) string {
```

Compute columns from width:

```go
colWidth := 20
cols := 2
if termWidth > 0 {
	if termWidth < 60 {
		cols = 1
	} else if termWidth > 100 {
		cols = 3
	}
}
```

Replace the hardcoded `i += 2` loop with:

```go
for i := 0; i < len(g.items); i += cols {
	b.WriteString("    ")
	for j := 0; j < cols && i+j < len(g.items); j++ {
		b.WriteString(renderComponentEntry(g.items[i+j], colWidth))
	}
	b.WriteString("\n")
}
```

- [ ] **Step 2: Update caller in cmd/info.go**

```go
import "os"
import "golang.org/x/term"

func runInfo(cmd *cobra.Command, args []string) error {
	// ...
	width := 80
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		width = w
	}
	output := tuiinfo.Render(sysInfo, version, components, width)
	// ...
}
```

Check if `golang.org/x/term` is already a dependency (it likely is via Bubble Tea). If not, use a simpler fallback: check `$COLUMNS` env var, default to 80.

- [ ] **Step 3: Build and test**

Run: `cd ctdev && go build ./... && go test ./...`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add ctdev/tui/info/info.go ctdev/cmd/info.go
git commit -m "feat: terminal-width-aware component layout in info command"
```

---

### Task 16: macOS setup message

**Files:**
- Modify: `ctdev/cmd/setup.go`

- [ ] **Step 1: Add message before macOS delegation**

In `runMacOSSetup()`, add at the top:

```go
func runMacOSSetup() error {
	fmt.Println(styles.Dimmed.Render("macOS setup uses the system configuration script."))
	fmt.Println()
	// ... existing bash delegation
}
```

Add import for styles package.

- [ ] **Step 2: Build**

Run: `cd ctdev && go build ./...`
Expected: Clean build

- [ ] **Step 3: Commit**

```bash
git add ctdev/cmd/setup.go
git commit -m "feat: add informational message for macOS setup delegation"
```

---

## Phase 5: Cleanup

### Task 17: Remove dead code and final verification

**Files:**
- Possibly delete: `ctdev/state/migrate.go`
- Modify: `CLAUDE.md` (update component count if changed)

- [ ] **Step 1: Check if migrate.go is still referenced**

Run: `cd ctdev && grep -r "MigrateOldMarkers\|migrate" --include="*.go" .`

If only `state/migrate.go` references itself, delete it. If `state/migrate_test.go` exists, delete that too.

- [ ] **Step 2: Run go vet**

Run: `cd ctdev && go vet ./...`
Expected: No issues

- [ ] **Step 3: Run full test suite**

Run: `cd ctdev && go test ./... -v`
Expected: All PASS

- [ ] **Step 4: Update CLAUDE.md component count**

If component count changed (was 35), update the count in `CLAUDE.md`.

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "chore: remove dead code and update docs"
```
