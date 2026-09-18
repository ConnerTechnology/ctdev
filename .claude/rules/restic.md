---
paths:
  - "ctdev/component/restic*.go"
  - "ctdev/component/configs/restic/**"
  - "ctdev/cmd/configure_restic.go"
  - "ctdev/cmd/backup*.go"
---
# restic backups

- `ctdev install restic` — restic backups with a daily systemd timer. Installs
  restic and deploys `/usr/local/bin/restic-backup.sh` (snapshots the paths listed
  in `/etc/restic/backup-paths` to the configured repo, optionally a second repo,
  then prunes 7d/4w/6m), `/usr/local/bin/restic-restore.sh` (a restore helper), and
  `restic-backup.{service,timer}`. Then run **`ctdev configure restic`** — it prompts
  for the repository (any backend: B2/S3/SFTP/local), credentials, and password,
  writes `/etc/restic/restic.env` (root-only, **never committed**), seeds default
  exclude patterns, runs `restic init`, and enables the timer. Backups are **opt-in**:
  nothing is snapshotted until you choose what to include with **`ctdev backup paths`** —
  a local web UI (localhost only) that browses the filesystem with folder sizes and
  include/exclude buttons, writing `/etc/restic/backup-paths` and `/etc/restic/backup-excludes`.
  (An `include` is a tree to back up; `exclude` globs/paths carve junk out of an included
  tree — e.g. include `~/Repos`, exclude `**/node_modules`.) Outside the timer,
  `ctdev backup now` snapshots immediately and
  `ctdev backup snapshots [primary|local]` lists this machine's snapshots (tagged by
  hostname); `ctdev restore …` inspects/restores. **Full restore runbook: `RECOVERY.md`.**
