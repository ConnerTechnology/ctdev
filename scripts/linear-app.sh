#!/bin/bash
# Gets a Linear token that acts as the Claude Code OAuth app, so what a Claude session does in
# Linear shows up as the app and notifies Thomas. A personal key acts as Thomas, and Linear
# doesn't notify you about your own actions. Copied from foil-it-up, where it was built (FIU-640);
# keep the two copies in step, because they share one token cache and one SCOPE.
#
# Usage:
#   scripts/linear-app.sh                     status only: where the token came from, its expiry
#                                             and scope. Never prints the token.
#   scripts/linear-app.sh --mcp-headers       the headersHelper in .mcp.json: prints
#                                             {"Authorization": "Bearer <token>"} for Claude Code
#   scripts/linear-app.sh graphql QUERY [VARIABLES_JSON]
#                                             sends one GraphQL request as the app and prints the
#                                             response, for the operations the Linear MCP has no
#                                             tool for (docs/agents/issue-tracker.md)
#
# The client id and secret come from ~/.secrets (LINEAR_CLAUDE_CLIENT_ID,
# LINEAR_CLAUDE_CLIENT_SECRET). The file is read here rather than taken from the environment
# because Claude Code strips every variable with SECRET, TOKEN or KEY in its name before it runs
# a project's headersHelper.
#
# Nothing written to stderr contains the token or the secret, and neither goes on a command line,
# where `ps` would show it.

set -euo pipefail

SECRETS_FILE="${HOME}/.secrets"
CACHE_DIR="${XDG_CACHE_HOME:-${HOME}/.cache}/linear-claude-code"
CACHE_FILE="${CACHE_DIR}/token.json"
TOKEN_URL="https://api.linear.app/oauth/token"
GRAPHQL_URL="https://api.linear.app/graphql"
# Asking for a different scope set revokes every token the app already has, so every caller
# must ask for exactly this one.
SCOPE="read,write,initiative:write"
# Tokens last 30 days. Refresh a day early so a long session never holds an expired one.
REFRESH_MARGIN=86400

die() {
  echo "linear-app: $*" >&2
  exit 1
}

# Pulls one field out of a JSON document on stdin, or prints nothing.
json_field() {
  jq -r --arg f "$1" '.[$f] // empty' 2>/dev/null
}

cached_field() {
  [[ -r "$CACHE_FILE" ]] || return 0
  json_field "$1" <"$CACHE_FILE"
}

# 0 when the cached token was granted exactly SCOPE and has at least REFRESH_MARGIN left. A token
# from before a scope change still passes Linear's viewer check, but lacks the new scope, so the
# scope has to be compared here. Linear reports scopes space-separated and in its own order.
sorted_scopes() {
  tr -s ', ' '\n' <<<"$1" | sed '/^$/d' | sort | paste -sd' '
}

cache_is_fresh() {
  local expires_at
  expires_at=$(cached_field expires_at)
  [[ "$(sorted_scopes "$(cached_field scope)")" == "$(sorted_scopes "$SCOPE")" ]] &&
    [[ "$expires_at" =~ ^[0-9]+$ ]] && ((expires_at - $(date +%s) > REFRESH_MARGIN))
}

