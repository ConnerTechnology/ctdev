package sysutil

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
)

// aptLockTimeout is how long ctdev waits for each apt lock. On a fresh VM's
// first boot unattended-upgrades and apt-daily hold them for a few minutes,
// and without a wait apt-get fails the moment it finds a lock taken.
const aptLockTimeout = 300 * time.Second

// aptLockRetryDelay spaces APTGet's retries while a lock apt-get can't wait
// on is held. A variable so tests can retry without waiting.
var aptLockRetryDelay = 10 * time.Second

// aptRetryLocks are the locks apt-get fails on at once, whatever
// DPkg::Lock::Timeout says: that option covers only the dpkg lock. `update`
// takes the lists lock and a download takes the archives lock. The paths are
// the one part of apt's lock message that isn't translated.
var aptRetryLocks = []string{"/var/lib/apt/lists/lock", "/var/cache/apt/archives/lock"}

// aptGetArgv is the one place ctdev builds an apt-get command line, so every
// call runs unattended the same way. ctdev runs apt-get from boot scripts and
// progress screens with no TTY behind them, where a debconf question or a
// conffile prompt would hang the run forever.
//
// `env` carries DEBIAN_FRONTEND across sudo, which resets the environment.
// --force-confold keeps a config file the user edited when a package ships a
// new one, which is the answer the prompt would default to; it only matters
// for the subcommands that unpack packages.
func aptGetArgv(args ...string) []string {
	argv := []string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get",
		"-o", "DPkg::Lock::Timeout=" + strconv.Itoa(int(aptLockTimeout.Seconds())),
		"-o", "Acquire::Retries=3"}
	if len(args) > 0 {
		switch args[0] {
		case "install", "upgrade", "dist-upgrade", "full-upgrade":
			argv = append(argv, "-o", "Dpkg::Options::=--force-confold")
		}
	}
	return append(argv, args...)
}

// APTGet runs apt-get as root with the unattended options from aptGetArgv.
// args start with the subcommand: APTGet(ctx, o, "install", "-y", "jq").
//
// apt-get waits on the dpkg lock by itself; for the locks it can't wait on
// (aptRetryLocks) APTGet reruns the command until it gets through or
// aptLockTimeout passes. Any other failure returns straight away.
func APTGet(ctx context.Context, o Opts, args ...string) error {
	argv := aptGetArgv(args...)
	deadline := time.Now().Add(aptLockTimeout)
	for {
		var out bytes.Buffer
		attempt := o
		attempt.Stdout = &out
		if o.Stdout != nil {
			attempt.Stdout = io.MultiWriter(o.Stdout, &out)
		}
		err := SudoRun(ctx, attempt, argv[0], argv[1:]...)
		if err == nil || !heldAPTLock(out.String()) || time.Now().After(deadline) {
			return err
		}
		if o.Stdout != nil {
			fmt.Fprintln(o.Stdout, "Another apt run holds an apt lock; waiting for it...")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(aptLockRetryDelay):
		}
	}
}

// heldAPTLock reports whether apt-get's output says it failed on one of the
// locks APTGet retries.
func heldAPTLock(out string) bool {
	for _, lock := range aptRetryLocks {
		if strings.Contains(out, lock) {
			return true
		}
	}
	return false
}

// EnsureGPG installs gnupg when gpg is missing. Minimal images (Debian cloud
// and container images) don't ship it, and they may have no package index
// either, so on apt the index is refreshed first.
func EnsureGPG(ctx context.Context, o Opts) error {
	if CommandExists("gpg") {
		return nil
	}
	if platform.Detect().PackageManager == "apt" {
		if err := APTUpdate(ctx, o); err != nil {
			return err
		}
	}
	if err := InstallPackage(ctx, o, "gnupg"); err != nil {
		return fmt.Errorf("install gnupg: %w", err)
	}
	return nil
}

// AddAPTKeyring downloads a GPG key and installs it as an APT keyring.
func AddAPTKeyring(ctx context.Context, o Opts, url, keyringPath string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download GPG key %s → %s\n", url, keyringPath)
		return nil
	}
	// The key is dearmored with gpg. Installing gpg when it's missing keeps
	// every keyring in the binary format, at the path existing sources
	// already reference.
	if err := EnsureGPG(ctx, o); err != nil {
		return err
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

// AddAPTSource writes an APT sources list entry. World-readable: `ctdev
// update` scans with `apt list --upgradable` as the user, which skips a
// source it can't read, so the repo's upgrades never show.
func AddAPTSource(ctx context.Context, o Opts, line, filename string) error {
	return SudoWriteFileMode(ctx, o, line+"\n", "/etc/apt/sources.list.d/"+filename, "0644")
}

// APTUpdate runs apt-get update.
func APTUpdate(ctx context.Context, o Opts) error {
	return APTGet(ctx, o, "update", "-qq")
}

// RemoveAPTRepo removes a third-party APT source list and its signing keyring —
// the inverse of the keyring+source setup InstallAPTRepoPackage performs. Left
// behind, the dangling repo makes `apt-get update` keep hitting a now-removed
// source and keeps an orphaned signing key trusted. sourceFile is the bare
// filename under /etc/apt/sources.list.d (e.g. "github-cli.list"); keyringPath
// is the absolute keyring path, or "" to skip key removal. Both removals are
// best-effort (`rm -f`); a missing file is not an error.
func RemoveAPTRepo(ctx context.Context, o Opts, sourceFile, keyringPath string) error {
	if err := SudoRun(ctx, o, "rm", "-f", "/etc/apt/sources.list.d/"+sourceFile); err != nil {
		return err
	}
	if keyringPath != "" {
		return SudoRun(ctx, o, "rm", "-f", keyringPath)
	}
	return nil
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
