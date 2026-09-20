# Backups

ctdev backs a machine up with [restic](https://restic.net) on a daily systemd
timer. Snapshots are encrypted, tagged with the hostname, and sent to whatever
backend you point them at — Backblaze B2, S3, SFTP, or a local or USB path.

```bash
ctdev install restic            # restic, the timer, the backup/restore scripts
ctdev configure restic          # repository, credentials, password, init, enable
ctdev backup paths              # choose what to back up
ctdev backup now                # snapshot immediately
ctdev backup snapshots          # list this machine's snapshots
ctdev backup test               # check config, connection and paths
```

## Backups are opt-in

Nothing is snapshotted until you say what to include. `ctdev backup paths`
opens a local web UI that browses the filesystem with folder sizes and
include/exclude buttons. An **include** is a tree to back up; an **exclude**
carves junk out of an included tree — include `~/Repos`, exclude
`**/node_modules`. The picker listens on localhost; `--listen tailnet` serves it
on the machine's Tailscale address instead, for a node you only reach over SSH.

## What is stored where

`ctdev configure restic` writes `/etc/restic/restic.env` — the repository, its
backend credentials and the repository password. It is root-only and **never
committed**. The include and exclude lists live beside it in
`/etc/restic/backup-paths` and `/etc/restic/backup-excludes`.

Snapshots are tagged with the hostname, so each machine backs up to its own repo
and sees only its own snapshots. Retention is 7 daily, 4 weekly, 6 monthly.

Keep the repository password in your password manager. restic cannot restore
its own credentials, so a backup that has been restored is no use if the
password went down with the machine.

## Restoring

`ctdev restore ls|files|in-place|check` inspects and restores. The full
disaster-recovery runbook — rebuilding a machine from nothing — is
[RECOVERY.md](../RECOVERY.md).

## Pausing

`ctdev backup disable` pauses the scheduled runs and keeps the config and the
snapshots; `ctdev backup enable` resumes.
