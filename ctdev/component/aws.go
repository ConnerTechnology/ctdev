package component

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// AWS CLI v2 (CTD-90 decision 3) comes from AWS's own installer on Linux,
// never apt, whose `awscli` package is v1 on several Ubuntu releases.
const (
	awsFormula    = "awscli"
	awsInstallDir = "/usr/local/aws-cli"
	awsBinDir     = "/usr/local/bin"

	// awsPublicKey is the AWS CLI Team key <aws-cli@amazon.com> that signs the
	// Linux zip, fingerprint FB5D B77F D5C1 18B8 0511 ADA8 A631 0ACC 4672 475C.
	// It expires 2027-07-01. To renew, replace the .asc with the key block from
	// AWS's install page, verbatim; TestAWSEmbeddedKeyIsAWSCLITeam checks the
	// fingerprint.
	awsPublicKey = "configs/aws/aws-cli-public-key.asc"
)

// awsVersion runs `aws --version`; a var so tests can fake the installed CLI.
var awsVersion = func(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "aws", "--version").CombinedOutput()
	return string(out), err
}

// awsIsV2 reports whether `aws --version` output is from AWS CLI v2.
func awsIsV2(versionOutput string) bool {
	return strings.HasPrefix(strings.TrimSpace(versionOutput), "aws-cli/2.")
}

// awsV2Installed is the aws component's Detect hook, so IsInstalled,
// InstallNeedsRoot, the installer's skip and `configure aws` all agree: an
// `aws` on PATH that reports v2. apt's v1 (Ubuntu's awscli package on several
// releases) doesn't count, so installing over it asks for sudo up front and
// `configure aws` offers v2 instead of running on v1.
func awsV2Installed() bool {
	if !sysutil.CommandExists("aws") {
		return false
	}
	out, err := awsVersion(context.Background())
	return err == nil && awsIsV2(out)
}

// awsZipURL is the latest Linux installer for a Go arch, as published at
// https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html.
func awsZipURL(arch string) (string, error) {
	switch arch {
	case "amd64":
		return "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip", nil
	case "arm64":
		return "https://awscli.amazonaws.com/awscli-exe-linux-aarch64.zip", nil
	default:
		return "", fmt.Errorf("aws: no AWS CLI v2 installer for linux/%s: %w", arch, ErrUnsupportedOS)
	}
}

// awsInstallArgs are the arguments to the unzipped aws/install. AWS documents
// --update for replacing an existing installation in the same directories.
func awsInstallArgs(update bool) []string {
	args := []string{"--bin-dir", awsBinDir, "--install-dir", awsInstallDir}
	if update {
		args = append(args, "--update")
	}
	return args
}

// awsInstall installs AWS CLI v2. `ctdev update` doesn't update the Linux
// install; `ctdev install aws --force` does, through aws/install --update.
func awsInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	// A v1 `aws` (apt's awscli) doesn't count as installed: v2 goes in over it.
	if !opts.Force && alreadyInstalled("aws") {
		fmt.Fprintln(opts.Stdout, "aws already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing AWS CLI v2...")

	// macOS uses the awscli Homebrew formula. AWS's docs don't cover Homebrew;
	// Thomas chose it in CTD-90 decision 3 ("the pkg or Homebrew").
	if p.OS == platform.MacOS {
		return sysutil.InstallPackage(ctx, o, awsFormula)
	}
	if p.OS != platform.Linux {
		return unsupportedPMError("aws", p.PackageManager)
	}

	// Linux follows AWS's command line installer: the zip, its .sig checked
	// with gpg, then aws/install. Sources, checked 2026-10-18:
	// https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html
	// https://docs.aws.amazon.com/cli/latest/userguide/getting-started-version.html (--update)
	// https://docs.aws.amazon.com/cli/latest/userguide/uninstall.html
	zipURL, err := awsZipURL(p.Arch)
	if err != nil {
		return err
	}
	_, statErr := os.Stat(awsInstallDir)
	update := statErr == nil

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download %s, verify its PGP signature, sudo ./aws/install %s\n",
			zipURL, strings.Join(awsInstallArgs(update), " "))
		return nil
	}

	if !sysutil.CommandExists("gpg") {
		if err := sysutil.InstallPackage(ctx, o, "gnupg"); err != nil {
			return err
		}
	}
	if !sysutil.CommandExists("unzip") {
		if err := sysutil.InstallPackage(ctx, o, "unzip"); err != nil {
			return err
		}
	}

	tmpDir, err := os.MkdirTemp("", "awscli-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	zipPath := filepath.Join(tmpDir, "awscliv2.zip")
	sigPath := filepath.Join(tmpDir, "awscliv2.sig")
	if err := sysutil.DownloadFile(ctx, zipURL, zipPath); err != nil {
		return fmt.Errorf("download AWS CLI: %w", err)
	}
	if err := sysutil.DownloadFile(ctx, zipURL+".sig", sigPath); err != nil {
		return fmt.Errorf("download AWS CLI signature: %w", err)
	}
	if err := awsVerifySignature(ctx, o, tmpDir, zipPath, sigPath); err != nil {
		return err
	}

	if err := sysutil.Run(ctx, o, "unzip", "-q", zipPath, "-d", tmpDir); err != nil {
		return err
	}
	return sysutil.SudoRun(ctx, o, filepath.Join(tmpDir, "aws", "install"), awsInstallArgs(update)...)
}

// awsVerifySignature checks the zip against the AWS CLI Team key embedded in
// ctdev, in a throwaway keyring so the user's own GnuPG home is never touched.
// Only that one key is in the keyring, so a good signature means it is AWS's.
func awsVerifySignature(ctx context.Context, o sysutil.Opts, tmpDir, zipPath, sigPath string) error {
	key, err := Configs.ReadFile(awsPublicKey)
	if err != nil {
		return err
	}
	gnupgHome := filepath.Join(tmpDir, "gnupg")
	if err := os.Mkdir(gnupgHome, 0o700); err != nil {
		return err
	}
	keyPath := filepath.Join(tmpDir, "aws-cli-public-key.asc")
	if err := os.WriteFile(keyPath, key, 0o644); err != nil {
		return err
	}
	if err := sysutil.Run(ctx, o, "gpg", "--homedir", gnupgHome, "--batch", "--quiet", "--import", keyPath); err != nil {
		return fmt.Errorf("import AWS CLI public key: %w", err)
	}
	if err := sysutil.Run(ctx, o, "gpg", "--homedir", gnupgHome, "--batch", "--verify", sigPath, zipPath); err != nil {
		return fmt.Errorf("AWS CLI signature check failed, not installing: %w", err)
	}
	return nil
}

func awsUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing AWS CLI...")

	if p.OS == platform.MacOS {
		return sysutil.RemovePackage(ctx, o, awsFormula)
	}
	if p.OS != platform.Linux {
		return unsupportedPMError("aws", p.PackageManager)
	}
	// AWS's documented uninstall (uninstall.html): the two symlinks, then the
	// install dir.
	// ~/.aws (profiles, SSO cache) stays: it's shared with every AWS SDK.
	return sysutil.SudoRun(ctx, o, "rm", "-rf",
		filepath.Join(awsBinDir, "aws"), filepath.Join(awsBinDir, "aws_completer"), awsInstallDir)
}
