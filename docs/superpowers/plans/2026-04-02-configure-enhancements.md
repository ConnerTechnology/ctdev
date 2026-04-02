# Configure Enhancements — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enhance `ctdev configure` with SSH key selection TUI, GitHub key upload, AWS profile picker, and colorls alias auto-offer.

**Architecture:** New `tui/configure/` package for the git config TUI. New `configure aws` subcommand. Shared `sysutil.AppendToExportsLocal` helper for writing to exports.local.zsh. Modify ruby installer for colorls alias prompt.

**Tech Stack:** Go, Bubble Tea v2, Cobra, os/exec for git/gh/ssh-keygen

**Spec:** `docs/superpowers/specs/2026-04-02-configure-enhancements-design.md`

---

## Task 1: Add SSH key scanning and exports.local.zsh helper

**Files:**
- Create: `ctdev/sysutil/sshkeys.go`
- Create: `ctdev/sysutil/sshkeys_test.go`
- Create: `ctdev/sysutil/exports_local.go`
- Create: `ctdev/sysutil/exports_local_test.go`

- [ ] **Step 1: Write SSH key scanning tests**

Create `ctdev/sysutil/sshkeys_test.go`:

```go
package sysutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindSSHPublicKeys(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "id_ed25519.pub"), []byte("ssh-ed25519 AAAA... user@host"), 0644)
	os.WriteFile(filepath.Join(dir, "id_rsa.pub"), []byte("ssh-rsa AAAA... user@host"), 0644)
	os.WriteFile(filepath.Join(dir, "id_ed25519"), []byte("private key"), 0600)
	os.WriteFile(filepath.Join(dir, "known_hosts"), []byte("host key"), 0644)

	keys := FindSSHPublicKeysIn(dir)
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
}

func TestFindSSHPublicKeysEmpty(t *testing.T) {
	dir := t.TempDir()
	keys := FindSSHPublicKeysIn(dir)
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}
```

- [ ] **Step 2: Write exports.local.zsh helper tests**

Create `ctdev/sysutil/exports_local_test.go`:

```go
package sysutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetLineInFileAppends(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "exports.local.zsh")
	os.WriteFile(f, []byte("# existing content\n"), 0644)

	err := SetLineInFile(f, "AWS_PROFILE", "export AWS_PROFILE=test-profile")
	if err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(f)
	if !strings.Contains(string(got), "export AWS_PROFILE=test-profile") {
		t.Errorf("expected AWS_PROFILE line, got: %s", string(got))
	}
	if !strings.Contains(string(got), "# existing content") {
		t.Error("existing content should be preserved")
	}
}

func TestSetLineInFileReplaces(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "exports.local.zsh")
	os.WriteFile(f, []byte("# header\nexport AWS_PROFILE=old-value\n# footer\n"), 0644)

	err := SetLineInFile(f, "AWS_PROFILE", "export AWS_PROFILE=new-value")
	if err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(f)
	content := string(got)
	if !strings.Contains(content, "export AWS_PROFILE=new-value") {
		t.Errorf("expected new value, got: %s", content)
	}
	if strings.Contains(content, "old-value") {
		t.Error("old value should be replaced")
	}
	if !strings.Contains(content, "# header") || !strings.Contains(content, "# footer") {
		t.Error("surrounding content should be preserved")
	}
}

func TestSetLineInFileCreatesFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "new.zsh")

	err := SetLineInFile(f, "AWS_PROFILE", "export AWS_PROFILE=test")
	if err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(f)
	if !strings.Contains(string(got), "export AWS_PROFILE=test") {
		t.Errorf("expected line in new file, got: %s", string(got))
	}
}

func TestAppendLineIfMissing(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.zsh")
	os.WriteFile(f, []byte("# stuff\n"), 0644)

	added, _ := AppendLineIfMissing(f, "alias lc='colorls -lA --sd'")
	if !added {
		t.Error("expected line to be added")
	}

	added, _ = AppendLineIfMissing(f, "alias lc='colorls -lA --sd'")
	if added {
		t.Error("expected line to be skipped (already present)")
	}
}
```

- [ ] **Step 3: Implement sshkeys.go**

