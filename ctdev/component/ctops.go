package component

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

const ctopsRepo = "ConnerTechnology/infrastructure"

var ctopsTagPattern = regexp.MustCompile(`^ctops/v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type ctopsRelease struct {
	TagName      string `json:"tagName"`
	IsDraft      bool   `json:"isDraft"`
	IsPrerelease bool   `json:"isPrerelease"`
}

// ctopsLatestTag ignores other tools' releases in the shared private repo.
// Compare numeric components rather than publication order or lexical tags.
func ctopsLatestTag(releases []ctopsRelease) (string, error) {
	latest := ""
	var best []string
	for _, r := range releases {
		parts := ctopsTagPattern.FindStringSubmatch(r.TagName)
		if r.IsDraft || r.IsPrerelease || parts == nil {
			continue
		}
		newer := best == nil
		if best != nil {
			for i := 1; i <= 3; i++ {
				if parts[i] != best[i] {
					// No integer conversion: even an oversized tag cannot overflow.
					newer = len(parts[i]) > len(best[i]) || len(parts[i]) == len(best[i]) && parts[i] > best[i]
					break
				}
			}
		}
		if newer {
			latest, best = r.TagName, parts
		}
	}
	if latest == "" {
		return "", fmt.Errorf("no stable ctops/vMAJOR.MINOR.PATCH release found in %s", ctopsRepo)
	}
	return latest, nil
}

func ctopsAsset(goos, arch string) (string, error) {
	if (goos != "linux" && goos != "darwin") || (arch != "amd64" && arch != "arm64") {
		return "", fmt.Errorf("ctops has no %s/%s build: %w", goos, arch, ErrUnsupportedOS)
	}
	return "ctops-" + goos + "-" + arch, nil
}

func ctopsVerifyChecksum(asset string, data, sums []byte) error {
	actual := sha256.Sum256(data)
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != asset {
			continue
		}
		if want != "" {
			return fmt.Errorf("duplicate checksum for %s in SHA256SUMS", asset)
		}
		decoded, err := hex.DecodeString(fields[0])
		if err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("invalid SHA256SUMS checksum for %s", asset)
		}
		want = strings.ToLower(fields[0])
	}
	if want == "" {
		return fmt.Errorf("no checksum for %s in SHA256SUMS", asset)
	}
	if got := hex.EncodeToString(actual[:]); got != want {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", asset, want, got)
	}
	return nil
}

func ctopsUserBin() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin", "ctops"), nil
}

func ctopsInstalled() bool {
	bin, err := ctopsUserBin()
	if err != nil {
		return false
	}
	info, err := os.Stat(bin)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func ctopsAuth(ctx context.Context) error {
	if err := sysutil.Run(ctx, execOpts(ExecOpts{Stdout: io.Discard}), "gh", "auth", "status"); err != nil {
		return fmt.Errorf("ctops releases are in the private %s repo; run gh auth login with access to that repo: %w", ctopsRepo, err)
	}
	return nil
}

func ctopsFetchTag(ctx context.Context) (string, error) {
	var out strings.Builder
	if err := sysutil.Run(ctx, execOpts(ExecOpts{Stdout: &out}), "gh", "release", "list", "--repo", ctopsRepo, "--json", "tagName,isDraft,isPrerelease", "--limit", "100"); err != nil {
		return "", fmt.Errorf("list ctops releases in private %s repo (gh needs repo access): %w", ctopsRepo, err)
	}
	var releases []ctopsRelease
	if err := json.Unmarshal([]byte(out.String()), &releases); err != nil {
		return "", fmt.Errorf("parse ctops releases: %w", err)
	}
	return ctopsLatestTag(releases)
}

// CTopsLatestVersion returns the newest stable ctops version for the update
// scan. gh supplies the credentials needed to read the private releases.
func CTopsLatestVersion(ctx context.Context) (string, error) {
	if err := ctopsAuth(ctx); err != nil {
		return "", err
	}
	tag, err := ctopsFetchTag(ctx)
	return strings.TrimPrefix(tag, "ctops/v"), err
}

// CTopsVersion reads the managed binary, like detection and uninstall, so a
// different ctops earlier on PATH cannot hide an update to this installation.
// It returns "" when the binary is missing or doesn't report a release version.
func CTopsVersion(ctx context.Context) string {
	bin, err := ctopsUserBin()
	if err != nil {
		return ""
	}
	var out strings.Builder
	if err := sysutil.Run(ctx, execOpts(ExecOpts{Stdout: &out}), bin, "--version"); err != nil {
		return ""
	}
	return parseCTopsVersion(out.String())
}

func parseCTopsVersion(out string) string {
	fields := strings.Fields(out)
	if len(fields) == 3 && fields[0] == "ctops" && fields[1] == "version" && ctopsTagPattern.MatchString("ctops/v"+fields[2]) {
		return fields[2]
	}
	return ""
}

func ctopsInstall(ctx context.Context, opts ExecOpts) error {
	if !opts.Force && alreadyInstalled("ctops") {
		fmt.Fprintln(opts.Stdout, "ctops already installed")
		return nil
	}
	return ctopsInstallVersion(ctx, execOpts(opts), "")
}

// CTopsUpdate reuses the verified installer, pinned to the version shown in
// the update checklist, even when a newer release appears during the scan.
func CTopsUpdate(ctx context.Context, o sysutil.Opts, version string) error {
	return ctopsInstallVersion(ctx, o, version)
}

func ctopsInstallVersion(ctx context.Context, o sysutil.Opts, version string) error {
	asset, err := ctopsAsset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	dest, err := ctopsUserBin()
	if err != nil {
		return err
	}
	tag := "ctops/v" + version
	if version != "" && !ctopsTagPattern.MatchString(tag) {
		return fmt.Errorf("invalid ctops version %q", version)
	}
	if o.DryRun {
		if version == "" {
			tag = "latest stable ctops/vMAJOR.MINOR.PATCH"
		}
		fmt.Fprintf(o.Stdout, "[dry-run] gh download %s from private %s (%s and SHA256SUMS), verify sha256, install to %s\n", tag, ctopsRepo, asset, dest)
		return nil
	}
	if err := ctopsAuth(ctx); err != nil {
		return err
	}
	if version == "" {
		tag, err = ctopsFetchTag(ctx)
		if err != nil {
			return err
		}
	}
	tmp, err := os.MkdirTemp("", "ctdev-ctops-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := sysutil.Run(ctx, o, "gh", "release", "download", tag, "--repo", ctopsRepo, "--pattern", asset, "--pattern", "SHA256SUMS", "--dir", tmp); err != nil {
		return fmt.Errorf("download ctops %s: %w", tag, err)
	}
	src := filepath.Join(tmp, asset)
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	sums, err := os.ReadFile(filepath.Join(tmp, "SHA256SUMS"))
	if err != nil {
		return err
	}
	if err := ctopsVerifyChecksum(asset, data, sums); err != nil {
		return err
	}
	if err := sysutil.InstallUserBinary(ctx, o, src, dest); err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "ctops %s installed\n", strings.TrimPrefix(tag, "ctops/v"))
	return nil
}

func ctopsUninstall(ctx context.Context, opts ExecOpts) error {
	dest, err := ctopsUserBin()
	if err != nil {
		return err
	}
	fmt.Fprintln(opts.Stdout, "Removing ctops...")
	return sysutil.Run(ctx, execOpts(opts), "rm", "-f", dest)
}
