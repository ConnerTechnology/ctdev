#!/usr/bin/env bash
# The CI checks that can run on Linux, run locally, in containers.
#
# ctdev configures the machine it runs on, so nothing here runs ctdev, its installer or its Go
# tests on the host: the Go job and the Ubuntu install jobs run in throwaway containers that copy
# the tree in. Each run gets its own containers, so parallel worktrees never share one; only the
# Go module and build caches are shared, and Go makes those safe to share.
#
# The PowerShell and macOS jobs need their own runners and run only on GitHub, when the goal
# pull request is marked ready (scripts/goal.sh land).
#
# Usage:
#   scripts/checks.sh --full                 every Linux job in ci.yml
#   scripts/checks.sh --changed FILE...      only the jobs the named files can break
#   (run from anywhere inside the worktree to check; -C DIR checks another one)
set -euo pipefail

usage() { sed -n '/^# Usage:/,/^set /p' "$0" | sed '$d; s/^# \{0,1\}//' >&2; exit 2; }

dir=""
mode=""
files=()
while [ $# -gt 0 ]; do
  case "$1" in
    -C) dir="$2"; shift 2 ;;
    --full) mode=full; shift ;;
    --changed) mode=changed; shift; files=("$@"); break ;;
    -h|--help) usage ;;
    *) usage ;;
  esac
done
[ -n "$mode" ] || usage
cd "$(git -C "${dir:-.}" rev-parse --show-toplevel)"

say() { printf '==> %s\n' "$*"; }
failed=()
run() { # run <name> <command...>: time a job and keep going on failure, so one run lists every failure
  local name=$1 start; shift
  start=$(date +%s)
  say "$name"
  if "$@"; then
    say "$name: ok ($(( $(date +%s) - start ))s)"
  else
    say "$name: FAILED ($(( $(date +%s) - start ))s)"
    failed+=("$name")
  fi
}

# --changed decides which jobs the files can break. Guidance always runs: it takes a second, and a
# rename anywhere can break a link it checks.
want_shell=0 want_go=0 want_goal=0 want_ubuntu=0 want_pwsh=0
if [ "$mode" = full ]; then
  want_shell=1 want_go=1 want_goal=1 want_ubuntu=1 want_pwsh=1
else
  for f in "${files[@]}"; do
    case "${f#./}" in
      ctdev/*) want_go=1 want_ubuntu=1 ;;
      # ci.yml holds the shellcheck list, the restic pin and the Ubuntu matrix the jobs read, and a
      # change to this script can break any job it runs: both run every container job.
      .github/workflows/*|scripts/checks.sh) want_shell=1 want_go=1 want_goal=1 want_ubuntu=1 ;;
      install.sh|uninstall.sh|devcontainer.sh) want_shell=1 want_ubuntu=1 ;;
      scripts/goal*) want_shell=1 want_goal=1 ;;
      scripts/*|.shellcheckrc) want_shell=1 ;;
      install.ps1) want_pwsh=1 ;;
    esac
  done
fi

# The shell scripts ci.yml lints. Keep in step with its shellcheck job.
shell_files=(install.sh uninstall.sh devcontainer.sh scripts/check-guidance.sh scripts/checks.sh
  scripts/goal.sh scripts/goal-test.sh)

shell_job() {
  local f present=()
  for f in "${shell_files[@]}"; do [ -f "$f" ] && present+=("$f"); done
  for f in "${present[@]}"; do bash -n "$f" || return 1; done
  shellcheck "${present[@]}"
}

# The Go toolchain CI installs is the toolchain line of ctdev/go.mod (setup-go reads it), so the
# container uses the same release.
go_image() {
  local v
  v=$(awk '$1 == "toolchain" { sub(/^go/, "", $2); print $2 }' ctdev/go.mod)
  [ -n "$v" ] || v=$(awk '$1 == "go" { print $2 }' ctdev/go.mod)
  printf 'golang:%s' "$v"
}

# restic: the backup tests need a real one. ci.yml pins the release and its checksum; read both
# from there rather than keep a second copy that could drift.
restic_pin() {
  local url sum
  url=$(grep -o 'https://github.com/restic/restic/releases/download/[^ ]*_linux_amd64\.bz2' \
    .github/workflows/ci.yml | head -n1)
  sum=$(grep -oE '[0-9a-f]{64}  /tmp/restic\.bz2' .github/workflows/ci.yml | head -n1 | cut -d' ' -f1)
  if [ -z "$url" ] || [ -z "$sum" ]; then
    echo "could not read the restic pin from ci.yml" >&2; return 1
  fi
  printf '%s %s' "$url" "$sum"
}

bin_dir=""
cleanup() { [ -z "$bin_dir" ] || rm -rf "$bin_dir"; }
trap cleanup EXIT

# The go-test job, plus a static build the Ubuntu jobs reuse. The golang image has no bunzip2, so
# a few lines of Go's compress/bzip2 unpack restic. The container scripts use no && lists: under -e
# a failure before the last && does not stop the script.
go_job() {
  local pin
  pin=$(restic_pin) || return 1
  bin_dir=$(mktemp -d) || return 1
  docker run --rm --quiet --name "ctdev-checks-go-$$" \
    -v "$PWD":/src:ro -v "$bin_dir":/out \
    -v ctdev-checks-gomod:/go/pkg/mod -v ctdev-checks-gocache:/root/.cache/go-build \
    -e RESTIC_PIN="$pin" -e HOST_UID="$(id -u)" \
    "$(go_image)" bash -euo pipefail -c '
      read -r url sum <<<"$RESTIC_PIN"
      curl -fsSLo /tmp/restic.bz2 "$url"
      echo "$sum  /tmp/restic.bz2" | sha256sum -c - >/dev/null
      mkdir /tmp/bunzip2
      cat >/tmp/bunzip2/main.go <<"GO"
package main

import (
	"compress/bzip2"
	"io"
	"os"
)

func main() {
	if _, err := io.Copy(os.Stdout, bzip2.NewReader(os.Stdin)); err != nil {
		os.Stderr.WriteString("bunzip2: " + err.Error() + "\n")
		os.Exit(1)
	}
}
GO
      go run /tmp/bunzip2/main.go </tmp/restic.bz2 >/tmp/restic
      install -m 0755 /tmp/restic /usr/local/bin/restic
      restic version
      cp -a /src /w
      cd /w/ctdev
      go test -race -timeout 20m ./...
      go vet ./...
      for t in darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
        GOOS="${t%/*}" GOARCH="${t#*/}" go vet ./...
      done
      version=$(cat ../VERSION)
      CGO_ENABLED=0 go build -ldflags "-X main.version=$version" -o ctdev .
      ./ctdev --help >/dev/null
      ./ctdev --version
      ./ctdev info >/dev/null
      ./ctdev --dry-run install jq >/dev/null
      install -m 0755 -o "$HOST_UID" ctdev /out/ctdev
    '
}