Create `ctdev/sysutil/sshkeys.go`:

```go
package sysutil

import (
	"os"
	"path/filepath"
	"strings"
)

type SSHPublicKey struct {
	Path    string // e.g. "/home/user/.ssh/id_ed25519.pub"
	Name    string // e.g. "id_ed25519.pub"
	KeyType string // e.g. "ssh-ed25519"
}

func FindSSHPublicKeys() []SSHPublicKey {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return FindSSHPublicKeysIn(filepath.Join(home, ".ssh"))
}

func FindSSHPublicKeysIn(dir string) []SSHPublicKey {
	matches, err := filepath.Glob(filepath.Join(dir, "*.pub"))
	if err != nil {
		return nil
	}
	var keys []SSHPublicKey
	for _, path := range matches {
		name := filepath.Base(path)
		keyType := ""
		if data, err := os.ReadFile(path); err == nil {
			parts := strings.Fields(string(data))
			if len(parts) > 0 {
				keyType = parts[0]
			}
		}
		keys = append(keys, SSHPublicKey{
			Path:    path,
			Name:    name,
			KeyType: keyType,
		})
	}
	return keys
}
```

- [ ] **Step 4: Implement exports_local.go**

Create `ctdev/sysutil/exports_local.go`:

```go
package sysutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExportsLocalPath returns the path to exports.local.zsh
func ExportsLocalPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".oh-my-zsh", "custom", "exports.local.zsh")
}

// SetLineInFile finds a line containing `key` and replaces it, or appends if not found.
func SetLineInFile(path, key, newLine string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(path, []byte(newLine+"\n"), 0644)
		}
		return err
	}

	lines := strings.Split(string(content), "\n")
	found := false
	for i, line := range lines {
		if strings.Contains(line, key) && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			lines[i] = newLine
			found = true
			break
		}
	}

	if !found {
		// Append with blank line separator if file doesn't end with one
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, newLine)
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// AppendLineIfMissing appends a line to a file if it's not already present.
// Returns true if the line was added, false if already present.
func AppendLineIfMissing(path, line string) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	if strings.Contains(string(content), line) {
		return false, nil
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, line)
	return err == nil, err
}
```

- [ ] **Step 5: Run tests**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev
go test ./sysutil/ -run "TestFindSSH|TestSetLine|TestAppendLine" -v
```

- [ ] **Step 6: Commit**

```bash
git add ctdev/sysutil/sshkeys.go ctdev/sysutil/sshkeys_test.go ctdev/sysutil/exports_local.go ctdev/sysutil/exports_local_test.go
git commit -m "feat: add SSH key scanner and exports.local.zsh helpers"
```

---

## Task 2: Add AWS profile parser

**Files:**
- Create: `ctdev/sysutil/awsconfig.go`
- Create: `ctdev/sysutil/awsconfig_test.go`

- [ ] **Step 1: Write tests**

Create `ctdev/sysutil/awsconfig_test.go`:

```go
package sysutil

import (
	"testing"
)

func TestParseAWSProfiles(t *testing.T) {
	content := `[profile developer-access-767828768904]
sso_session = BlueWaterAutonomy
sso_account_id = 767828768904
sso_role_name = developer-access
region = us-east-2

[sso-session BlueWaterAutonomy]
sso_start_url = https://d-9067c20008.awsapps.com/start
sso_region = us-east-1

[default]
region = us-east-1
`
	profiles := ParseAWSProfiles(content)
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles (developer-access + default), got %d: %v", len(profiles), profiles)
	}
	if profiles[0] != "developer-access-767828768904" {
		t.Errorf("expected developer-access-767828768904, got %s", profiles[0])
	}
	if profiles[1] != "default" {
		t.Errorf("expected default, got %s", profiles[1])
	}
}

func TestParseAWSProfilesEmpty(t *testing.T) {
	profiles := ParseAWSProfiles("")
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(profiles))
	}
}

