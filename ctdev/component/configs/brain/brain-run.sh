#!/usr/bin/env bash
# brain-run — the single entry point for every scheduled brain job.
#
#   brain-run sync      pull the checkout up to origin, push anything local
#   brain-run triage    pull, run the scheduled triage, commit + push what it wrote
#
# Deployed by `ctdev install brain`. Reads /etc/ctdev/brain.conf for every path
# and setting; holds no secrets. The Claude credential arrives from systemd as
# an encrypted credential, never from this file or the environment of the shell
# that started it.
#
# The single-writer property for memory/ lives here, and it is three rules:
#
#   1. Only one brain job touches the checkout at a time (flock).
#   2. Every run rebases onto origin immediately before it works, and pushes
#      immediately after, so the window where this node holds unpushed commits
#      is seconds rather than hours.
#   3. A rejected push or a conflicting rebase FAILS THE UNIT. It is never
#      forced, never auto-resolved, and never swallowed — `ctdev status` and
#      `systemctl --failed` are how divergence becomes visible instead of
#      quietly accumulating.
set -euo pipefail

CONF=/etc/ctdev/brain.conf
MODE="${1:-}"

die() { printf 'brain-run: %s\n' "$*" >&2; exit 1; }
log() { printf 'brain-run: %s\n' "$*"; }

case "$MODE" in
sync | triage) ;;
*) die "usage: brain-run <sync|triage>" ;;
esac

[[ -r "$CONF" ]] || die "$CONF is missing — run 'ctdev configure brain'"
# Values are shell-quoted by `ctdev configure brain`, so source them rather than
# using systemd's EnvironmentFile=, whose quoting rules are not the same.
set -a
# shellcheck disable=SC1090
. "$CONF"
set +a

REPO="${BRAIN_REPO:?BRAIN_REPO not set in $CONF}"
STATE="${BRAIN_STATE:-/var/lib/brain}"
BRANCH="${BRAIN_BRANCH:-main}"
GIT_NAME="${BRAIN_GIT_NAME:-brain}"
GIT_EMAIL="${BRAIN_GIT_EMAIL:-brain@localhost}"
RUNS="$STATE/runs"

[[ -d "$REPO/.git" ]] || die "$REPO is not a git checkout — run 'ctdev install brain'"

# Claude Code installs per-user into ~/.local/bin, which is not on systemd's
# default PATH.
export PATH="$HOME/.local/bin:/usr/local/bin:/usr/bin:/bin"

mkdir -p "$RUNS"

# --------------------------------------------------------------------- lock --
# Serializes the two timers against each other AND against a hand-run
# `brain-run`, which is what makes "one writer" true rather than aspirational.
exec 9>"$STATE/brain.lock"
if [[ "$MODE" == sync ]]; then
	# A sync that collides with a triage has nothing to add — the triage pulls
	# and pushes on its own. Skip quietly rather than queueing behind it.
	flock -n 9 || { log "another brain job holds the lock, skipping this sync"; exit 0; }
else
	flock -w 1800 9 || die "timed out waiting for the brain lock"
fi

# ---------------------------------------------------------------------- git --
git_c() { git -C "$REPO" "$@"; }

# rebase_onto_origin brings the checkout to origin/$BRANCH, keeping any local
# commits on top. A conflict aborts and fails: two disagreeing versions of a
# memory file need a human, and guessing is exactly the silent divergence this
# whole arrangement exists to prevent.
rebase_onto_origin() {
	if ! git_c fetch --quiet origin "$BRANCH"; then
		die "could not reach origin — is this node's deploy key on the repo, with write access?"
	fi
	if ! git_c rebase --autostash --quiet "origin/$BRANCH"; then
		git_c rebase --abort || true
		die "rebase onto origin/$BRANCH conflicted — resolve it by hand in $REPO"
	fi
}

# commit_local stages everything the run touched and commits it. Returns 1 when
# there was nothing to commit, which is the common case and not an error.
commit_local() {
	git_c add -A
	if git_c diff --cached --quiet; then
		return 1
	fi
	git_c -c "user.name=$GIT_NAME" -c "user.email=$GIT_EMAIL" \
		commit --quiet -m "chore: brain $MODE run on $(hostname -s)"
}

# push_local pushes, and on rejection rebases once and retries. Never --force:
# a force here would delete whatever a laptop pushed in between, which is the
# one outcome that turns "memory diverged" into "memory was lost".
push_local() {
	if git_c push --quiet origin "HEAD:$BRANCH"; then
		return 0
	fi
	log "push rejected — rebasing onto origin and retrying once"
	rebase_onto_origin
	git_c push --quiet origin "HEAD:$BRANCH" ||
		die "push still rejected after a rebase — $REPO holds unpushed commits"
}

log "rebasing $REPO onto origin/$BRANCH"
rebase_onto_origin