# The test-ubuntu jobs: install two components into a stock Ubuntu container.
ubuntu_job() {
  local version=$1
  [ -x "$bin_dir/ctdev" ] || { echo "no ctdev build: the Go job failed" >&2; return 1; }
  docker run --rm --quiet --name "ctdev-checks-ubuntu-$version-$$" -v "$bin_dir":/bin-ctdev:ro \
    "ubuntu:$version" bash -euo pipefail -c '
      apt-get update -qq >/dev/null
      DEBIAN_FRONTEND=noninteractive apt-get install -y -qq git sudo curl >/dev/null
      cp /bin-ctdev/ctdev /usr/local/bin/ctdev
      ctdev --force install zsh >/dev/null
      command -v zsh >/dev/null
      [ -d ~/.oh-my-zsh ]
      ctdev --force install jq >/dev/null
      command -v jq >/dev/null
      ctdev info >/dev/null
    '
}

# Every release in ci.yml's test-ubuntu matrix, side by side; each one's output is printed whole.
ubuntu_both() {
  local versions v rc=0 logs pids=()
  versions=$(sed -n "s/.*ubuntu-version: *\[\(.*\)\].*/\1/p" .github/workflows/ci.yml | head -n1 | tr -d "'" | tr ',' ' ')
  [ -n "$versions" ] || { echo "could not read the Ubuntu matrix from ci.yml" >&2; return 1; }
  logs=$(mktemp -d)
  for v in $versions; do
    ubuntu_job "$v" >"$logs/$v" 2>&1 &
    pids+=("$!:$v")
  done
  for p in "${pids[@]}"; do
    if wait "${p%%:*}"; then
      say "ubuntu ${p#*:}: ok"
    else
      say "ubuntu ${p#*:}: FAILED"; cat "$logs/${p#*:}"; rc=1
    fi
  done
  rm -rf "$logs"
  return "$rc"
}

pwsh_job() {
  if ! command -v pwsh >/dev/null; then
    say "pwsh is not installed: install.ps1 is checked only on GitHub, when the goal is landed"
    return 0
  fi
  # shellcheck disable=SC2016 # PowerShell expands these, not bash
  pwsh -NoProfile -Command '$e = $null; [System.Management.Automation.Language.Parser]::ParseFile(
    (Resolve-Path ./install.ps1), [ref]$null, [ref]$e) | Out-Null; if ($e) { $e; exit 1 }'
}

needs_docker() {
  command -v docker >/dev/null && docker info >/dev/null 2>&1 && return 0
  echo "docker is required: the Go and Ubuntu jobs never run on the host" >&2
  exit 1
}

[ "$want_go$want_ubuntu" = "00" ] || needs_docker

[ "$want_shell" = 0 ] || run shellcheck shell_job
run guidance scripts/check-guidance.sh
# Skipped only where the tree has no goal tests yet; one that is there but not executable fails,
# as it would in ci.yml.
[ "$want_goal" = 0 ] || [ ! -e scripts/goal-test.sh ] || run goal-tests scripts/goal-test.sh
[ "$want_pwsh" = 0 ] || run powershell pwsh_job
if [ "$want_go" = 1 ] || [ "$want_ubuntu" = 1 ]; then
  run go go_job
fi
if [ "$want_ubuntu" = 1 ]; then
  # The two releases ci.yml's test-ubuntu matrix covers, run side by side.
  run ubuntu ubuntu_both
fi

if [ ${#failed[@]} -gt 0 ]; then
  say "failed: ${failed[*]}"
  exit 1
fi
say "all checks passed"
