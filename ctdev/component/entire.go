package component

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// EntireRepo is where Entire publishes its releases.
const EntireRepo = "entireio/cli"

// entireCask is the tap-qualified cask, so `brew install` taps entireio/tap on
// its own. The cask installs both binaries and isn't marked auto_updates, so
// `ctdev update` upgrades it through the brew cask scan.
const entireCask = "entireio/tap/entire"

// entireBinaries are both executables in the release archive. git-remote-entire
// is the helper git runs for entire:// remotes; it has to match the CLI.
var entireBinaries = []string{"entire", "git-remote-entire"}

// entireOrigin is somewhere that serves Entire's release files as
// <base>/v<version>/<file>.
type entireOrigin struct {
	name string
	base string
}

// entireOrigins are tried in order. install.entire.com mirrors every release
// since v0.12.0, so an install still works while GitHub is down; Entire's own
// install.sh uses the same two origins.
var entireOrigins = []entireOrigin{
	{name: "GitHub", base: "https://github.com/entireio/cli/releases/download"},
	{name: "install.entire.com", base: "https://install.entire.com"},
}

// entireChannelURL is the mirror's pointer to the latest stable release, read
// when GitHub can't say what the latest is.
const entireChannelURL = "https://install.entire.com/channels/stable.json"

// entireStableVersion is a stable release number. It ends up in a URL, so
// anything else (a nightly, a stray path) is refused.
var entireStableVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// errEntireOriginUnavailable means one origin couldn't serve the release, so
// the next one is tried. Any other error, a checksum mismatch above all, stops
// the install: bytes that disagree with their own origin's checksums mean that
// origin is broken or hostile, and trying elsewhere would hide it.
var errEntireOriginUnavailable = errors.New("origin unavailable")

// entireBinDir is where the Linux binaries go: ~/.local/bin, which is where
// Entire's own installer puts them and where ctdev and restic live, so neither
// install nor update needs root. A hand install there is the same files in the
// same place, so ctdev takes it over instead of making a second copy.
func entireBinDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin"), nil
}

// entireBrewPrefixes are Homebrew's prefixes on Apple silicon and Intel Macs.
var entireBrewPrefixes = []string{"/opt/homebrew", "/usr/local"}

// entireInstalled is the registry's Detect. On Linux it's both binaries in
// ~/.local/bin, so a hand install there counts and one missing its helper
// doesn't. On macOS only the cask counts: an `entire` from install.sh in
// ~/.local/bin would otherwise read as installed, and with Entire's own
// updater turned off nothing would ever update it.
func entireInstalled() bool {
	if platform.Detect().OS == platform.MacOS {
		return entireCaskInstalled()
	}
	dir, err := entireBinDir()
	if err != nil {
		return false
	}
	for _, b := range entireBinaries {
		if !isExecutableFile(filepath.Join(dir, b)) {
			return false
		}
	}
	return true
}

func entireCaskInstalled() bool {
	for _, prefix := range entireBrewPrefixes {
		if _, err := os.Stat(filepath.Join(prefix, "Caskroom", "entire")); err != nil {
			continue
		}
		if isExecutableFile(filepath.Join(prefix, "bin", "entire")) {
			return true
		}
	}
	return false
}

func isExecutableFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir() && fi.Mode().Perm()&0o111 != 0
}

// entireInstall puts the CLI in place and then, installed or not, turns off
// Entire's own update check. It never runs `entire enable`, `login` or
// `configure`: signing in and a repo's Entire settings belong to that repo.
func entireInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	// Phase 1: the binaries.
	if !opts.Force && alreadyInstalled("entire") {
		fmt.Fprintln(opts.Stdout, "entire already installed")
	} else {
		fmt.Fprintln(opts.Stdout, "Installing entire...")
		switch p.OS {
		case platform.MacOS:
			if err := sysutil.BrewCaskInstallVerified(ctx, o, entireCask, entireCaskInstalled); err != nil {
				return err
			}
			if err := removeHandInstalledEntire(ctx, o); err != nil {
				return err
			}
		case platform.Linux:
			ver, err := EntireLatestVersion(ctx)
			if err != nil {
				return err
			}
			if err := installEntireRelease(ctx, o, p.Arch, ver); err != nil {
				return err
			}
			if !o.DryRun {
				fmt.Fprintf(opts.Stdout, "entire %s installed\n", ver)
			}
		default:
			return ErrUnsupportedOS
		}
	}

	// Phase 2: always, so a hand install ctdev finds is taken over too.
	if err := disableEntireAutoUpdate(o); err != nil {
		return err
	}
	hintOtherEntire(opts.Stdout)
	return nil
}

