package component

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/nodenv"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// npmGlobalInstaller installs an npm package globally, the same way on every
// OS. `ctdev update` keeps it current through its npm-globals scan, so the
// component needs no scanner of its own. name is the registry entry, used for
// the "already installed" check.
func npmGlobalInstaller(name, pkg string) func(context.Context, ExecOpts) error {
	return func(ctx context.Context, opts ExecOpts) error {
		o := execOpts(opts)
		if !opts.Force && alreadyInstalled(name) {
			fmt.Fprintf(opts.Stdout, "%s already installed\n", name)
			return nil
		}
		fmt.Fprintf(opts.Stdout, "Installing %s...\n", name)
		return npmInstallGlobal(ctx, o, pkg)
	}
}

func npmGlobalUninstaller(name, pkg string) func(context.Context, ExecOpts) error {
	return func(ctx context.Context, opts ExecOpts) error {
		o := execOpts(opts)
		fmt.Fprintf(opts.Stdout, "Removing %s...\n", name)
		return npmUninstallGlobal(ctx, o, pkg)
	}
}

// npmPackageDetect counts an npm component as installed when nodenv's global
// Node has its package: npm components are machine-wide tools, installed
// under the global version whatever this folder pins. A nodenv shim stays on
// PATH after the Node it came from stops being global, so finding cmd isn't
// enough. It only stats files: the picker runs it for every component.
func npmPackageDetect(cmd, pkg string) func() bool {
	return func() bool {
		if v := nodenv.Global(); v != "" && nodenv.Available() {
			return nodenv.HasPackage(v, pkg)
		}
		p, err := exec.LookPath(cmd)
		return err == nil && !strings.HasPrefix(p, filepath.Join(nodenv.Root(), "shims")+string(filepath.Separator))
	}
}

func npmInstallGlobal(ctx context.Context, o sysutil.Opts, pkg string) error {
	node, err := ensureGlobalNode(ctx, o)
	if err != nil {
		return err
	}
	if err := runNpm(ctx, o, node, "install", "-g", pkg); err != nil {
		return fmt.Errorf("npm install %s: %w", pkg, err)
	}
	return nil
}

func npmUninstallGlobal(ctx context.Context, o sysutil.Opts, pkg string) error {
	if err := runNpm(ctx, o, nodenv.GlobalSelector(), "uninstall", "-g", pkg); err != nil {
		return fmt.Errorf("npm uninstall %s: %w", pkg, err)
	}
	return nil
}

// latestLTS is a var so tests answer without nodejs.org.
var latestLTS = nodenv.LatestLTS

// ensureGlobalNode is the nodenv version an npm component installs under:
// the global one, so a project's .node-version doesn't decide where a
// machine-wide tool goes. With no global it installs the latest LTS and makes
// it global; an existing global is never changed. "" means npm runs outside
// nodenv.
func ensureGlobalNode(ctx context.Context, o sysutil.Opts) (string, error) {
	if !nodenv.Available() {
		// A dry run installs nothing, so nodenv from earlier in the run isn't
		// there to ask yet.
		if o.DryRun {
			fmt.Fprintln(o.Stdout, "[dry-run] if no global Node version is set: nodenv install <latest LTS> and make it global")
		}
		return "", nil
	}
	if v := nodenv.Global(); v != "" {
		return v, nil
	}
	if nodenv.GlobalIsSystem() {
		return "", errors.New(`nodenv's global Node is "system", and npm components need a Node nodenv manages. ` +
			"Pick one with: ctdev configure node install --global")
	}
	lts, err := latestLTS(ctx)
	if err != nil {
		return "", fmt.Errorf("find the latest Node LTS: %w", err)
	}
	fmt.Fprintf(o.Stdout, "No global Node version is set. Installing the latest LTS, Node %s (v%d %q)...\n", lts.Version, lts.Major, lts.Codename)
	if err := nodenv.Install(ctx, o, lts.Version); err != nil {
		return "", err
	}
	if err := nodenv.SetGlobal(ctx, o, lts.Version); err != nil {
		return "", err
	}
	return lts.Version, nil
}

// runNpm runs npm under nodenv version node, or the npm on PATH when node is
// "". nodenv makes a shim for a package's command only on rehash, so without
// one `codex` is not found until a new shell runs `nodenv init`. The package
// is already in place by then, so a failed rehash only warns.
func runNpm(ctx context.Context, o sysutil.Opts, node string, args ...string) error {
	if node != "" {
		env := []string{"NODENV_VERSION=" + node}
		if err := sysutil.RunEnv(ctx, o, env, nodenv.Bin(), append([]string{"exec", "npm"}, args...)...); err != nil {
			return err
		}
	} else {
		npm, err := npmPath(o)
		if err != nil {
			return err
		}
		if err := sysutil.Run(ctx, o, npm, args...); err != nil {
			return err
		}
		if !strings.HasPrefix(npm, nodenv.Root()+string(filepath.Separator)) {
			return nil
		}
	}
	if err := nodenv.Rehash(ctx, o); err != nil {
		fmt.Fprintf(o.Stdout, "warning: nodenv rehash: %v (open a new shell to pick up the command)\n", err)
	}
	return nil
}

// npmPath finds npm, falling back to the nodenv shim when nodenv isn't yet on
// PATH (common right after `ctdev install nodenv` in the same shell session).
// In a dry run with no npm yet it returns the bare "npm", so callers print
// what they would run instead of failing.
func npmPath(o sysutil.Opts) (string, error) {
	if p, err := exec.LookPath("npm"); err == nil {
		return p, nil
	}
	shim := filepath.Join(nodenv.Root(), "shims", "npm")
	if _, err := os.Stat(shim); err == nil {
		return shim, nil
	}
	// A dry run installs nothing, so nodenv earlier in the same run can't have
	// put npm in place yet. Report the command a real run would use.
	if o.DryRun {
		return "npm", nil
	}
	return "", fmt.Errorf("npm not found — install the nodenv component first")
}
