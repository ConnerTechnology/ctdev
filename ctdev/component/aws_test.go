package component

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

func TestAWSIsRegistered(t *testing.T) {
	c := FindByName("aws")
	if c == nil {
		t.Fatal("aws should be in the registry")
	}
	if !c.SupportsOS(OSLinux) || !c.SupportsOS(OSMacOS) {
		t.Error("aws should support Linux and macOS")
	}
}

func TestAWSIsV2(t *testing.T) {
	cases := map[string]bool{
		"aws-cli/2.27.41 Python/3.11.6 Linux/5.10.205 exe/x86_64\n":   true,
		"aws-cli/1.22.34 Python/3.10.12 Linux/6.8.0 botocore/1.23.34": false,
		"": false,
	}
	for out, want := range cases {
		if got := awsIsV2(out); got != want {
			t.Errorf("awsIsV2(%q) = %v, want %v", out, got, want)
		}
	}
}

func TestAWSZipURL(t *testing.T) {
	for arch, want := range map[string]string{
		"amd64": "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip",
		"arm64": "https://awscli.amazonaws.com/awscli-exe-linux-aarch64.zip",
	} {
		got, err := awsZipURL(arch)
		if err != nil || got != want {
			t.Errorf("awsZipURL(%q) = %q, %v; want %q", arch, got, err, want)
		}
	}
	if _, err := awsZipURL("386"); !errors.Is(err, ErrUnsupportedOS) {
		t.Errorf("an arch AWS doesn't build for should be unsupported, got %v", err)
	}
}

// Re-running over an existing install needs --update, or AWS's installer stops
// at the preexisting installation.
func TestAWSInstallArgs(t *testing.T) {
	if got := strings.Join(awsInstallArgs(false), " "); got != "--bin-dir /usr/local/bin --install-dir /usr/local/aws-cli" {
		t.Errorf("fresh install args = %q", got)
	}
	if got := awsInstallArgs(true); got[len(got)-1] != "--update" {
		t.Errorf("update args should end in --update, got %q", got)
	}
}

// fakeAWSOnPath puts an `aws` stub on PATH and fakes its --version output.
func fakeAWSOnPath(t *testing.T, version string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "aws"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	orig := awsVersion
	awsVersion = func(context.Context) (string, error) { return version, nil }
	t.Cleanup(func() { awsVersion = orig })
}

func TestAWSInstallSkipsWhenV2Present(t *testing.T) {
	fakeAWSOnPath(t, "aws-cli/2.27.41 Python/3.11.6 Linux/6.8.0\n")
	var out bytes.Buffer
	if err := awsInstall(context.Background(), ExecOpts{DryRun: true, Stdout: &out}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "aws already installed") {
		t.Errorf("expected already-installed, got:\n%s", out.String())
	}
}

// apt's awscli is v1; it must not pass for the v2 this component promises.
func TestAWSInstallReplacesV1(t *testing.T) {
	if platform.Detect().OS != platform.Linux {
		t.Skip("the zip installer is Linux-only")
	}
	fakeAWSOnPath(t, "aws-cli/1.22.34 Python/3.10.12 Linux/6.8.0 botocore/1.23.34\n")
	var out bytes.Buffer
	if err := awsInstall(context.Background(), ExecOpts{DryRun: true, Stdout: &out}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Contains(got, "already installed") || !strings.Contains(got, "awscli.amazonaws.com/awscli-exe-linux-") {
		t.Errorf("a v1 aws should get the v2 installer, got:\n%s", got)
	}
	if strings.Contains(got, "apt") {
		t.Errorf("AWS CLI v2 must not come from apt, got:\n%s", got)
	}
}

func TestAWSEmbeddedKeyIsAWSCLITeam(t *testing.T) {
	key, err := Configs.ReadFile(awsPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(key, []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----")) {
		t.Error("embedded AWS CLI key should be an armored public key")
	}
	if _, err := exec.LookPath("gpg"); err != nil {
		t.Skip("gpg not installed")
	}
	out, err := exec.Command("gpg", "--homedir", t.TempDir(), "--show-keys", "--with-colons", "--fingerprint", awsPublicKey).Output()
	if err != nil {
		t.Fatal(err)
	}
	// Fingerprint as published by AWS for the AWS CLI Team key.
	if !strings.Contains(string(out), "FB5DB77FD5C118B80511ADA8A6310ACC4672475C") {
		t.Errorf("embedded key fingerprint doesn't match AWS's published one:\n%s", out)
	}
}

// A zip whose signature doesn't check out must never reach aws/install.
func TestAWSVerifySignatureRejectsBadSignature(t *testing.T) {
	if _, err := exec.LookPath("gpg"); err != nil {
		t.Skip("gpg not installed")
	}
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "awscliv2.zip")
	sigPath := filepath.Join(dir, "awscliv2.sig")
	if err := os.WriteFile(zipPath, []byte("not the real installer"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sigPath, []byte("not a signature"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := awsVerifySignature(context.Background(), sysutil.Opts{Stdout: &out}, dir, zipPath, sigPath)
	if err == nil || !strings.Contains(err.Error(), "signature check failed") {
		t.Errorf("expected a signature failure, got %v", err)
	}
}

// Detection is v2-aware in one place, so the sudo pre-check agrees with the
// installer: installing over apt's v1 needs root, and the TUI must not be the
// first to ask for it.
func TestAWSDetectionIgnoresV1(t *testing.T) {
	c := FindByName("aws")
	fakeAWSOnPath(t, "aws-cli/1.22.34 Python/3.10.12 Linux/6.8.0 botocore/1.23.34\n")
	if c.IsInstalled() {
		t.Error("a v1 aws should not count as installed")
	}
	if !InstallNeedsRoot("apt", []string{"aws"}, false) {
		t.Error("installing v2 over v1 should need root")
	}
}

func TestAWSDetectionAcceptsV2(t *testing.T) {
	c := FindByName("aws")
	fakeAWSOnPath(t, "aws-cli/2.27.41 Python/3.11.6 Linux/6.8.0\n")
	if !c.IsInstalled() {
		t.Error("a v2 aws should count as installed")
	}
	if InstallNeedsRoot("apt", []string{"aws"}, false) {
		t.Error("re-running over v2 shouldn't need root")
	}
}
