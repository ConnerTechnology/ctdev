#!/usr/bin/env bash
# Keep the race/shuffle coverage and expose the slowest tests in each CI run.
set -euo pipefail
cd -- "$(dirname -- "$0")/../ctdev"

log=$(mktemp)
trap 'rm -f -- "$log"' EXIT
status=0
go test -race -vet=all -shuffle=on -timeout 20m -json ./... > "$log" || status=$?
jq -rs '
  [.[] | select((.Action == "fail" or .Action == "build-fail") and .Test == null) | .Package]
    as $failed |
  (.[] | select(.Action == "build-output" or
    (.Action == "output" and (.Package as $p | $failed | index($p)))) | .Output | rtrimstr("\n")),
  "",
  "Package timings:",
  ([.[] | select(.Test == null and (.Action == "pass" or .Action == "fail"))]
    | sort_by(-(.Elapsed // 0))[] | "  \(.Elapsed // 0)s  \(.Package)"),
  "",
  "Slowest individual tests:",
  ([.[] | select(.Test and (.Action == "pass" or .Action == "fail"))]
    | sort_by(-(.Elapsed // 0))[:10][] | "  \(.Elapsed // 0)s  \(.Package) \(.Test)")
' "$log"
exit "$status"