func TestParseAWSProfilesSkipsSSOSessions(t *testing.T) {
	content := `[sso-session MySession]
sso_start_url = https://example.com
`
	profiles := ParseAWSProfiles(content)
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles (sso-session only), got %d", len(profiles))
	}
}
```

- [ ] **Step 2: Implement awsconfig.go**

Create `ctdev/sysutil/awsconfig.go`:

```go
package sysutil

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	profileRegex = regexp.MustCompile(`^\[profile\s+(.+)\]$`)
	defaultRegex = regexp.MustCompile(`^\[default\]$`)
)

// ParseAWSProfiles extracts profile names from AWS config file content.
// Returns profile names in order found. Skips [sso-session] sections.
func ParseAWSProfiles(content string) []string {
	var profiles []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if m := profileRegex.FindStringSubmatch(line); len(m) == 2 {
			profiles = append(profiles, m[1])
		} else if defaultRegex.MatchString(line) {
			profiles = append(profiles, "default")
		}
	}
	return profiles
}

// ReadAWSProfiles reads and parses ~/.aws/config.
func ReadAWSProfiles() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(home, ".aws", "config"))
	if err != nil {
		return nil, err
	}
	return ParseAWSProfiles(string(data)), nil
}
```

- [ ] **Step 3: Run tests**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev
go test ./sysutil/ -run TestParseAWS -v
```

- [ ] **Step 4: Commit**

```bash
git add ctdev/sysutil/awsconfig.go ctdev/sysutil/awsconfig_test.go
git commit -m "feat: add AWS config profile parser"
```

---

## Task 3: Rewrite `ctdev configure git` with TUI

**Files:**
- Modify: `ctdev/cmd/configure.go`

- [ ] **Step 1: Rewrite runConfigureGit**

Read the current `ctdev/cmd/configure.go` first. Rewrite `runConfigureGit` to:

1. **Scope detection**: check if `.git/` exists in CWD. If yes and no `--local`/`--global` flag, prompt with a simple "Global / This repo only" choice. If no `.git/`, default to global.

2. **Name/Email**: same text prompts as now (keep `fmt.Scanln`), pre-filled with current values.

3. **SSH key picker**: After name/email, scan `~/.ssh/*.pub` using `sysutil.FindSSHPublicKeys()`. Present a numbered list:
   ```
   SSH Signing Key:
     1) ~/.ssh/id_ed25519.pub (ssh-ed25519)
     2) Generate new ed25519 key
     3) Enter custom path
     4) Skip (no signing)
   Select [1]: 
   ```
   Use simple numbered prompt (not full Bubble Tea picker — this is a configure flow, not a component picker). Default to first key if one exists.

4. **Key generation**: If "Generate new" selected, run `ssh-keygen -t ed25519 -C <email>` with stdin/stdout passthrough for passphrase prompt.

5. **GitHub upload**: If key selected/generated and `gh auth status` succeeds, prompt "Add this key to GitHub? [Y/n]". If yes, run `gh ssh-key add <path> --title "ctdev-<hostname>"`. If `gh` not available, print manual instructions.

6. **Apply**: Set `user.name`, `user.email`, `user.signingKey` via `git config`. Also set `commit.gpgsign true` if signing key is configured.

Also add `--signing-key` flag to the command.

Update `showGitConfig` to also display `user.signingKey` and `commit.gpgsign`.

- [ ] **Step 2: Build and test**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev
go build ./... && go test ./...
```

- [ ] **Step 3: Commit**

```bash
git add ctdev/cmd/configure.go
git commit -m "feat: enhance configure git with SSH key picker and GitHub integration"
```

---

## Task 4: Add `ctdev configure aws`

**Files:**
- Modify: `ctdev/cmd/configure.go`

- [ ] **Step 1: Add AWS subcommand**

Add to `ctdev/cmd/configure.go`:

New flag: `flagAWSProfile string`

New command:
```go
var configureAWSCmd = &cobra.Command{
    Use:   "aws",
    Short: "Configure AWS profile",
    Long:  "Set the default AWS_PROFILE in your shell environment.",
    RunE:  runConfigureAWS,
}
```

Register in `init()`: `configureCmd.AddCommand(configureAWSCmd)`

`runConfigureAWS` implementation:
1. If `--profile` flag provided, use it directly
2. Otherwise, call `sysutil.ReadAWSProfiles()`
3. If no profiles found, print error: "No AWS profiles found. Configure AWS CLI first: aws configure sso"
4. If profiles found, show numbered list picker (same style as SSH key picker)
5. Call `sysutil.SetLineInFile(sysutil.ExportsLocalPath(), "AWS_PROFILE", "export AWS_PROFILE="+selected)`
6. Print success message with instructions to source the file or restart shell

- [ ] **Step 2: Build**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add ctdev/cmd/configure.go
git commit -m "feat: add configure aws command with profile picker"
```

