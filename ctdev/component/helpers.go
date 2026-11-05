package component

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// execOpts converts component ExecOpts to sysutil Opts.
func execOpts(opts ExecOpts) sysutil.Opts {
	return sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun, NoSudoPrompt: opts.NoSudoPrompt}
}

// unsupportedPMError wraps ErrUnsupportedOS with a descriptive message so the
// executor's errors.Is(err, ErrUnsupportedOS) check maps it to Skipped.
// Use this in the default branch of package-manager switches.
func unsupportedPMError(component, pm string) error {
	return fmt.Errorf("%s install not supported for package manager %q: %w", component, pm, ErrUnsupportedOS)
}

// componentByName is populated from Registry in init(). Indexing it from
// within installer functions avoids the init-time cycle that would otherwise
// arise from `slackInstall -> alreadyInstalled -> FindByName -> Registry`.
var componentByName = make(map[string]*Component)

func init() {
	for i := range Registry {
		componentByName[Registry[i].Name] = &Registry[i]
	}
}

// alreadyInstalled is the canonical install-time "is this already present?"
// check. It delegates to the registry entry's IsInstalled so detection logic
// (Detect, DetectApps, DetectCmd, DetectPath) lives in exactly one place rather than
// being duplicated inline in each installer.
func alreadyInstalled(name string) bool {
	c := componentByName[name]
	return c != nil && c.IsInstalled()
}

// installDebWithDepFix installs a downloaded .deb with `apt-get install`,
// which pulls in its dependencies in the same step and goes through
// sysutil.APTGet's lock wait, unlike `dpkg -i`. apt-get takes a file only when
// the argument looks like a path, so debPath is made absolute. It then
// verifies pkgName is actually installed rather than trusting the exit code.
func installDebWithDepFix(ctx context.Context, o sysutil.Opts, debPath, pkgName string) error {
	abs, err := filepath.Abs(debPath)
	if err != nil {
		return err
	}
	if err := sysutil.APTGet(ctx, o, "install", "-y", abs); err != nil {
		return err
	}
	// Dry-run can't verify; trust the call chain.
	if o.DryRun {
		return nil
	}
	if !sysutil.IsPackageInstalled(pkgName) {
		return fmt.Errorf("%s not reported installed after installing %s", pkgName, abs)
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

// deployOrDryRun deploys an embedded config file, or reports it under --dry-run.
func deployOrDryRun(o sysutil.Opts, srcEmbed, dst string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] deploy %s → %s\n", filepath.Base(srcEmbed), dst)
		return nil
	}
	return sysutil.DeployFileFromFS(Configs, srcEmbed, dst)
}
