#!/usr/bin/env bash
# restic-restore.sh — inspect and restore from this machine's restic repos.
# Reads /etc/restic/restic.env (root-only). Run as root.
#
# Usage:
#   restic-restore.sh snapshots [primary|local]            List this host's snapshots (newest last)
#   restic-restore.sh ls <snap|latest> [primary|local]     List files in a snapshot
#   restic-restore.sh restore <snap|latest> <dir> [repo]   Restore a snapshot INTO <dir> (safe)
#   restic-restore.sh restore-in-place [--yes] <snap|latest> [repo]
#                                                          Restore to original paths (/). DANGER.
#   restic-restore.sh check [primary|local]                Verify repository integrity
#
# repo defaults to 'primary' (RESTIC_REPOSITORY). Use 'local' for the optional
# second repo (RESTIC_REPOSITORY_LOCAL). Snapshots store absolute paths, so an
# in-place restore puts files back exactly where they came from.
set -uo pipefail

ENV_FILE=${RESTIC_ENV_FILE:-/etc/restic/restic.env}
if [ ! -r "$ENV_FILE" ]; then
	echo "!!! $ENV_FILE not readable — run as root and configure restic first (ctdev configure restic)" >&2
	exit 1
fi
set -a
# shellcheck source=/dev/null
source "$ENV_FILE"
set +a

# Under the restic-check systemd unit there is no $HOME; use the cache
# directory systemd provisions so checks don't re-download repo metadata.
if [ -z "${RESTIC_CACHE_DIR:-}" ] && [ -n "${CACHE_DIRECTORY:-}" ]; then
	export RESTIC_CACHE_DIR="$CACHE_DIRECTORY"
fi

usage() { sed -n '2,18p' "$0" >&2; exit 2; }

resolve_repo() {
	case "${1:-primary}" in
	primary) echo "${RESTIC_REPOSITORY:?RESTIC_REPOSITORY not set in restic.env}" ;;
	local) echo "${RESTIC_REPOSITORY_LOCAL:?RESTIC_REPOSITORY_LOCAL not set in restic.env}" ;;
	*)
		echo "unknown repo '$1' (use primary or local)" >&2
		exit 2
		;;
	esac
}

cmd=${1:-}
shift || true
case "$cmd" in
snapshots)
	# Scope to this host so you only see the machine you're on.
	exec restic -r "$(resolve_repo "${1:-primary}")" snapshots --host "$(hostname)"
	;;
ls)
	snap=${1:?snapshot id or 'latest' required}
	exec restic -r "$(resolve_repo "${2:-primary}")" ls "$snap"
	;;
restore)
	snap=${1:?snapshot id or 'latest' required}
	target=${2:?target directory required}
	repo=$(resolve_repo "${3:-primary}")
	mkdir -p "$target"
	exec restic -r "$repo" restore "$snap" --target "$target"
	;;
restore-in-place)
	# --yes skips the prompt for callers that already confirmed (ctdev runs
	# children without a stdin, so the read below would see EOF and abort).
	assume_yes=0
	if [ "${1:-}" = "--yes" ]; then
		assume_yes=1
		shift
	fi
	snap=${1:?snapshot id or 'latest' required}
	repo=$(resolve_repo "${2:-primary}")
	if [ "$assume_yes" -ne 1 ]; then
		echo "Restoring '$snap' to ORIGINAL paths — overwrites live files at their"
		echo "absolute locations. Stop any affected services/stacks first."
		printf "Type YES to continue: "
		read -r ans
		[ "$ans" = "YES" ] || {
			echo "aborted"
			exit 1
		}
	fi
	exec restic -r "$repo" restore "$snap" --target /
	;;
check)
	exec restic -r "$(resolve_repo "${1:-primary}")" check
	;;
*) usage ;;
esac
