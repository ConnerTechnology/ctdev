package component

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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
		npm, err := npmPath(o)
		if err != nil {
			return err
		}
		if err := sysutil.Run(ctx, o, npm, "install", "-g", pkg); err != nil {
			return fmt.Errorf("npm install %s: %w", pkg, err)
		}
		return nil
	}
}

func npmGlobalUninstaller(name, pkg string) func(context.Context, ExecOpts) error {
	return func(ctx context.Context, opts ExecOpts) error {
		o := execOpts(opts)
		fmt.Fprintf(opts.Stdout, "Removing %s...\n", name)
		npm, err := npmPath(o)
		if err != nil {
			return err
		}
		return sysutil.Run(ctx, o, npm, "uninstall", "-g", pkg)
	}
}

// npmPath finds npm, falling back to the nodenv shim when nodenv isn't yet on
// PATH (common right after `ctdev install node` in the same shell session). In
// a dry run with no npm yet it returns the bare "npm", so callers print what
// they would run instead of failing.
func npmPath(o sysutil.Opts) (string, error) {
	if p, err := exec.LookPath("npm"); err == nil {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	shim := filepath.Join(home, ".nodenv", "shims", "npm")
	if _, err := os.Stat(shim); err == nil {
		return shim, nil
	}
	// A dry run installs nothing, so the node component earlier in the same run
	// can't have put npm in place yet. Report the command a real run would use.
	if o.DryRun {
		return "npm", nil
	}
	return "", fmt.Errorf("npm not found — install the node component first")
}
