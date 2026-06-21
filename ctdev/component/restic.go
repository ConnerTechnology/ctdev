package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// restic installs the restic backup tool plus the homelab backup script, a
// restore helper, and a daily systemd timer. The backup script (run as root)
// snapshots the stack dirs under $HOME and the Docker named-volume data dirs to
// an offsite B2 repo and, when /mnt/backup is mounted, a local USB repo, then
// prunes (keep 7 daily / 4 weekly / 6 monthly).
//
// Repo locations, B2 credentials, and the repository password live in
// /etc/restic/ (root-only; NEVER committed). The timer is enabled only once
// that config is present. See RECOVERY.md for the full restore runbook.

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

// resticConfigured reports whether /etc/restic holds both the env file and the
// password (checked via sudo, since /etc/restic is root-only 0700).
func resticConfigured(ctx context.Context, o sysutil.Opts) bool {
	if err := sysutil.SudoRun(ctx, o, "test", "-f", "/etc/restic/restic.env"); err != nil {
		return false
	}
	return sysutil.SudoRun(ctx, o, "test", "-f", "/etc/restic/password") == nil
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

	// Enable the daily timer only once credentials/password are in place,
	// otherwise it would just fail every night.
	if resticConfigured(ctx, o) {
		if err := sysutil.SudoRun(ctx, o, "systemctl", "enable", "--now", "restic-backup.timer"); err != nil {
			return err
		}
		fmt.Fprintln(opts.Stdout, "restic configured — daily backup timer enabled.")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "restic installed. Finish setup to enable backups (see RECOVERY.md):")
	fmt.Fprintln(opts.Stdout, "  1. Write a strong random password to /etc/restic/password (sudo, chmod 600)")
	fmt.Fprintln(opts.Stdout, "     and save a copy in your password manager — losing it makes the backups unrecoverable.")
	fmt.Fprintln(opts.Stdout, "  2. Create /etc/restic/restic.env (0600) with RESTIC_PASSWORD_FILE, B2_ACCOUNT_ID, B2_ACCOUNT_KEY, RESTIC_REPO_B2, RESTIC_REPO_LOCAL")
	fmt.Fprintln(opts.Stdout, "  3. sudo bash -c 'source /etc/restic/restic.env; restic -r \"$RESTIC_REPO_B2\" init'")
	fmt.Fprintln(opts.Stdout, "  4. sudo systemctl enable --now restic-backup.timer")
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
