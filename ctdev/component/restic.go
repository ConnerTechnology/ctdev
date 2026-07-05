package component

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// restic installs the restic backup tool plus a per-machine backup script, a
// restore helper, and a daily systemd timer. The backup script (run as root)
// snapshots the paths listed in /etc/restic/backup-paths to the repo configured
// in /etc/restic/restic.env, then prunes (keep 7 daily / 4 weekly / 6 monthly).
//
// All restic config lives in /etc/restic/ (root-only; NEVER committed) and is
// written by `ctdev configure restic`. The timer is enabled only once that
// config is present. See RECOVERY.md for the full restore runbook.

const (
	// ResticEnvPath holds RESTIC_REPOSITORY, RESTIC_PASSWORD, and any backend
	// credentials. Root-only 0600.
	ResticEnvPath = "/etc/restic/restic.env"
	// ResticPathsFile lists the paths to snapshot (one per line, '#' comments).
	ResticPathsFile = "/etc/restic/backup-paths"
	// ResticExcludesFile lists optional restic --exclude patterns, one per line.
	ResticExcludesFile = "/etc/restic/backup-excludes"
)

// resticDeploy writes an embedded config file to a root-owned path and chmods it.
func resticDeploy(ctx context.Context, o sysutil.Opts, src, dest, mode string) error {
	b, err := Configs.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read embedded %s: %w", src, err)
	}
	if err := sysutil.SudoWriteFile(ctx, o, string(b), dest); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	return sysutil.SudoRun(ctx, o, "chmod", mode, dest)
}

// ResticConfigured reports whether /etc/restic/restic.env exists (checked via
// sudo, since /etc/restic is root-only 0700).
func ResticConfigured(ctx context.Context, o sysutil.Opts) bool {
	return sysutil.SudoRun(ctx, o, "test", "-f", ResticEnvPath) == nil
}

// ResticReadEnv returns the key/value pairs in /etc/restic/restic.env (read via
// sudo), or an empty map when it's absent/unreadable.
func ResticReadEnv(ctx context.Context) map[string]string {
	out, err := captureOutput(ctx, "sudo", "cat", ResticEnvPath)
	if err != nil {
		return map[string]string{}
	}
	return parseEnv(out)
}