# Quotes a value for a curl config file, where a backslash or a double quote inside quotes would
# otherwise be read as an escape or the end of the value.
curl_config_quote() {
  local value=${1//\\/\\\\}
  printf '"%s"' "${value//\"/\\\"}"
}

# Posts a GraphQL body with the given token and prints the response body, then the HTTP status
# on its own last line. The token reaches curl on stdin as config, not as an argument.
post_graphql() {
  local token=$1 body=$2 max_time=${3:-8}
  printf 'header = %s\n' "$(curl_config_quote "Authorization: Bearer ${token}")" |
    curl -sS --max-time "$max_time" -K - -w '\n%{http_code}' \
      -H 'Content-Type: application/json' --data-binary "$body" "$GRAPHQL_URL"
}

# 0 unless Linear says the cached token is no longer accepted. This costs one small request per
# connect, and it is what makes Claude Code's re-run of the helper after a 401 useful: a token
# revoked early (a scope change or a rotated secret revokes them all) is replaced, not handed
# back. A network failure keeps the cached token, because fetching a new one would fail the same
# way.
cached_token_is_accepted() {
  local response
  response=$(post_graphql "$1" '{"query":"{ viewer { id } }"}' 3 2>/dev/null) || return 0
  ! grep -q 'AUTHENTICATION_ERROR' <<<"$response" && [[ "${response##*$'\n'}" != 401 ]]
}

# Prints one variable that ~/.secrets exports, read in a subshell so nothing else leaks out.
secret_var() {
  # shellcheck source=/dev/null
  (set +u && source "$SECRETS_FILE" >/dev/null 2>&1 && printf '%s' "${!1:-}")
}

fetch_token() {
  [[ -r "$SECRETS_FILE" ]] || die "cannot read ${SECRETS_FILE}; it should export LINEAR_CLAUDE_CLIENT_ID and LINEAR_CLAUDE_CLIENT_SECRET"
  local client_id client_secret
  client_id=$(secret_var LINEAR_CLAUDE_CLIENT_ID)
  client_secret=$(secret_var LINEAR_CLAUDE_CLIENT_SECRET)
  [[ -n "$client_id" && -n "$client_secret" ]] || die "${SECRETS_FILE} does not set LINEAR_CLAUDE_CLIENT_ID and LINEAR_CLAUDE_CLIENT_SECRET"

  local response status body
  response=$(printf 'user = %s\n' "$(curl_config_quote "${client_id}:${client_secret}")" |
    curl -sS --max-time 6 -K - -w '\n%{http_code}' \
      --data-urlencode grant_type=client_credentials --data-urlencode "scope=${SCOPE}" \
      "$TOKEN_URL") || die "could not reach ${TOKEN_URL}"
  status=${response##*$'\n'}
  body=${response%$'\n'*}

  local error
  if [[ "$status" != 200 ]] ||
    ! jq -e '(.access_token | type == "string") and (.expires_in | type == "number")' <<<"$body" >/dev/null 2>&1; then
    error=$(jq -r '[.error, .error_description] | map(select(. != null)) | join(": ")' <<<"$body" 2>/dev/null || true)
    die "token request failed with HTTP ${status}${error:+ (${error})}"
  fi

  mkdir -p "$CACHE_DIR"
  chmod 700 "$CACHE_DIR"
  local tmp
  tmp=$(umask 077 && mktemp "${CACHE_DIR}/token.XXXXXX")
  # The token reaches jq on stdin, never as an argument.
  jq --argjson now "$(date +%s)" \
    '{access_token, expires_at: ($now + .expires_in), scope}' <<<"$body" >"$tmp"
  mv "$tmp" "$CACHE_FILE"
}

# Leaves a usable token in the cache and prints where it came from: "cache" or "fresh".
ensure_token() {
  if cache_is_fresh && cached_token_is_accepted "$(cached_field access_token)"; then
    echo cache
    return
  fi
  fetch_token
  echo fresh
}

command -v jq >/dev/null || die "jq is not installed"
command -v curl >/dev/null || die "curl is not installed"

case "${1:-}" in
  --mcp-headers)
    ensure_token >/dev/null
    jq '{Authorization: ("Bearer " + .access_token)}' <"$CACHE_FILE"
    ;;
  graphql)
    [[ -n "${2:-}" ]] || die "usage: scripts/linear-app.sh graphql QUERY [VARIABLES_JSON]"
    ensure_token >/dev/null
    body=$(jq -n --arg query "$2" --argjson variables "${3:-null}" '{query: $query, variables: $variables}') ||
      die "VARIABLES_JSON is not valid JSON"
    response=$(post_graphql "$(cached_field access_token)" "$body") || die "could not reach ${GRAPHQL_URL}"
    printf '%s\n' "${response%$'\n'*}"
    [[ "${response##*$'\n'}" == 200 ]] || die "GraphQL request failed with HTTP ${response##*$'\n'}"
    ;;
  "")
    origin=$(ensure_token)
    jq -n --arg origin "$origin" --arg cache "$CACHE_FILE" \
      --argjson private "$([[ -n $(find "$CACHE_FILE" -perm 600) ]] && echo true || echo false)" \
      --slurpfile c "$CACHE_FILE" \
      '{token_from: $origin, has_token: ($c[0].access_token | length > 0), scope: $c[0].scope,
        expires_at: ($c[0].expires_at | todate), cache: $cache, cache_readable_only_by_owner: $private}'
    ;;
  *)
    die "unknown argument '$1'; see the usage at the top of scripts/linear-app.sh"
    ;;
esac