// EntireUpdate moves the Linux install to ver, the release `ctdev update`
// listed. It replaces both binaries in ~/.local/bin, so it needs no root.
func EntireUpdate(ctx context.Context, o sysutil.Opts, ver string) error {
	if err := installEntireRelease(ctx, o, platform.Detect().Arch, ver); err != nil {
		return err
	}
	return disableEntireAutoUpdate(o)
}

// entireUninstall removes the binaries only. ~/.config/entire holds login
// contexts and credentials, and each repo's .entire/ and checkpoints are that
// repo's, so a reinstall finds them all again.
func entireUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing entire...")

	if p.OS == platform.MacOS {
		return sysutil.BrewCaskRemove(ctx, o, entireCask)
	}
	dir, err := entireBinDir()
	if err != nil {
		return err
	}
	for _, b := range entireBinaries {
		dest := filepath.Join(dir, b)
		if o.DryRun {
			fmt.Fprintf(o.Stdout, "[dry-run] rm -f %s\n", dest)
			continue
		}
		if err := os.Remove(dest); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	hintOtherEntire(opts.Stdout)
	return nil
}

// EntireLatestVersion returns the latest stable release ("0.13.4"), from
// GitHub or, when GitHub can't answer, from Entire's mirror.
func EntireLatestVersion(ctx context.Context) (string, error) {
	ver, ghErr := sysutil.GitHubLatestVersion(ctx, EntireRepo)
	if ghErr == nil && entireStableVersion.MatchString(ver) {
		return ver, nil
	}
	ver, mirrorErr := entireChannelVersion(ctx, entireChannelURL)
	if mirrorErr != nil {
		return "", fmt.Errorf("latest Entire release: GitHub: %v; install.entire.com: %w", ghErr, mirrorErr)
	}
	return ver, nil
}

