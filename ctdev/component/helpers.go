package component

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

// verifiedPublicKey returns the armored public key in keyPath, provided the
// file holds exactly one OpenPGP primary key, its fingerprint is want (40 hex
// digits, no spaces) and nothing in it is secret key material. Exactly one,
// not "at least one": a keyring file is trusted whole, so a second key riding
// along with the vendor's would be trusted too, and apt's sqv accepts
// signatures from a secret key in a signed-by file.
//
// The bytes returned are gpg's re-export of the pinned key, not the file
// itself, so whatever else the file carried never reaches the caller. gpg runs
// in a throwaway home; the user's own keyring is never read or touched.
func verifiedPublicKey(ctx context.Context, keyPath, want string) ([]byte, error) {
	home, err := os.MkdirTemp("", "ctdev-gnupg-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(home)

	out, err := runGPG(ctx, home, "--show-keys", "--with-colons", keyPath)
	if err != nil {
		return nil, fmt.Errorf("read OpenPGP key %s: %w", keyPath, err)
	}
	got, err := primaryKeyFingerprints(string(out))
	switch {
	case err != nil:
		return nil, fmt.Errorf("%s: %w", keyPath, err)
	case len(got) == 0:
		return nil, fmt.Errorf("no OpenPGP key in %s, want %s", keyPath, want)
	case len(got) != 1 || got[0] != want:
		return nil, fmt.Errorf("key fingerprint is %s, want %s", strings.Join(got, ", "), want)
	}

	if _, err := runGPG(ctx, home, "--import", keyPath); err != nil {
		return nil, fmt.Errorf("import OpenPGP key %s: %w", keyPath, err)
	}
	key, err := runGPG(ctx, home, "--export", "--armor", want)
	if err != nil {
		return nil, fmt.Errorf("export OpenPGP key %s: %w", want, err)
	}
	if len(key) == 0 {
		return nil, fmt.Errorf("export OpenPGP key %s: gpg exported nothing", want)
	}
	return key, nil
}

// runGPG runs gpg against home and returns its stdout. On failure the error
// carries gpg's stderr, which is where it says what was wrong with the key.
func runGPG(ctx context.Context, home string, args ...string) ([]byte, error) {
	var stderr strings.Builder
	cmd := exec.CommandContext(ctx, "gpg", append([]string{"--homedir", home, "--batch"}, args...)...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gpg: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// primaryKeyFingerprints picks the primary keys' fingerprints out of gpg
// --with-colons output. Each primary record is followed by its "fpr"; the
// "fpr" records after a "sub" belong to subkeys and are skipped. Any secret
// key or subkey ("sec", "ssb") is an error: a vendor's public key file never
// has one.
func primaryKeyFingerprints(colons string) ([]string, error) {
	var fprs []string
	afterPrimary := false
	for _, line := range strings.Split(colons, "\n") {
		fields := strings.Split(line, ":")
		switch fields[0] {
		case "sec", "ssb":
			return nil, fmt.Errorf("contains secret key material (%s record)", fields[0])
		case "pub":
			afterPrimary = true
		case "sub":
			afterPrimary = false
		case "fpr":
			if afterPrimary && len(fields) > 9 {
				fprs = append(fprs, fields[9])
				afterPrimary = false
			}
		}
	}
	return fprs, nil
}