// ResticReadLines returns the non-blank, non-comment lines of a root-owned
// restic config file (read via sudo), e.g. backup-paths or backup-excludes.
// Returns nil when the file is absent/unreadable.
func ResticReadLines(ctx context.Context, path string) []string {
	out, err := captureOutput(ctx, "sudo", "cat", path)
	if err != nil {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// ResticWriteEnv renders env as a managed dotenv and writes it to
// /etc/restic/restic.env (0600, root). Keys are sorted for a stable diff.
func ResticWriteEnv(ctx context.Context, o sysutil.Opts, env map[string]string) error {
	if err := sysutil.SudoRun(ctx, o, "install", "-d", "-m", "700", "/etc/restic"); err != nil {
		return err
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		if env[k] != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# managed by `ctdev configure restic` — do not commit\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, env[k])
	}
	if err := sysutil.SudoWriteFile(ctx, o, b.String(), ResticEnvPath); err != nil {
		return err
	}
	return sysutil.SudoRun(ctx, o, "chmod", "0600", ResticEnvPath)
}

// ResticFileExists reports whether a root-owned path exists (via sudo).
func ResticFileExists(ctx context.Context, o sysutil.Opts, path string) bool {
	return sysutil.SudoRun(ctx, o, "test", "-e", path) == nil
}

// ResticSeedFile writes content to a root-owned path only if it doesn't already
// exist, so re-running configure never clobbers an edited backup-paths list.
// Returns whether it wrote the file.
func ResticSeedFile(ctx context.Context, o sysutil.Opts, path, content string) (bool, error) {
	if ResticFileExists(ctx, o, path) {
		return false, nil
	}
	if err := sysutil.SudoWriteFile(ctx, o, content, path); err != nil {
		return false, err
	}
	return true, sysutil.SudoRun(ctx, o, "chmod", "0600", path)
}

// ResticInitIfNeeded initializes the primary repo unless it's already a restic
// repository. It sources the env file so the backend credentials are in scope.
func ResticInitIfNeeded(ctx context.Context, o sysutil.Opts) error {
	script := fmt.Sprintf(
		"set -a; . %s; set +a; restic cat config >/dev/null 2>&1 || restic init",
		ResticEnvPath)
	return sysutil.SudoRun(ctx, o, "bash", "-c", script)
}

// ResticEnableTimer enables and starts the daily backup timer.
func ResticEnableTimer(ctx context.Context, o sysutil.Opts) error {
	return sysutil.SudoRun(ctx, o, "systemctl", "enable", "--now", "restic-backup.timer")
}

// DefaultBackupExcludes returns conservative exclude patterns seeded alongside
// the paths file. Caches and sockets are never worth backing up.
func DefaultBackupExcludes() []string {
	return []string{"**/.cache", "**/Trash", "*.sock", "**/node_modules"}
}

func resticInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if p.PackageManager != "apt" {
		return unsupportedPMError("restic", p.PackageManager)
	}

	// Phase 1: install the binary (skip if present unless --force).
	if opts.Force || !sysutil.CommandExists("restic") {
		if err := sysutil.InstallPackage(ctx, o, "restic"); err != nil {
			return err
		}
	}

	if o.DryRun {
		fmt.Fprintln(o.Stdout, "[dry-run] deploy restic backup script, restore helper, and daily systemd timer")
		return nil
	}

	// Phase 2: deploy the scripts and systemd units.
	if err := resticDeploy(ctx, o, "configs/restic/restic-backup.sh", "/usr/local/bin/restic-backup.sh", "0755"); err != nil {
		return err
	}
	if err := resticDeploy(ctx, o, "configs/restic/restic-restore.sh", "/usr/local/bin/restic-restore.sh", "0755"); err != nil {
		return err
	}
	if err := resticDeploy(ctx, o, "configs/restic/restic-backup.service", "/etc/systemd/system/restic-backup.service", "0644"); err != nil {
		return err
	}
	if err := resticDeploy(ctx, o, "configs/restic/restic-backup.timer", "/etc/systemd/system/restic-backup.timer", "0644"); err != nil {
		return err
	}
	if err := sysutil.SudoRun(ctx, o, "systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := sysutil.SudoRun(ctx, o, "install", "-d", "-m", "700", "/etc/restic"); err != nil {
		return err
	}

	// Enable the daily timer only once credentials are in place, otherwise it
	// would just fail every night.
	if ResticConfigured(ctx, o) {
		if err := ResticEnableTimer(ctx, o); err != nil {
			return err
		}
		fmt.Fprintln(opts.Stdout, "restic configured — daily backup timer enabled.")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "restic installed. Configure backups with:")
	fmt.Fprintln(opts.Stdout, "  ctdev configure restic")
	fmt.Fprintln(opts.Stdout, "It prompts for the repo and credentials, writes /etc/restic/restic.env,")
	fmt.Fprintln(opts.Stdout, "seeds default excludes, runs `restic init`, and enables the daily timer.")
	fmt.Fprintln(opts.Stdout, "Then pick what to back up with: ctdev backup paths")
	return nil
}

func resticUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	_ = sysutil.SudoRun(ctx, o, "systemctl", "disable", "--now", "restic-backup.timer")
	for _, f := range []string{
		"/etc/systemd/system/restic-backup.timer",
		"/etc/systemd/system/restic-backup.service",
		"/usr/local/bin/restic-backup.sh",
		"/usr/local/bin/restic-restore.sh",
	} {
		_ = sysutil.SudoRun(ctx, o, "rm", "-f", f)
	}
	_ = sysutil.SudoRun(ctx, o, "systemctl", "daemon-reload")
	fmt.Fprintln(opts.Stdout, "restic backup units removed. /etc/restic/ kept (holds your repo password — keep it safe!).")
	return sysutil.RemovePackage(ctx, o, "restic")
}
