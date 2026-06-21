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

// resticConfigured reports whether /etc/restic/restic.env exists (checked via
// sudo, since /etc/restic is root-only 0700). That file holds RESTIC_PASSWORD,
// the B2 credentials, and the repo locations.
func resticConfigured(ctx context.Context, o sysutil.Opts) bool {
	return sysutil.SudoRun(ctx, o, "test", "-f", "/etc/restic/restic.env") == nil
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
	fmt.Fprintln(opts.Stdout, "  Existing node (have the age key): decrypt the committed secrets into place —")
	fmt.Fprintln(opts.Stdout, "    sops -d ctdev/component/configs/restic/hosts/$(hostname).sops.env | sudo install -m600 /dev/stdin /etc/restic/restic.env")
	fmt.Fprintln(opts.Stdout, "  New node: create /etc/restic/restic.env (0600) with RESTIC_PASSWORD, B2_ACCOUNT_ID,")
	fmt.Fprintln(opts.Stdout, "    B2_ACCOUNT_KEY, RESTIC_REPO_B2, RESTIC_REPO_LOCAL, then:")
	fmt.Fprintln(opts.Stdout, "    sudo bash -c 'set -a; source /etc/restic/restic.env; restic -r \"$RESTIC_REPO_B2\" init'")
	fmt.Fprintln(opts.Stdout, "  Then: sudo systemctl enable --now restic-backup.timer  (or re-run 'ctdev install restic')")
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
