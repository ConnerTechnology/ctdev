#!/usr/bin/env bash
# restic-restore.sh — inspect and restore from the homelab restic repos.
# Reads /etc/restic/restic.env (root-only). Run as root.
#
# Usage:
#   restic-restore.sh snapshots [b2|local]              List snapshots (newest last)
#   restic-restore.sh ls <snap|latest> [b2|local]       List files in a snapshot
#   restic-restore.sh restore <snap|latest> <dir> [b2|local]
#                                                       Restore a snapshot INTO <dir> (safe)
#   restic-restore.sh restore-in-place <snap|latest> [b2|local]
#                                                       Restore to original paths (/). DANGER.
#   restic-restore.sh check [b2|local]                  Verify repository integrity
#
# repo defaults to 'b2' (offsite, always reachable). Use 'local' for the USB
# repo at /mnt/backup. Snapshots store absolute paths, so an in-place restore
# puts files back exactly where they came from.
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

usage() { sed -n '2,18p' "$0" >&2; exit 2; }

resolve_repo() {
	case "${1:-b2}" in
	b2) echo "$RESTIC_REPO_B2" ;;
	local) echo "$RESTIC_REPO_LOCAL" ;;
	*)
		echo "unknown repo '$1' (use b2 or local)" >&2
		exit 2
		;;
	esac
}

cmd=${1:-}
shift || true
case "$cmd" in
snapshots)
	exec restic -r "$(resolve_repo "${1:-b2}")" snapshots
	;;
ls)
	snap=${1:?snapshot id or 'latest' required}
	exec restic -r "$(resolve_repo "${2:-b2}")" ls "$snap"
	;;
restore)
	snap=${1:?snapshot id or 'latest' required}
	target=${2:?target directory required}
	repo=$(resolve_repo "${3:-b2}")
	mkdir -p "$target"
	exec restic -r "$repo" restore "$snap" --target "$target"
	;;
restore-in-place)
	snap=${1:?snapshot id or 'latest' required}
	repo=$(resolve_repo "${2:-b2}")
	echo "Restoring '$snap' to ORIGINAL paths — overwrites live files under"
	echo "/home/ctadmin and /var/lib/docker/volumes. Stop the stacks first."
	printf "Type YES to continue: "
	read -r ans
	[ "$ans" = "YES" ] || {
		echo "aborted"
		exit 1
	}
	exec restic -r "$repo" restore "$snap" --target /
	;;
check)
	exec restic -r "$(resolve_repo "${1:-b2}")" check
	;;
*) usage ;;
esac
