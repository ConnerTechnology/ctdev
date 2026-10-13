package component

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

const pythonVersion = "3.14.8"

func pythonInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	pyenvDir := filepath.Join(home, ".pyenv")

	if !opts.Force && dirExists(pyenvDir) {
		fmt.Fprintln(opts.Stdout, "Python already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing Python via pyenv...")

	switch p.PackageManager {
	case "brew":
		// pyenv ships python-build; the rest are what CPython compiles against.
		if err := sysutil.Run(ctx, o, "brew", "install", "pyenv"); err != nil {
			return err
		}
		for _, pkg := range []string{"openssl", "readline", "sqlite3", "xz", "zlib", "tcl-tk"} {
			_ = sysutil.Run(ctx, o, "brew", "install", pkg)
		}

	case "apt":
		// pyenv/pyenv bundles python-build as a plugin, so one clone is enough.
		if err := ensureGitClone(ctx, o, "https://github.com/pyenv/pyenv.git", pyenvDir); err != nil {
			return fmt.Errorf("clone pyenv: %w", err)
		}
		fmt.Fprintln(opts.Stdout, "Installing Python build dependencies...")
		if err := sysutil.InstallPackage(ctx, o, "build-essential", "libssl-dev", "zlib1g-dev",
			"libbz2-dev", "libreadline-dev", "libsqlite3-dev", "libncursesw5-dev", "xz-utils",
			"tk-dev", "libxml2-dev", "libxmlsec1-dev", "libffi-dev", "liblzma-dev"); err != nil {
			return fmt.Errorf("install build deps: %w", err)
		}

	default:
		return unsupportedPMError("python", p.PackageManager)
	}

	// Use pyenv from the install location
	pyenvBin := "pyenv"
	if p.PackageManager != "brew" {
		pyenvBin = filepath.Join(pyenvDir, "bin", "pyenv")
	}

	fmt.Fprintf(opts.Stdout, "Installing Python %s...\n", pythonVersion)
	if err := sysutil.Run(ctx, o, pyenvBin, "install", "--skip-existing", pythonVersion); err != nil {
		return fmt.Errorf("pyenv install %s: %w", pythonVersion, err)
	}

	if err := sysutil.Run(ctx, o, pyenvBin, "global", pythonVersion); err != nil {
		return fmt.Errorf("pyenv global %s: %w", pythonVersion, err)
	}

	if p.PackageManager == "brew" {
		migrateBrewPython(ctx, o, opts)
	}

	return nil
}

func pythonUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Python (pyenv)...")

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// Removing ~/.pyenv takes every installed Python and its site-packages
	// with it. The Homebrew pyenv formula, if any, is left to `brew`.
	pyenvDir := filepath.Join(home, ".pyenv")
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] rm -rf %s\n", pyenvDir)
		return nil
	}

	return os.RemoveAll(pyenvDir)
}

// migrateBrewPython moves a machine off Homebrew-managed Python: formulae
// nothing else needs are uninstalled, and the rest are kept and reported.
// Homebrew formulae such as gcloud-cli depend on python@3.x, so removing one
// that has dependents would break those tools; pyenv's shims win on PATH
// regardless, so keeping it costs nothing. Failures here are warnings — the
// pyenv install already succeeded.
func migrateBrewPython(ctx context.Context, o sysutil.Opts, opts ExecOpts) {
	if o.DryRun {
		fmt.Fprintln(opts.Stdout, "[dry-run] remove Homebrew python formulae that nothing depends on")
		return
	}

	out, err := brewOutput(ctx, "list", "--formula", "-1")
	if err != nil {
		fmt.Fprintf(opts.Stdout, "warning: could not list Homebrew formulae: %v\n", err)
		return
	}
	formulae := brewPythonFormulae(strings.Fields(out))
	if len(formulae) == 0 {
		return
	}

	dependents := make(map[string][]string, len(formulae))
	for _, f := range formulae {
		uses, err := brewOutput(ctx, "uses", "--installed", f)
		if err != nil {
			fmt.Fprintf(opts.Stdout, "warning: could not check what uses %s: %v\n", f, err)
			return
		}
		dependents[f] = strings.Fields(uses)
	}

	remove, keep := planBrewPythonMigration(formulae, dependents)
	for _, f := range remove {
		fmt.Fprintf(opts.Stdout, "Removing Homebrew %s (replaced by pyenv)...\n", f)
		args := []string{"uninstall"}
		for _, d := range dependents[f] { // add-ons first: brew refuses to remove a formula they use
			args = append(args, d)
		}
		args = append(args, f)
		if err := sysutil.Run(ctx, o, "brew", args...); err != nil {
			fmt.Fprintf(opts.Stdout, "warning: could not remove %s: %v\n", f, err)
		}
	}
	for _, f := range sortedKeys(keep) {
		fmt.Fprintf(opts.Stdout, "Keeping Homebrew %s: needed by %s. pyenv's python still comes first on PATH.\n",
			f, strings.Join(keep[f], ", "))
	}
}

// brewPythonFormulae picks the Python interpreters out of a formula list:
// "python" and the versioned "python@3.x". Libraries like python-tk@3.14 are
// not interpreters and are not matched.
func brewPythonFormulae(installed []string) []string {
	var out []string
	for _, f := range installed {
		if f == "python" || strings.HasPrefix(f, "python@") {
			out = append(out, f)
		}
	}
	return out
}

// planBrewPythonMigration splits formulae into those safe to uninstall and
// those kept, with the dependents that keep them. A dependent that is itself
// being removed (python-tk@3.14 needs python@3.14) does not count.
func planBrewPythonMigration(formulae []string, dependents map[string][]string) (remove []string, keep map[string][]string) {
	keep = make(map[string][]string)
	for _, f := range formulae {
		var blockers []string
		for _, d := range dependents[f] {
			if !isPythonAddon(d, f) {
				blockers = append(blockers, d)
			}
		}
		if len(blockers) > 0 {
			keep[f] = blockers
		} else {
			remove = append(remove, f)
		}
	}
	return remove, keep
}

// isPythonAddon reports whether dep is a Python-version add-on of formula
// (python-tk@3.14 for python@3.14), which goes away with it.
func isPythonAddon(dep, formula string) bool {
	version := strings.TrimPrefix(formula, "python")
	return strings.HasPrefix(dep, "python-") && strings.HasSuffix(dep, version) && version != ""
}

func brewOutput(ctx context.Context, args ...string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, "brew", args...)
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