if [[ "$MODE" == sync ]]; then
	# A sync exists to keep the checkout current for whoever SSHes in, but it
	# also sweeps up anything a previous run committed and failed to push.
	if commit_local; then
		log "committed local changes found outside a scheduled run"
	fi
	if [[ -n "$(git_c log --oneline "origin/$BRANCH..HEAD")" ]]; then
		push_local
		log "pushed"
	fi
	exit 0
fi

# ------------------------------------------------------------------- triage --
hour=$(date +%H)
if ((10#$hour < 12)); then pass=morning; else pass=afternoon; fi
stamp=$(date +%Y-%m-%dT%H%M%S)
out="$RUNS/$stamp-triage.md"

# The prompt is a POINTER, never a copy of the rules. A prompt with rules baked
# into it is a snapshot that goes stale silently — which already happened once,
# on 2026-08-20, when a prompt written at 09:00 was applying superseded filing
# rules by 15:07. Everything below the per-run header is read fresh at run time.
#
# The checkout wins if it carries its own scheduled prompt, so the AI repo can
# own this text without a ctdev release.
if [[ -r "$REPO/scheduled/triage.md" ]]; then
	prompt_file="$REPO/scheduled/triage.md"
else
	prompt_file=/etc/ctdev/brain-triage.prompt
fi
[[ -r "$prompt_file" ]] || die "no triage prompt at $prompt_file"

command -v claude >/dev/null || die "claude is not on PATH for $(id -un)"

# The credential is delivered by systemd into a private ramfs directory that
# only this unit can read, and decrypted from a host-bound key. It is never in
# a file on disk in plaintext and never in this script's argv.
if [[ -n "${CREDENTIALS_DIRECTORY:-}" && -r "$CREDENTIALS_DIRECTORY/claude-oauth-token" ]]; then
	CLAUDE_CODE_OAUTH_TOKEN="$(cat "$CREDENTIALS_DIRECTORY/claude-oauth-token")"
	export CLAUDE_CODE_OAUTH_TOKEN
elif [[ -z "${CLAUDE_CODE_OAUTH_TOKEN:-}" ]]; then
	die "no Claude credential — run 'ctdev configure brain' to store one"
fi

prompt=$(
	printf 'Scheduled run: %s pass, %s, on %s.\n\n' "$pass" "$(date +%F)" "$(hostname -s)"
	cat "$prompt_file"
)

# --tools is a capability limit rather than a permission prompt: it removes
# Bash, web access, and everything else not named from the session that
# delegates. Verified against a live session, not assumed. The git work is done
# by this script, so the session never needs a shell, and the subagent it
# delegates to carries its own narrower allowlist in its frontmatter.
tools="${BRAIN_CLAUDE_TOOLS:-Task,Read,Glob,Grep,Write,Edit,TodoWrite}"
perm="${BRAIN_CLAUDE_PERMISSION_MODE:-bypassPermissions}"

# MCP is an ALLOW-LIST, not a deny-list, and that distinction is the point: a
# deny-list fails open. Add a server to the AI repo's mcp/servers.json a year
# from now and a deny-list would silently put it in reach of a session that
# handles attacker-controlled email. This names what the run needs, and
# --strict-mcp-config ignores every other registration.
#
# The config is filtered out of what setup.sh already registered, so no URL is
# duplicated here and a moved endpoint needs no change to this script.
mcp_allow="${BRAIN_CLAUDE_MCP:-email}"
mcp_cfg="$STATE/mcp-allowed.json"
python3 - "$HOME/.claude.json" "$mcp_cfg" "$mcp_allow" <<'PY' || die "could not build the MCP allow-list"
import json, sys

src, dst, allow = sys.argv[1], sys.argv[2], sys.argv[3].split(",")
try:
    servers = json.load(open(src)).get("mcpServers", {})
except Exception:
    servers = {}
keep = {name: cfg for name, cfg in servers.items() if name in allow}
missing = [name for name in allow if name not in keep]
if missing:
    # A triage with no mail server would report "no mailboxes" and look like a
    # quiet morning. Fail instead.
    sys.exit("MCP server(s) not registered for this user: " + ", ".join(missing))
json.dump({"mcpServers": keep}, open(dst, "w"))
PY

log "running the $pass triage; digest → $out"
status=0
(cd "$REPO" && claude -p "$prompt" \
	--permission-mode "$perm" \
	--tools "$tools" \
	--mcp-config "$mcp_cfg" \
	--strict-mcp-config \
	--output-format text) | tee "$out" || status=$?

if ((status != 0)); then
	# Still commit: an agent that updated memory and then hit an error has
	# produced learning that must not be stranded on this node.
	if commit_local; then push_local; fi
	die "claude exited $status — see $out"
fi

if commit_local; then
	push_local
	log "committed and pushed what the run wrote"
else
	log "run produced no repo changes"
fi

# Keep the run log bounded; the digests are a durable record, not an archive.
find "$RUNS" -name '*-triage.md' -type f -mtime +90 -delete 2>/dev/null || true

log "done"
