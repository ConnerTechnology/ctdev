#!/usr/bin/env bash
# restic-backup.sh — snapshot the homelab stacks and Docker volumes to the
# configured restic repos, then prune. Backs up offsite (B2) always and to the
# local USB repo whenever /mnt/backup is mounted (3-2-1). Run as root via the
# restic-backup.service systemd unit (needs to read Docker volume dirs).
#
# Reads /etc/restic/restic.env (root-only; NOT in the dotfiles repo) for the
# repo locations, B2 credentials, and RESTIC_PASSWORD_FILE.
set -uo pipefail

ENV_FILE=${RESTIC_ENV_FILE:-/etc/restic/restic.env}
if [ ! -r "$ENV_FILE" ]; then
	echo "!!! $ENV_FILE not readable — run as root and configure restic first" >&2
	exit 1
fi
set -a
# shellcheck source=/dev/null
source "$ENV_FILE"
set +a

USER_HOME=${RESTIC_USER_HOME:-/home/ctadmin}

# Stack dirs under $HOME plus Docker named-volume data dirs that exist.
BACKUP_PATHS=(
	"$USER_HOME/caddy"
	"$USER_HOME/pihole"
	"$USER_HOME/portainer"
	"$USER_HOME/beszel"
)
for v in caddy_caddy_data caddy_caddy_config portainer_portainer_data; do
	d="/var/lib/docker/volumes/$v/_data"
	if [ -d "$d" ]; then BACKUP_PATHS+=("$d"); fi
done

EXCLUDES=(
	--exclude "$USER_HOME/beszel/beszel_socket"
	--exclude "*.sock"
)
RETENTION=(--keep-daily 7 --keep-weekly 4 --keep-monthly 6)

overall=0

backup_to() {
	local repo="$1" label="$2" rc=0
	echo ">>> [$label] backup → $repo"
	restic -r "$repo" backup "${BACKUP_PATHS[@]}" "${EXCLUDES[@]}" \
		--host ctpi01 --tag homelab
	rc=$?
	# 0 = ok, 3 = completed but some source files were unreadable (non-fatal).
	if [ "$rc" -ne 0 ] && [ "$rc" -ne 3 ]; then
		echo "!!! [$label] backup failed (exit $rc)" >&2
		return "$rc"
	fi
	restic -r "$repo" forget "${RETENTION[@]}" --prune || return $?
	echo ">>> [$label] ok"
}

# Offsite (always).
backup_to "$RESTIC_REPO_B2" b2 || overall=1

# Local USB (only when the drive is mounted; auto-init on first run).
if mountpoint -q /mnt/backup; then
	mkdir -p "$RESTIC_REPO_LOCAL"
	if [ ! -f "$RESTIC_REPO_LOCAL/config" ]; then
		echo ">>> initializing local repo at $RESTIC_REPO_LOCAL"
		restic -r "$RESTIC_REPO_LOCAL" init || overall=1
	fi
	backup_to "$RESTIC_REPO_LOCAL" local || overall=1
else
	echo ">>> /mnt/backup not mounted — skipping local repo"
fi

exit "$overall"
