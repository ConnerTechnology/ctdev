// Update apply phase: updateStep construction and the per-tool updaters
// (helm, kubectl, terraform, go) with their download/verify helpers.
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/checklist"
)

// updateStep is one unit of the update apply phase — a labeled action run
// under the shared progress TUI (or the batch fallback). Steps write progress
// to o.Stdout only; anything printed elsewhere would corrupt the TUI.
type updateStep struct {
	name string
	run  func(ctx context.Context, o sysutil.Opts) error
}

func executeUpdates(ctx context.Context, items []checklist.UpdateItem) error {
	return runUpdateSteps(ctx, buildUpdateSteps(items))
}

// buildUpdateSteps translates the selected update items into ordered steps.
// Package managers batch into one step per manager; runtimes, CLI tools, and
// docker stacks update independently, so each gets its own step and a failure
// in one never blocks the rest.
func buildUpdateSteps(items []checklist.UpdateItem) []updateStep {
	bySource := make(map[string][]checklist.UpdateItem)
	for _, item := range items {
		bySource[item.Source] = append(bySource[item.Source], item)
	}

	var steps []updateStep

	if pkgs := bySource["apt"]; len(pkgs) > 0 {
		names := itemNames(pkgs)
		steps = append(steps, updateStep{
			name: fmt.Sprintf("apt (%d packages)", len(names)),
			run: func(ctx context.Context, o sysutil.Opts) error {
				args := append([]string{"install", "--only-upgrade", "-y"}, names...)
				return sysutil.SudoRun(ctx, o, "apt", args...)
			},
		})
	}

	for _, item := range bySource["flatpak"] {
		steps = append(steps, updateStep{
			name: "flatpak: " + item.Name,
			run: func(ctx context.Context, o sysutil.Opts) error {
				return sysutil.Run(ctx, o, "flatpak", "update", "-y", item.Name)
			},
		})
	}

	if pkgs := bySource["brew"]; len(pkgs) > 0 {
		names := itemNames(pkgs)
		steps = append(steps, updateStep{
			name: fmt.Sprintf("brew (%d packages)", len(names)),
			run: func(ctx context.Context, o sysutil.Opts) error {
				return sysutil.Run(ctx, o, "brew", append([]string{"upgrade"}, names...)...)
			},
		})
	}

	if pkgs := bySource["brew-cask"]; len(pkgs) > 0 {
		names := itemNames(pkgs)
		steps = append(steps, updateStep{
			name: fmt.Sprintf("brew casks (%d)", len(names)),
			run: func(ctx context.Context, o sysutil.Opts) error {
				return sysutil.Run(ctx, o, "brew", append([]string{"upgrade", "--cask"}, names...)...)
			},
		})
	}

	if len(bySource["git"]) > 0 { // only one oh-my-zsh
		steps = append(steps, updateStep{
			name: "oh-my-zsh",
			run: func(ctx context.Context, o sysutil.Opts) error {
				omzDir := os.ExpandEnv("$HOME/.oh-my-zsh")
				return sysutil.Run(ctx, o, "git", "-C", omzDir, "pull", "--rebase", "--quiet")
			},
		})
	}

	for _, item := range bySource["runtime"] {
		if step, ok := runtimeUpdateStep(item); ok {
			steps = append(steps, step)
		}
	}

	if pkgs := bySource["npm"]; len(pkgs) > 0 {
		names := itemNames(pkgs)
		steps = append(steps, updateStep{
			name: fmt.Sprintf("npm globals (%d)", len(names)),
			run: func(ctx context.Context, o sysutil.Opts) error {
				return sysutil.Run(ctx, o, "npm", append([]string{"update", "-g"}, names...)...)
			},
		})
	}

	for _, item := range bySource["ctdev"] {
		steps = append(steps, updateStep{
			name: fmt.Sprintf("ctdev %s → %s", item.CurrentVer, item.NewVer),
			run: func(ctx context.Context, o sysutil.Opts) error {
				return sysutil.Run(ctx, o, "bash", "-c",
					"curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/ctdev/main/install.sh | bash")
			},
		})
	}

	for _, item := range bySource["cli"] {
		if step, ok := cliUpdateStep(item); ok {
			steps = append(steps, step)
		}
	}

	// Docker compose stacks (pihole, caddy, beszel, portainer, ...)
	for _, item := range bySource["docker"] {
		steps = append(steps, updateStep{
			name: "docker: " + item.Name,
			run: func(ctx context.Context, o sysutil.Opts) error {
				return updateDockerStack(ctx, o, item)
			},
		})
	}

	return steps
}