// entireChannelVersion reads the mirror's stable channel pointer,
// {"channel":"stable","tag":"v0.13.4","version":"0.13.4"}. The tag wins over
// version because it's what the files are published under, as in install.sh.
func entireChannelVersion(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := sysutil.HTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var ch struct {
		Channel string `json:"channel"`
		Tag     string `json:"tag"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&ch); err != nil {
		return "", fmt.Errorf("parse channel: %w", err)
	}
	if ch.Channel != "" && ch.Channel != "stable" {
		return "", fmt.Errorf("channel pointer is for %q, not stable", ch.Channel)
	}
	ver := strings.TrimPrefix(ch.Tag, "v")
	if ver == "" {
		ver = strings.TrimPrefix(ch.Version, "v")
	}
	if !entireStableVersion.MatchString(ver) {
		return "", fmt.Errorf("channel pointer names %q, not a stable release", ver)
	}
	return ver, nil
}

// entireArchiveName is the Linux release archive for arch. Entire publishes
// Linux builds for amd64 and arm64 only.
func entireArchiveName(arch string) (string, error) {
	switch arch {
	case "amd64", "arm64":
		return "entire_linux_" + arch + ".tar.gz", nil
	}
	return "", fmt.Errorf("entire Linux build only available for amd64/arm64 (got %s): %w", arch, ErrUnsupportedOS)
}

// installEntireRelease downloads release ver, checks it, and replaces both
// binaries in ~/.local/bin.
func installEntireRelease(ctx context.Context, o sysutil.Opts, arch, ver string) error {
	if !entireStableVersion.MatchString(ver) {
		return fmt.Errorf("unexpected Entire version %q", ver)
	}
	archive, err := entireArchiveName(arch)
	if err != nil {
		return err
	}
	dir, err := entireBinDir()
	if err != nil {
		return err
	}
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download %s v%s, verify it against checksums.txt, install %s to %s\n",
			archive, ver, strings.Join(entireBinaries, " and "), dir)
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "ctdev-entire-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archivePath, err := fetchEntireRelease(ctx, o.Stdout, entireOrigins, ver, archive, tmpDir)
	if err != nil {
		return err
	}
	extracted := filepath.Join(tmpDir, "x")
	if err := os.Mkdir(extracted, 0o700); err != nil {
		return err
	}
	if err := sysutil.Run(ctx, o, "tar", "-xzf", archivePath, "-C", extracted); err != nil {
		return fmt.Errorf("extract %s: %w", archive, err)
	}
	return replaceEntireBinaries(extracted, dir)
}

// fetchEntireRelease downloads archive and checksums.txt for ver from the first
// origin that serves both, verifies one against the other, and returns the
// archive's path. Archive and checksums always come from the same origin:
// mixing them would let one origin's checksums vouch for another's bytes.
func fetchEntireRelease(ctx context.Context, w io.Writer, origins []entireOrigin, ver, archive, tmpDir string) (string, error) {
	var tried []string
	for _, origin := range origins {
		path, err := fetchEntireFromOrigin(ctx, origin, ver, archive, tmpDir)
		if err == nil {
			return path, nil
		}
		if !errors.Is(err, errEntireOriginUnavailable) {
			return "", err
		}
		fmt.Fprintf(w, "Could not get %s v%s from %s, trying the next source\n", archive, ver, origin.name)
		tried = append(tried, err.Error())
	}
	return "", fmt.Errorf("download %s v%s: %s", archive, ver, strings.Join(tried, "; "))
}

func fetchEntireFromOrigin(ctx context.Context, origin entireOrigin, ver, archive, tmpDir string) (string, error) {
	dir := filepath.Join(tmpDir, strings.ReplaceAll(origin.name, "/", "_"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	base := fmt.Sprintf("%s/v%s/", origin.base, ver)
	archivePath := filepath.Join(dir, archive)
	sumsPath := filepath.Join(dir, "checksums.txt")
	if err := sysutil.DownloadFile(ctx, base+archive, archivePath); err != nil {
		return "", fmt.Errorf("%s: %w: %v", origin.name, errEntireOriginUnavailable, err)
	}
	if err := sysutil.DownloadFile(ctx, base+"checksums.txt", sumsPath); err != nil {
		return "", fmt.Errorf("%s: %w: %v", origin.name, errEntireOriginUnavailable, err)
	}
	sums, err := os.ReadFile(sumsPath)
	if err != nil {
		return "", err
	}
	want := entireChecksumFor(string(sums), archive)
	if want == "" {
		return "", fmt.Errorf("%s: %w: checksums.txt does not list %s", origin.name, errEntireOriginUnavailable, archive)
	}
	if err := sysutil.VerifyChecksum(archivePath, strings.ToLower(want)); err != nil {
		return "", fmt.Errorf("%s v%s from %s: %w", archive, ver, origin.name, err)
	}
	return archivePath, nil
}

// entireChecksumFor returns the hash checksums.txt lists for name ("hash  name",
// or "hash *name" from `sha256sum -b`), or "".
func entireChecksumFor(sums, name string) string {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimPrefix(fields[1], "*") == name {
			return fields[0]
		}
	}
	return ""
}

// replaceEntireBinaries moves both binaries from src into dir. Each is copied
// beside its destination and renamed over it, so a git hook that runs `entire`
// mid-update gets the old binary or the new one, never half of one, and a
// running entire doesn't make the write fail.
func replaceEntireBinaries(src, dir string) error {
	for _, b := range entireBinaries {
		if !isExecutableFile(filepath.Join(src, b)) {
			return fmt.Errorf("release archive has no %s", b)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, b := range entireBinaries {
		dest := filepath.Join(dir, b)
		tmp := dest + ".ctdev-new"
		if err := copyFileMode(filepath.Join(src, b), tmp, 0o755); err != nil {
			os.Remove(tmp)
			return err
		}
		if err := os.Rename(tmp, dest); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	return nil
}

func copyFileMode(src, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dest, mode)
}

// entireConfigPath is Entire's user config file. Entire reads ENTIRE_CONFIG_DIR
// and otherwise ~/.config/entire; XDG_CONFIG_HOME doesn't move it.
func entireConfigPath() (string, error) {
	if dir := os.Getenv("ENTIRE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "entire", "config.json"), nil
}

// disableEntireAutoUpdate sets "no_auto_update": true in Entire's config.json,
// keeping every other key. ctdev owns Entire's updates (ADR-0018): Entire's
// own prompt runs install.sh or `brew upgrade` itself, which is a second
// updater. The file, unlike ENTIRE_NO_AUTO_UPDATE, reaches every invocation,
// including git hooks run by apps that never read a shell profile.
//
// A file that isn't valid JSON is left as it is: Entire itself reports it by
// name, and rewriting it would lose whatever the user meant to put there.
func disableEntireAutoUpdate(o sysutil.Opts) error {
	path, err := entireConfigPath()
	if err != nil {
		return err
	}
	settings := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(data, &settings); err != nil || settings == nil {
			fmt.Fprintf(o.Stdout, "%s isn't valid JSON, so Entire's own update check was left as it is. Fix the file, then run 'ctdev install entire' again.\n", path)
			return nil
		}
	}
	var off bool
	if raw, ok := settings["no_auto_update"]; ok && json.Unmarshal(raw, &off) == nil && off {
		return nil
	}
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] set \"no_auto_update\": true in %s\n", path)
		return nil
	}
	settings["no_auto_update"] = json.RawMessage("true")
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	// Login contexts and credentials share this directory, so it stays private.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp := path + ".ctdev-new"
	if err := os.WriteFile(tmp, append(out, '\n'), mode); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	fmt.Fprintf(o.Stdout, "Turned off Entire's own update check; 'ctdev update' updates it\n")
	return nil
}

// ours is where ctdev puts entire on this OS.
func entireOurs() []string {
	if platform.Detect().OS == platform.MacOS {
		var paths []string
		for _, prefix := range entireBrewPrefixes {
			paths = append(paths, filepath.Join(prefix, "bin", "entire"))
		}
		return paths
	}
	dir, err := entireBinDir()
	if err != nil {
		return nil
	}
	return []string{filepath.Join(dir, "entire")}
}

// otherEntireOnPath returns the first `entire` on PATH that isn't one of ours,
// or "".
func otherEntireOnPath(ours []string) string {
	resolve := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return filepath.Clean(p)
	}
	mine := map[string]bool{}
	for _, p := range ours {
		mine[resolve(p)] = true
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		cand := filepath.Join(dir, "entire")
		if !isExecutableFile(cand) || mine[resolve(cand)] {
			continue
		}
		return cand
	}
	return ""
}

// hintOtherEntire prints one line when another entire (a `go install`, a
// Linuxbrew cask) is on PATH beside ours. It is left alone: ctdev didn't put it
// there and can't know what else relies on it.
func hintOtherEntire(w io.Writer) {
	if other := otherEntireOnPath(entireOurs()); other != "" {
		fmt.Fprintf(w, "Another entire is at %s, outside ctdev; remove it so only one copy is on PATH\n", other)
	}
}

// removeHandInstalledEntire removes what Entire's install.sh put in
// ~/.local/bin on a Mac once the cask is in place, so only the cask's copy is
// on PATH. entire is removed only when it answers as Entire, and its helper
// only beside it.
func removeHandInstalledEntire(ctx context.Context, o sysutil.Opts) error {
	dir, err := entireBinDir()
	if err != nil {
		return err
	}
	cli := filepath.Join(dir, "entire")
	if !isExecutableFile(cli) {
		return nil
	}
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] remove an Entire install.sh copy in %s, if there is one\n", dir)
		return nil
	}
	out, err := entireVersionCmd(ctx, cli).Output()
	if err != nil || parseEntireVersion(string(out)) == "" {
		return nil
	}
	for _, b := range entireBinaries {
		if err := os.Remove(filepath.Join(dir, b)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	fmt.Fprintf(o.Stdout, "Removed the copy of Entire in %s; the Homebrew cask replaces it\n", dir)
	return nil
}

// EntireVersion returns the version of the entire ctdev manages ("0.13.4"),
// or "" when it isn't installed or doesn't answer. On Linux that's the one in
// ~/.local/bin, not whatever is first on PATH.
func EntireVersion(ctx context.Context) string {
	if !entireInstalled() {
		return ""
	}
	bin := "entire"
	if platform.Detect().OS != platform.MacOS {
		dir, err := entireBinDir()
		if err != nil {
			return ""
		}
		bin = filepath.Join(dir, "entire")
	}
	out, err := entireVersionCmd(ctx, bin).Output()
	if err != nil {
		return ""
	}
	return parseEntireVersion(string(out))
}

// entireVersionCmd is `entire version` with nothing sent anywhere. ctdev runs
// it to read the version, so it isn't the user's use of Entire: inside a repo
// that opted into Entire's telemetry it would report the command, and
// Entire's update check would reach out. The user's own telemetry choice is
// left alone (ADR-0018).
func entireVersionCmd(ctx context.Context, bin string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, bin, "version")
	cmd.Env = append(os.Environ(), "ENTIRE_TELEMETRY_OPTOUT=1", "ENTIRE_NO_AUTO_UPDATE=1")
	return cmd
}

// parseEntireVersion reads `entire version`, whose first line is
// "Entire CLI 0.13.4".
func parseEntireVersion(out string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	rest, ok := strings.CutPrefix(strings.TrimSpace(first), "Entire CLI ")
	if !ok {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(rest), "v")
}
