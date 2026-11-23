// Package nodenv reads and changes the Node versions nodenv manages. Reads go
// straight to nodenv's files, so they are cheap enough for the install picker;
// changes shell out through sysutil, so a dry run prints them.
package nodenv

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// IndexURL lists every Node release with its LTS codename.
const IndexURL = "https://nodejs.org/dist/index.json"

// VersionFile is the per-project pin nodenv reads.
const VersionFile = ".node-version"

// Root is nodenv's directory: $NODENV_ROOT, else ~/.nodenv, which Homebrew's
// nodenv uses too.
func Root() string {
	if r := os.Getenv("NODENV_ROOT"); r != "" {
		return r
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".nodenv")
}

// Bin is the nodenv to run: the git clone's, else Homebrew's on PATH. A shell
// started before the clone has neither on PATH, so the clone's path is
// absolute.
func Bin() string {
	p := filepath.Join(Root(), "bin", "nodenv")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return "nodenv"
}

// Available reports whether nodenv is installed.
func Available() bool {
	_, err := exec.LookPath(Bin())
	return err == nil
}

// Installed lists the versions under nodenv's versions directory, oldest
// first.
func Installed() []string {
	entries, _ := os.ReadDir(filepath.Join(Root(), "versions"))
	var vs []string
	for _, e := range entries {
		if IsInstalled(e.Name()) {
			vs = append(vs, e.Name())
		}
	}
	slices.SortFunc(vs, Compare)
	return vs
}

// IsInstalled reports whether version v is installed.
func IsInstalled(v string) bool {
	info, err := os.Stat(filepath.Join(Root(), "versions", v))
	return v != "" && err == nil && info.IsDir()
}

// readVersionFile is the first word of a nodenv version file. "system" means
// no nodenv-managed Node, so it reads as "".
func readVersionFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Split(bufio.ScanWords)
	if !sc.Scan() || sc.Text() == "system" {
		return ""
	}
	return strings.TrimPrefix(sc.Text(), "v")
}

// Global is `nodenv global`, "" when it is unset or "system".
func Global() string {
	return readVersionFile(filepath.Join(Root(), "version"))
}

// ProjectPin finds the .node-version that governs dir, searching upward as
// nodenv does. It returns the file and its version, or "" for both.
func ProjectPin(dir string) (file, version string) {
	for dir != "" {
		p := filepath.Join(dir, VersionFile)
		if _, err := os.Stat(p); err == nil {
			return p, readVersionFile(p)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", ""
}

// Current is the version nodenv runs in dir: $NODENV_VERSION, else the
// nearest .node-version, else the global version. "" means none.
func Current(dir string) string {
	if v := os.Getenv("NODENV_VERSION"); v != "" {
		if v == "system" {
			return ""
		}
		return v
	}
	if file, v := ProjectPin(dir); file != "" {
		return v
	}
	return Global()
}

func nodeModules(v string) string {
	return filepath.Join(Root(), "versions", v, "lib", "node_modules")
}

// HasPackage reports whether version v has the global npm package pkg.
func HasPackage(v, pkg string) bool {
	_, err := os.Stat(filepath.Join(nodeModules(v), pkg, "package.json"))
	return v != "" && err == nil
}

// bundled ship with Node itself, so every version already has them.
var bundled = map[string]bool{"npm": true, "corepack": true}

// GlobalPackages lists the global npm packages version v has, read from its
// node_modules as `npm ls -g` would. Linked packages (`npm link`) are left
// out: reinstalling one from the registry would replace the user's checkout.
func GlobalPackages(v string) []Package {
	var ps []Package
	var walk func(dir, scope string)
	walk = func(dir, scope string) {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			name := e.Name()
			switch {
			case strings.HasPrefix(name, "."), !e.IsDir():
				continue
			case scope == "" && strings.HasPrefix(name, "@"):
				walk(filepath.Join(dir, name), name+"/")
				continue
			}
			full := scope + name
			if bundled[full] {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, name, "package.json"))
			if err != nil {
				continue
			}
			var meta struct {
				Version string `json:"version"`
			}
			if json.Unmarshal(data, &meta) == nil && meta.Version != "" {
				ps = append(ps, Package{Name: full, Version: meta.Version})
			}
		}
	}
	walk(nodeModules(v), "")
	slices.SortFunc(ps, func(a, b Package) int { return strings.Compare(a.Name, b.Name) })
	return ps
}

// List is `nodenv install --list`: node-build's newest release of each major
// it supports. It is a read, so it runs in a dry run too.
func List(ctx context.Context) ([]string, error) {
	return list(ctx, "--list")
}

// ListAll is `nodenv install --list-all`, which still carries retired majors.
func ListAll(ctx context.Context) ([]string, error) {
	return list(ctx, "--list-all")
}

func list(ctx context.Context, flag string) ([]string, error) {
	out, err := exec.CommandContext(ctx, Bin(), "install", flag).Output()
	if err != nil {
		return nil, fmt.Errorf("nodenv install %s: %w", flag, err)
	}
	return ParseList(string(out)), nil
}

// LTS is the release LatestLTS picked.
type LTS struct {
	Version string
	LTSLine
}

// LatestLTS is the newest release node-build has of the newest LTS major in
// nodejs.org's index.
func LatestLTS(ctx context.Context) (LTS, error) {
	data, err := fetchIndex(ctx)
	if err != nil {
		return LTS{}, err
	}
	rs, err := ParseIndex(data)
	if err != nil {
		return LTS{}, err
	}
	line, ok := PickLTS(rs)
	if !ok {
		return LTS{}, errors.New("nodejs.org lists no LTS release")
	}
	vs, err := List(ctx)
	if err != nil {
		return LTS{}, err
	}
	v := LatestOfMajor(vs, line.Major)
	if v == "" {
		return LTS{}, fmt.Errorf("node-build has no Node %d yet; update it with ctdev update", line.Major)
	}
	return LTS{Version: v, LTSLine: line}, nil
}

func fetchIndex(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, IndexURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := sysutil.HTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch Node release index: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch Node release index: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// Install installs version v, skipping it when it is already there.
func Install(ctx context.Context, o sysutil.Opts, v string) error {
	return sysutil.Run(ctx, o, Bin(), "install", "--skip-existing", v)
}

// Uninstall deletes version v and every global package it has.
func Uninstall(ctx context.Context, o sysutil.Opts, v string) error {
	return sysutil.Run(ctx, o, Bin(), "uninstall", "-f", v)
}

// SetGlobal is `nodenv global v`.
func SetGlobal(ctx context.Context, o sysutil.Opts, v string) error {
	return sysutil.Run(ctx, o, Bin(), "global", v)
}

// Rehash refreshes nodenv's shims, which is how a package's command reaches
// PATH.
func Rehash(ctx context.Context, o sysutil.Opts) error {
	return sysutil.Run(ctx, o, Bin(), "rehash")
}

// WritePin writes version v to the .node-version file p, as `nodenv local`
// does in p's folder.
func WritePin(o sysutil.Opts, p, v string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] write %s: %s\n", p, v)
		return nil
	}
	return os.WriteFile(p, []byte(v+"\n"), 0o644)
}

// InstallPackages installs ps globally under version v at the versions they
// carry.
func InstallPackages(ctx context.Context, o sysutil.Opts, v string, ps []Package) error {
	if len(ps) == 0 {
		return nil
	}
	args := []string{"exec", "npm", "install", "-g"}
	for _, p := range ps {
		args = append(args, p.Spec())
	}
	return sysutil.RunEnv(ctx, o, []string{"NODENV_VERSION=" + v}, Bin(), args...)
}