// runtimeUpdateStep builds the step for a runtime update (bun/node/go/ruby).
func runtimeUpdateStep(item checklist.UpdateItem) (updateStep, bool) {
	name := fmt.Sprintf("%s %s → %s", item.Name, item.CurrentVer, item.NewVer)
	switch item.Name {
	case "bun":
		return updateStep{name: name, run: func(ctx context.Context, o sysutil.Opts) error {
			return sysutil.Run(ctx, o, "bun", "upgrade")
		}}, true
	case "node (nodenv)":
		return updateStep{name: name, run: func(ctx context.Context, o sysutil.Opts) error {
			if err := sysutil.Run(ctx, o, "nodenv", "install", "--skip-existing", item.NewVer); err != nil {
				return fmt.Errorf("nodenv install: %w", err)
			}
			return sysutil.Run(ctx, o, "nodenv", "global", item.NewVer)
		}}, true
	case "go":
		return updateStep{name: name, run: func(ctx context.Context, o sysutil.Opts) error {
			if o.DryRun {
				fmt.Fprintf(o.Stdout, "  [dry-run] download and install go%s\n", item.NewVer)
				return nil
			}
			return updateGo(ctx, o, item.NewVer)
		}}, true
	case "ruby":
		return updateStep{name: name, run: func(ctx context.Context, o sysutil.Opts) error {
			if _, err := exec.LookPath("rbenv"); err != nil {
				return fmt.Errorf("rbenv not found — install ruby %s manually", item.NewVer)
			}
			if err := sysutil.Run(ctx, o, "rbenv", "install", "--skip-existing", item.NewVer); err != nil {
				return fmt.Errorf("rbenv install: %w", err)
			}
			return sysutil.Run(ctx, o, "rbenv", "global", item.NewVer)
		}}, true
	}
	return updateStep{}, false
}

// cliUpdateStep builds the step for a CLI-tool update (helm/kubectl/terraform).
func cliUpdateStep(item checklist.UpdateItem) (updateStep, bool) {
	name := fmt.Sprintf("%s %s → %s", item.Name, item.CurrentVer, item.NewVer)
	switch item.Name {
	case "helm":
		return updateStep{name: name, run: func(ctx context.Context, o sysutil.Opts) error {
			if o.DryRun {
				fmt.Fprintf(o.Stdout, "  [dry-run] download helm v%s and install to /usr/local/bin\n", item.NewVer)
				return nil
			}
			return updateHelm(ctx, o, item.NewVer)
		}}, true
	case "kubectl":
		return updateStep{name: name, run: func(ctx context.Context, o sysutil.Opts) error {
			if o.DryRun {
				fmt.Fprintf(o.Stdout, "  [dry-run] download kubectl v%s and install to /usr/local/bin\n", item.NewVer)
				return nil
			}
			return updateKubectl(ctx, o, item.NewVer)
		}}, true
	case "terraform":
		return updateStep{name: name, run: func(ctx context.Context, o sysutil.Opts) error {
			if o.DryRun {
				fmt.Fprintf(o.Stdout, "  [dry-run] download terraform %s and install\n", item.NewVer)
				return nil
			}
			return updateTerraform(ctx, o, item.NewVer)
		}}, true
	}
	return updateStep{}, false
}

// resolveInstallDest returns the existing install path for bin (from `which`)
// or a sensible default under /usr/local/bin.
func resolveInstallDest(ctx context.Context, bin, fallback string) string {
	out, _ := exec.CommandContext(ctx, "which", bin).Output()
	dest := strings.TrimSpace(string(out))
	if dest == "" {
		return fallback
	}
	return dest
}

