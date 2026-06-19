package sysutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// AddAPTKeyring downloads a GPG key and installs it as an APT keyring.
func AddAPTKeyring(ctx context.Context, o Opts, url, keyringPath string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download GPG key %s → %s\n", url, keyringPath)
		return nil
	}
	tmp, err := os.CreateTemp("", "gpg-key-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	if err := DownloadFile(ctx, url, tmp.Name()); err != nil {
		return fmt.Errorf("download GPG key: %w", err)
	}

	// Dearmor into a temp file, then sudo-copy to the keyring path
	dearmorTmp, err := os.CreateTemp("", "gpg-dearmor-*")
	if err != nil {
		return err
	}
	defer os.Remove(dearmorTmp.Name())
	dearmorTmp.Close()

	dearmor := exec.CommandContext(ctx, "gpg", "--dearmor", "--yes", "-o", dearmorTmp.Name())
	in, err := os.Open(tmp.Name())
	if err != nil {
		return err
	}
	defer in.Close()
	dearmor.Stdin = in
	dearmor.Stdout = o.Stdout
	dearmor.Stderr = o.Stdout
	if err := dearmor.Run(); err != nil {
		return err
	}
	if err := SudoRun(ctx, o, "cp", dearmorTmp.Name(), keyringPath); err != nil {
		return err
	}
	// World-readable: the temp source is 0600 and cp preserves that, but apt's
	// signature verifier (Debian 13's sqv runs sandboxed) must be able to read
	// the keyring or the repo is rejected as unsigned.
	return SudoRun(ctx, o, "chmod", "0644", keyringPath)
}

// AddAPTSource writes an APT sources list entry.
func AddAPTSource(ctx context.Context, o Opts, line, filename string) error {
	path := "/etc/apt/sources.list.d/" + filename
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] write %s\n", path)
		return nil
	}
	tmp, err := os.CreateTemp("", "apt-source-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(line + "\n"); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	return SudoRun(ctx, o, "cp", tmp.Name(), path)
}

// APTUpdate runs apt-get update.
func APTUpdate(ctx context.Context, o Opts) error {
	return SudoRun(ctx, o, "apt-get", "update", "-qq")
}

// APTRepoPackage describes a third-party APT repository and the package(s) to
// install from it. RepoLine should already embed signed-by=KeyringPath.
type APTRepoPackage struct {
	KeyURL      string
	KeyringPath string
	RepoLine    string
	SourceFile  string
	Packages    []string
}

// InstallAPTRepoPackage runs the standard add-keyring → add-source → update →
// install sequence shared by most third-party APT components.
func InstallAPTRepoPackage(ctx context.Context, o Opts, spec APTRepoPackage) error {
	if err := AddAPTKeyring(ctx, o, spec.KeyURL, spec.KeyringPath); err != nil {
		return fmt.Errorf("add GPG key: %w", err)
	}
	if err := AddAPTSource(ctx, o, spec.RepoLine, spec.SourceFile); err != nil {
		return fmt.Errorf("add apt source: %w", err)
	}
	if err := APTUpdate(ctx, o); err != nil {
		return err
	}
	return InstallPackage(ctx, o, spec.Packages...)
}
