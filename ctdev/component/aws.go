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

// AWS CLI v2 comes from AWS's own installer, never apt: Ubuntu's `awscli`
// package is v1 on several releases, and `aws configure sso` needs v2.
const (
	awsFormula    = "awscli"
	awsInstallDir = "/usr/local/aws-cli"
	awsBinDir     = "/usr/local/bin"
	awsPublicKey  = "configs/aws/aws-cli-public-key.asc"
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

func awsInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	// A v1 `aws` (apt's awscli) on PATH doesn't count: install v2 over it.
	if !opts.Force && alreadyInstalled("aws") {
		if out, err := awsVersion(ctx); err == nil && awsIsV2(out) {
			fmt.Fprintln(opts.Stdout, "aws already installed")
			return nil
		}
	}

	fmt.Fprintln(opts.Stdout, "Installing AWS CLI v2...")

	if p.OS == platform.MacOS {
		return sysutil.InstallPackage(ctx, o, awsFormula)
	}
	if p.OS != platform.Linux {
		return unsupportedPMError("aws", p.PackageManager)
	}

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

	for _, dep := range [][2]string{{"gpg", "gnupg"}, {"unzip", "unzip"}} {
		if !sysutil.CommandExists(dep[0]) {
			if err := sysutil.InstallPackage(ctx, o, dep[1]); err != nil {
				return err
			}
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
	// AWS's documented uninstall: the two symlinks, then the install dir.
	// ~/.aws (profiles, SSO cache) stays: it's shared with every AWS SDK.
	return sysutil.SudoRun(ctx, o, "rm", "-rf",
		filepath.Join(awsBinDir, "aws"), filepath.Join(awsBinDir, "aws_completer"), awsInstallDir)
}
