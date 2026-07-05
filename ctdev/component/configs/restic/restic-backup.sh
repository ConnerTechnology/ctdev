#!/usr/bin/env bash
# restic-backup.sh — snapshot this machine's configured paths to its restic repo,
# then prune. Backs up to the primary repo (RESTIC_REPOSITORY) always, and to an
# optional second repo (RESTIC_REPOSITORY_LOCAL) whenever it's reachable (3-2-1).
# Run as root via the restic-backup.service systemd unit (needs to read Docker
# volume dirs and other root-owned paths).
#
# Config (all root-only, NONE in the dotfiles repo):
#   /etc/restic/restic.env       repo locations, credentials, RESTIC_PASSWORD
#   /etc/restic/backup-paths     one path per line to snapshot ('#' comments)
#   /etc/restic/backup-excludes  optional restic --exclude patterns, one per line
# Seed these with `ctdev configure restic`.
set -uo pipefail

ENV_FILE=${RESTIC_ENV_FILE:-/etc/restic/restic.env}
PATHS_FILE=${RESTIC_PATHS_FILE:-/etc/restic/backup-paths}
EXCLUDES_FILE=${RESTIC_EXCLUDES_FILE:-/etc/restic/backup-excludes}

if [ ! -r "$ENV_FILE" ]; then
	echo "!!! $ENV_FILE not readable — run as root and configure restic first (ctdev configure restic)" >&2
	exit 1
fi
set -a
# shellcheck source=/dev/null
source "$ENV_FILE"
set +a

if [ -z "${RESTIC_REPOSITORY:-}" ]; then
	echo "!!! RESTIC_REPOSITORY not set in $ENV_FILE — run: ctdev configure restic" >&2
	exit 1
fi

# Read the snapshot paths (skip blanks and '#' comments). Only existing paths are
# kept, so a host that lists a stack dir it doesn't have doesn't fail the run.
BACKUP_PATHS=()
if [ -r "$PATHS_FILE" ]; then
	while IFS= read -r line; do
		line=${line%%#*}
		# Trim whitespace in pure bash — `echo | xargs` chokes on quotes in
		# paths (O'Brien/) and would silently drop that line.
		line="${line#"${line%%[![:space:]]*}"}"
		line="${line%"${line##*[![:space:]]}"}"
		[ -z "$line" ] && continue
		if [ -e "$line" ]; then
			BACKUP_PATHS+=("$line")
		else
			echo ">>> skipping missing path: $line"
		fi
	done <"$PATHS_FILE"
fi
if [ "${#BACKUP_PATHS[@]}" -eq 0 ]; then
	# Backups are opt-in: with nothing selected there's simply nothing to do.
	# Exit cleanly so the nightly timer doesn't log a failure before you pick.
	echo ">>> nothing to back up yet — choose folders with 'ctdev backup paths'"
	exit 0
fi

EXCLUDES=(--exclude-caches --exclude "*.sock")
if [ -r "$EXCLUDES_FILE" ]; then
	while IFS= read -r line; do
		line=${line%%#*}
		# Trim whitespace in pure bash — `echo | xargs` chokes on quotes in
		# paths (O'Brien/) and would silently drop that line.
		line="${line#"${line%%[![:space:]]*}"}"
		line="${line%"${line##*[![:space:]]}"}"
		[ -z "$line" ] && continue
		EXCLUDES+=(--exclude "$line")
	done <"$EXCLUDES_FILE"
fi

RETENTION=(--keep-daily 7 --keep-weekly 4 --keep-monthly 6)
HOSTTAG=$(hostname)

# The systemd unit runs without $HOME; point restic's cache at the directory
# systemd provisions (CacheDirectory=restic → $CACHE_DIRECTORY). Without a
# cache every run re-downloads repo metadata from the backend.
if [ -z "${RESTIC_CACHE_DIR:-}" ] && [ -n "${CACHE_DIRECTORY:-}" ]; then
	export RESTIC_CACHE_DIR="$CACHE_DIRECTORY"
fi

overall=0

backup_to() {
	local repo="$1" label="$2" rc=0
	# A backup killed mid-run (shutdown, suspend) leaves a stale lock that
	# blocks the next prune forever. `restic unlock` removes only locks whose
	# owning process is gone — safe on a repo only this host writes to.
	restic -r "$repo" unlock >/dev/null 2>&1 || true
	echo ">>> [$label] backup → $repo"
	restic -r "$repo" backup "${BACKUP_PATHS[@]}" "${EXCLUDES[@]}" \
		--host "$HOSTTAG" --tag ctdev
	rc=$?
	# 0 = ok, 3 = completed but some source files were unreadable (non-fatal).
	if [ "$rc" -ne 0 ] && [ "$rc" -ne 3 ]; then
		echo "!!! [$label] backup failed (exit $rc)" >&2
		return "$rc"
	fi
	restic -r "$repo" forget "${RETENTION[@]}" --prune || return $?
	echo ">>> [$label] ok"
}

# Primary repo (always).
backup_to "$RESTIC_REPOSITORY" primary || overall=1

# Optional second copy. For a local-filesystem repo, only run when its parent is a
# mountpoint (the external drive is attached); auto-init on first use. For remote
# repos (sftp:, s3:, b2:) just attempt it.
if [ -n "${RESTIC_REPOSITORY_LOCAL:-}" ]; then
	case "$RESTIC_REPOSITORY_LOCAL" in
	/*)
		parent=$(dirname "$RESTIC_REPOSITORY_LOCAL")
		if mountpoint -q "$parent" || mountpoint -q "$RESTIC_REPOSITORY_LOCAL"; then
			mkdir -p "$RESTIC_REPOSITORY_LOCAL"
			if [ ! -f "$RESTIC_REPOSITORY_LOCAL/config" ]; then
				echo ">>> initializing local repo at $RESTIC_REPOSITORY_LOCAL"
				restic -r "$RESTIC_REPOSITORY_LOCAL" init || overall=1
			fi
			backup_to "$RESTIC_REPOSITORY_LOCAL" local || overall=1
		else
			echo ">>> $parent not mounted — skipping local repo"
		fi
		;;
	*)
		backup_to "$RESTIC_REPOSITORY_LOCAL" local || overall=1
		;;
	esac
fi

# Optional dead-man's-switch: with HEALTHCHECK_URL set in restic.env (e.g. a
# healthchecks.io check URL), ping success or /fail so a silently-broken backup
# gets noticed instead of rotting for months. Never affects the exit code.
if [ -n "${HEALTHCHECK_URL:-}" ]; then
	ping_url="$HEALTHCHECK_URL"
	[ "$overall" -ne 0 ] && ping_url="$HEALTHCHECK_URL/fail"
	curl -fsS -m 10 --retry 3 -o /dev/null "$ping_url" || echo ">>> healthcheck ping failed (non-fatal)"
fi

exit "$overall"