---

## Task 5: Add colorls alias offer to ruby installer

**Files:**
- Modify: `ctdev/component/ruby.go`

- [ ] **Step 1: Add colorls gem install and alias offer**

At the end of `rubyInstall`, after `rbenv global`, add:

```go
// Install colorls gem
fmt.Fprintln(opts.Stdout, "Installing colorls gem...")
gemBin := "gem"
if p.PackageManager != "brew" {
    gemBin = filepath.Join(rbenvDir, "shims", "gem")
}
if err := sysutil.Run(o, gemBin, "install", "colorls"); err != nil {
    fmt.Fprintf(opts.Stdout, "warning: could not install colorls: %v\n", err)
} else if !isBatchMode() {
    exportsPath := sysutil.ExportsLocalPath()
    aliasLine := "alias lc='colorls -lA --sd'"
    fmt.Print("  Add alias 'lc' for colorls to your shell? [Y/n] ")
    var answer string
    fmt.Scanln(&answer)
    if answer == "" || strings.HasPrefix(strings.ToLower(answer), "y") {
        if added, err := sysutil.AppendLineIfMissing(exportsPath, aliasLine); err != nil {
            fmt.Fprintf(opts.Stdout, "warning: could not add alias: %v\n", err)
        } else if added {
            fmt.Fprintln(opts.Stdout, styles.Success.Render("  Added alias lc='colorls -lA --sd' to exports.local.zsh"))
        } else {
            fmt.Fprintln(opts.Stdout, "  Alias already exists in exports.local.zsh")
        }
    }
}
```

Note: The `isBatchMode()` function is in `cmd/` package, not accessible from `component/`. Instead, check if `opts.Stdout` is connected to a terminal, or add a simple stdin check. Simplest: just check if we can read from stdin. Actually, the component function doesn't have access to `isBatchMode()`. Instead, use `opts.Force` as the signal — if Force is set (typically from batch/CI), skip the prompt. Or better: only prompt if stdin is a terminal. Use `term.IsTerminal(int(os.Stdin.Fd()))`. If `golang.org/x/term` isn't available, use a simpler check: `os.Stdin.Stat()` and check `Mode()&os.ModeCharDevice`.

```go
if !isInteractive() {
    // batch mode, skip prompt
} else {
    // prompt
}
```

Add helper:
```go
func isInteractive() bool {
    fi, err := os.Stdin.Stat()
    if err != nil {
        return false
    }
    return fi.Mode()&os.ModeCharDevice != 0
}
```

- [ ] **Step 2: Build and test**

```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev
go build ./... && go test ./...
```

- [ ] **Step 3: Commit**

```bash
git add ctdev/component/ruby.go
git commit -m "feat: install colorls gem and offer lc alias during ruby install"
```

---

## Task 6: Update exports.local.zsh template

**Files:**
- Modify: `ctdev/component/configs/zsh/exports.local.zsh`

- [ ] **Step 1: Update template**

Replace the content of `ctdev/component/configs/zsh/exports.local.zsh`:

```zsh
# Local/personal environment variables
# This file is per-machine — edit freely, it won't be overwritten.
#
# Set via ctdev configure:
#   ctdev configure aws    — set AWS_PROFILE
#   ctdev configure git    — set git identity and signing key
#
# Personal aliases (e.g., after installing ruby + colorls gem):
#   alias lc='colorls -lA --sd'
```

- [ ] **Step 2: Commit**

```bash
git add ctdev/component/configs/zsh/exports.local.zsh
git commit -m "docs: update exports.local.zsh template with configure examples"
```
