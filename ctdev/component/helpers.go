package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// execOpts converts component ExecOpts to sysutil Opts.
func execOpts(opts ExecOpts) sysutil.Opts {
	return sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
}

// unsupportedPMError wraps ErrUnsupportedOS with a descriptive message so the
// executor's errors.Is(err, ErrUnsupportedOS) check maps it to Skipped.
// Use this in the default branch of package-manager switches.
func unsupportedPMError(component, pm string) error {
	return fmt.Errorf("%s install not supported for package manager %q: %w", component, pm, ErrUnsupportedOS)
}

// componentByName is populated from Registry in init(). Indexing it from
// within installer functions avoids the init-time cycle that would otherwise
// arise from `chatgptInstall -> alreadyInstalled -> FindByName -> Registry`.
var componentByName = make(map[string]*Component)

func init() {
	for i := range Registry {
		componentByName[Registry[i].Name] = &Registry[i]
	}
}

// alreadyInstalled is the canonical install-time "is this already present?"
// check. It delegates to the registry entry's IsInstalled so detection logic
// (DetectApps, DetectCmd, DetectPath) lives in exactly one place rather than
// being duplicated inline in each installer.
func alreadyInstalled(name string) bool {
	c := componentByName[name]
	return c != nil && c.IsInstalled()
}

// installDebWithDepFix installs a .deb via dpkg, recovers missing dependencies
// with `apt-get install -f`, and then verifies that pkgName is actually
// installed. Returns the original dpkg error (wrapped) when post-fix
// verification shows the package still isn't present — previously this path
// silently treated a successful `apt-get -f` as a successful install even
// when dpkg had failed for non-dependency reasons (corrupt .deb, wrong arch).
func installDebWithDepFix(ctx context.Context, o sysutil.Opts, debPath, pkgName string) error {
	dpkgErr := sysutil.SudoRun(ctx, o, "dpkg", "-i", debPath)
	if dpkgErr != nil {
		if fixErr := sysutil.SudoRun(ctx, o, "apt-get", "install", "-f", "-y"); fixErr != nil {
			return fmt.Errorf("dpkg failed and apt-get fix failed: dpkg: %w; fix: %v", dpkgErr, fixErr)
		}
	}
	// Dry-run can't verify; trust the call chain.
	if o.DryRun {
		return nil
	}
	if !sysutil.IsPackageInstalled(pkgName) {
		if dpkgErr != nil {
			return fmt.Errorf("dpkg -i failed and %s not installed after apt-get fix: %w", pkgName, dpkgErr)
		}
		return fmt.Errorf("%s not reported installed after dpkg -i", pkgName)
	}
	return nil
}

func SimplePackageInstaller(name string) func(context.Context, ExecOpts) error {
	return func(ctx context.Context, opts ExecOpts) error {
		o := execOpts(opts)
		if !opts.Force && sysutil.CommandExists(name) {
			fmt.Fprintf(opts.Stdout, "%s already installed\n", name)
			return nil
		}
		fmt.Fprintf(opts.Stdout, "Installing %s...\n", name)
		return sysutil.InstallPackage(ctx, o, name)
	}
}

func SimplePackageUninstaller(name string) func(context.Context, ExecOpts) error {
	return func(ctx context.Context, opts ExecOpts) error {
		o := execOpts(opts)
		fmt.Fprintf(opts.Stdout, "Removing %s...\n", name)
		return sysutil.RemovePackage(ctx, o, name)
	}
}