// downloadVerifiedArchive downloads archiveURL into a fresh temp dir, fetches
// checksumURL, verifies the archive against the hash that pickHash extracts
// from the checksum file, then calls install with the temp dir and archive
// path. The temp dir is always cleaned up. Each caller keeps its own checksum
// format (bare hash, "hash file", or SHA256SUMS) and extraction step, so this
// only collapses the shared download/verify boilerplate.
func downloadVerifiedArchive(ctx context.Context, archiveURL, checksumURL string, pickHash func(checksumContents string) (string, error), install func(tmpDir, archivePath string) error) error {
	tmpDir, err := os.MkdirTemp("", "ctdev-dl-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, filepath.Base(archiveURL))
	if err := sysutil.DownloadFile(ctx, archiveURL, archivePath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	csPath := filepath.Join(tmpDir, "checksum")
	if err := sysutil.DownloadFile(ctx, checksumURL, csPath); err != nil {
		return fmt.Errorf("download checksum failed: %w", err)
	}
	csData, err := os.ReadFile(csPath)
	if err != nil {
		return fmt.Errorf("read checksum: %w", err)
	}
	expected, err := pickHash(string(csData))
	if err != nil {
		return err
	}
	if err := sysutil.VerifyChecksum(archivePath, expected); err != nil {
		return fmt.Errorf("checksum mismatch: %w", err)
	}
	return install(tmpDir, archivePath)
}

func updateHelm(ctx context.Context, o sysutil.Opts, ver string) error {
	goos := detectUpdateOS()
	goarch := detectUpdateArch()
	url := fmt.Sprintf("https://get.helm.sh/helm-v%s-%s-%s.tar.gz", ver, goos, goarch)
	return downloadVerifiedArchive(ctx, url, url+".sha256sum",
		func(cs string) (string, error) {
			// helm publishes the bare hash (optionally "hash  filename").
			parts := strings.Fields(strings.TrimSpace(cs))
			if len(parts) == 0 {
				return "", fmt.Errorf("empty checksum file")
			}
			return parts[0], nil
		},
		func(tmpDir, archivePath string) error {
			if err := sysutil.Run(ctx, o, "tar", "-xzf", archivePath, "-C", tmpDir); err != nil {
				return fmt.Errorf("extract failed: %w", err)
			}
			dest := resolveInstallDest(ctx, "helm", "/usr/local/bin/helm")
			return sysutil.SudoRun(ctx, o, "mv", fmt.Sprintf("%s/%s-%s/helm", tmpDir, goos, goarch), dest)
		})
}

func updateKubectl(ctx context.Context, o sysutil.Opts, ver string) error {
	goos := detectUpdateOS()
	goarch := detectUpdateArch()
	url := fmt.Sprintf("https://dl.k8s.io/release/v%s/bin/%s/%s/kubectl", ver, goos, goarch)
	return downloadVerifiedArchive(ctx, url, url+".sha256",
		func(cs string) (string, error) { return strings.TrimSpace(cs), nil },
		func(tmpDir, archivePath string) error {
			if err := os.Chmod(archivePath, 0755); err != nil {
				return err
			}
			dest := resolveInstallDest(ctx, "kubectl", "/usr/local/bin/kubectl")
			return sysutil.SudoRun(ctx, o, "mv", archivePath, dest)
		})
}

func updateTerraform(ctx context.Context, o sysutil.Opts, ver string) error {
	goos := detectUpdateOS()
	goarch := detectUpdateArch()
	zipName := fmt.Sprintf("terraform_%s_%s_%s.zip", ver, goos, goarch)
	url := fmt.Sprintf("https://releases.hashicorp.com/terraform/%s/%s", ver, zipName)
	csURL := fmt.Sprintf("https://releases.hashicorp.com/terraform/%s/terraform_%s_SHA256SUMS", ver, ver)
	return downloadVerifiedArchive(ctx, url, csURL,
		func(cs string) (string, error) {
			// SHA256SUMS has lines like "hash  terraform_1.2.3_linux_amd64.zip".
			for _, line := range strings.Split(cs, "\n") {
				if strings.Contains(line, zipName) {
					if parts := strings.Fields(line); len(parts) >= 1 {
						return parts[0], nil
					}
				}
			}
			return "", fmt.Errorf("checksum not found for %s", zipName)
		},
		func(tmpDir, archivePath string) error {
			if err := sysutil.Run(ctx, o, "unzip", "-o", archivePath, "-d", tmpDir); err != nil {
				return fmt.Errorf("unzip failed: %w", err)
			}
			dest := resolveInstallDest(ctx, "terraform", "/usr/local/bin/terraform")
			return sysutil.SudoRun(ctx, o, "mv", filepath.Join(tmpDir, "terraform"), dest)
		})
}

func updateGo(ctx context.Context, o sysutil.Opts, ver string) error {
	goos := detectUpdateOS()
	goarch := detectUpdateArch()
	url := fmt.Sprintf("https://go.dev/dl/go%s.%s-%s.tar.gz", ver, goos, goarch)

	// Fetch the release manifest first so we can verify the tarball checksum
	// before touching the existing /usr/local/go install.
	releases, err := fetchGoReleases(ctx)
	if err != nil {
		return fmt.Errorf("fetch go release manifest: %w", err)
	}
	expectedSHA := goArchiveSHA256(releases, ver, goos, goarch)
	if expectedSHA == "" {
		return fmt.Errorf("no published sha256 for go%s %s/%s", ver, goos, goarch)
	}

	tmpFile, err := os.CreateTemp("", "ctdev-go-*.tar.gz")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)
	if err := sysutil.DownloadFile(ctx, url, tmpPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	if err := sysutil.VerifyChecksum(tmpPath, expectedSHA); err != nil {
		return fmt.Errorf("go checksum mismatch: %w", err)
	}
	// Extract to a temp directory first so we don't destroy the existing install
	// if the archive is corrupted or extraction fails.
	tmpDir, err := os.MkdirTemp("", "ctdev-go-extract-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	if err := sysutil.Run(ctx, o, "tar", "-C", tmpDir, "-xzf", tmpPath); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}
	// Verify the extraction produced a go directory with a binary
	if _, err := os.Stat(filepath.Join(tmpDir, "go", "bin", "go")); err != nil {
		return fmt.Errorf("extracted archive missing go binary: %w", err)
	}
	// Safe to replace now
	if err := sysutil.SudoRun(ctx, o, "rm", "-rf", "/usr/local/go"); err != nil {
		return fmt.Errorf("remove old go failed: %w", err)
	}
	if err := sysutil.SudoRun(ctx, o, "mv", filepath.Join(tmpDir, "go"), "/usr/local/go"); err != nil {
		return fmt.Errorf("install new go failed: %w", err)
	}
	return nil
}

func detectUpdateOS() string {
	return runtime.GOOS
}

func detectUpdateArch() string {
	return runtime.GOARCH
}
