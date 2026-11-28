#!/usr/bin/env bash
# The CI checks that can run on Linux, run locally, in containers.
#
# ctdev configures the machine it runs on, so nothing here runs ctdev, its installer or its Go
# tests on the host: the Go jobs and the Ubuntu install jobs run in throwaway containers that copy
# the tree in, and so does the PowerShell job where pwsh is not installed. Each run gets its own
# containers, so parallel worktrees never share one; only the Go module/build caches and the base
# tool images (Go + pinned restic/gpg/zsh, Ubuntu + git/sudo/curl) are cached locally.
#
# GitHub CI adds native macOS coverage on applicable PRs and all main pushes.
# A release requires successful CI on the exact tagged main commit.
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

# --changed decides which jobs the files can break. Guidance always runs: it takes a second, and a
# rename anywhere can break a link it checks.
want_shell=0 want_go=0 want_ubuntu=0 want_pwsh=0
if [ "$mode" = full ]; then
  want_shell=1 want_go=1 want_ubuntu=1 want_pwsh=1
else
  for f in "${files[@]}"; do
    case "${f#./}" in
      ctdev/*) want_go=1 want_ubuntu=1 ;;
      # Changes to the workflow or test tools can affect all container jobs.
      .github/workflows/*|.github/actions/*|scripts/checks.sh|scripts/install-test-restic.sh|scripts/ci-go-test.sh) want_shell=1 want_go=1 want_ubuntu=1 want_pwsh=1 ;;
      scripts/test-install.sh) want_go=1 want_shell=1 ;;
      scripts/test-uninstall.sh) want_ubuntu=1 want_shell=1 ;;
      install.sh|uninstall.sh|devcontainer.sh) want_go=1 want_shell=1 want_ubuntu=1 ;;
      scripts/*|.shellcheckrc) want_shell=1 ;;
      install.ps1) want_pwsh=1 ;;
    esac
  done
fi

# The shell scripts ci.yml lints. Keep in step with its shellcheck job.
shell_files=(install.sh uninstall.sh devcontainer.sh scripts/*.sh)

shell_job() {
  local f present=()
  for f in "${shell_files[@]}"; do [ -f "$f" ] && present+=("$f"); done
  for f in "${present[@]}"; do bash -n "$f" || return 1; done
  shellcheck "${present[@]}"
}

# Use the same Go toolchain as hosted CI and release builds.
go_base_image() {
  local v
  v=$(awk '$1 == "toolchain" { sub(/^go/, "", $2); print $2 }' ctdev/go.mod)
  [ -n "$v" ] || v=$(awk '$1 == "go" { print $2 }' ctdev/go.mod)
  printf 'golang:%s' "$v"
}

ubuntu_versions() {
  local versions
  versions=$(sed -n "s/.*ubuntu-version: *\[\(.*\)\].*/\1/p" .github/workflows/ci.yml | head -n1 | tr -d "'" | tr ',' ' ')
  [ -n "$versions" ] || { echo "could not read the Ubuntu matrix from ci.yml" >&2; return 1; }
  printf '%s' "$versions"
}

# Build a cached local Go test image with the same tools as the hosted Linux tests: pinned restic, gpg and zsh
# installed, and a non-root tester user so permission-boundary tests (which skip under root)
# actually run pre-merge instead of only failing after push.
go_test_image=""
ensure_go_image() {
  local base restic_script uid tag
  base=$(go_base_image)
  restic_script=$(sha256sum scripts/install-test-restic.sh) || return 1
  uid=$(id -u)
  [ "$uid" -gt 0 ] || uid=1000
  tag=$(printf 'v2:%s:%s:%s' "$base" "$restic_script" "$uid" | sha256sum | cut -c1-12)
  go_test_image="ctdev-checks-go:$tag"
  if docker image inspect "$go_test_image" >/dev/null 2>&1; then
    return 0
  fi
  say "building cached Go test image ($go_test_image)"
  docker build --quiet -t "$go_test_image" -f - scripts <<DOCKERFILE >/dev/null
FROM $base
COPY install-test-restic.sh /tmp/install-test-restic.sh
RUN apt-get update -qq >/dev/null \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends bzip2 gnupg zsh >/dev/null \
 && rm -rf /var/lib/apt/lists/* \
 && /tmp/install-test-restic.sh /tmp/ctdev-restic /usr/local/bin \
 && rm -rf /tmp/ctdev-restic /tmp/install-test-restic.sh \
 && useradd -m -u $uid -s /bin/bash tester \
 && mkdir -p /go/pkg/mod /home/tester/.cache/go-build /w \
 && chown -R tester:tester /go /home/tester /w
DOCKERFILE
}

# Build a cached Ubuntu base image with only the prerequisites ci.yml installs (git, sudo, curl),
# so each smoke run starts fresh without paying apt-get update/install overhead every time.
ensure_ubuntu_image() {
  local version=$1 img
  img="ctdev-checks-ubuntu-base:$version"
  if docker image inspect "$img" >/dev/null 2>&1; then
    return 0
  fi
  say "building cached Ubuntu base image ($img)"
  docker build --quiet -t "$img" - <<DOCKERFILE >/dev/null
FROM ubuntu:$version
RUN apt-get update -qq >/dev/null \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq git sudo curl >/dev/null
DOCKERFILE
}

bin_dir=""
logs_dir=""
cleanup() {
  [ -z "$bin_dir" ] || rm -rf "$bin_dir"
  [ -z "$logs_dir" ] || rm -rf "$logs_dir"
}
trap cleanup EXIT

# Build the static Linux binary first (~1-2s with warm caches) and run the quick CLI checks, so
# the Ubuntu smoke jobs can start immediately in parallel with go test.
go_build_job() {
  bin_dir=$(mktemp -d) || return 1
  docker run --rm --quiet --name "ctdev-checks-build-$$" \
    -v "$PWD":/src:ro -v "$bin_dir":/out \
    -v ctdev-checks-gomod:/go/pkg/mod -v ctdev-checks-gocache:/home/tester/.cache/go-build \
    -e GOCACHE=/home/tester/.cache/go-build \
    -e GOMODCACHE=/go/pkg/mod \
    "$go_test_image" bash -euo pipefail -c '
      tuid=$(id -u tester)
      [ "$(stat -c %u /go/pkg/mod)" = "$tuid" ] || chown -R tester:tester /go/pkg/mod
      [ "$(stat -c %u /home/tester/.cache/go-build)" = "$tuid" ] || chown -R tester:tester /home/tester/.cache/go-build
      chown tester:tester /out
      su -s /bin/bash tester -c '\''
        set -euo pipefail
        export PATH=/usr/local/go/bin:/go/bin:/usr/local/bin:/usr/bin:/bin
        export GOCACHE=/home/tester/.cache/go-build GOMODCACHE=/go/pkg/mod
        version=$(cat /src/VERSION)
        cd /src/ctdev
        CGO_ENABLED=0 go build -buildvcs=false -ldflags "-X main.version=$version" -o /out/ctdev .
        /out/ctdev --help >/dev/null
        /out/ctdev --version
        /out/ctdev info >/dev/null
        /out/ctdev --dry-run install jq >/dev/null
      '\''
    '
}

# The go-test + vet + cross-compile vet job, run as non-root tester with CI=1 and -shuffle=on.
go_test_job() {
  docker run --rm --quiet --name "ctdev-checks-go-$$" \
    -v "$PWD":/src:ro \
    -v ctdev-checks-gomod:/go/pkg/mod -v ctdev-checks-gocache:/home/tester/.cache/go-build \
    "$go_test_image" bash -euo pipefail -c '
      cp -a /src /w/repo
      chown -R tester:tester /w/repo
      tuid=$(id -u tester)
      [ "$(stat -c %u /go/pkg/mod)" = "$tuid" ] || chown -R tester:tester /go/pkg/mod
      [ "$(stat -c %u /home/tester/.cache/go-build)" = "$tuid" ] || chown -R tester:tester /home/tester/.cache/go-build
      su -s /bin/bash tester -c '\''
        set -euo pipefail
        export PATH=/usr/local/go/bin:/go/bin:/usr/local/bin:/usr/bin:/bin
        export GOCACHE=/home/tester/.cache/go-build GOMODCACHE=/go/pkg/mod CI=1
        restic version
        /w/repo/scripts/test-install.sh
        cd /w/repo/ctdev
        go test -race -vet=all -shuffle=on -timeout 20m ./...
        pids=()
        for t in darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
          GOOS="${t%/*}" GOARCH="${t#*/}" go vet ./... &
          pids+=("$!")
        done
        for p in "${pids[@]}"; do wait "$p"; done
      '\''
    '
}

# The test-ubuntu job: install two components into a fresh Ubuntu container.
ubuntu_job() {
  local version=$1
  [ -x "$bin_dir/ctdev" ] || { echo "no ctdev build: the build job failed" >&2; return 1; }
  docker run --rm --quiet --name "ctdev-checks-ubuntu-$version-$$" -v "$bin_dir":/bin-ctdev:ro \
    -v "$PWD/uninstall.sh":/src/uninstall.sh:ro -v "$PWD/scripts/test-uninstall.sh":/src/scripts/test-uninstall.sh:ro \
    "ctdev-checks-ubuntu-base:$version" bash -euo pipefail -c '
      bash /src/scripts/test-uninstall.sh
      cp /bin-ctdev/ctdev /usr/local/bin/ctdev
      ctdev --force install zsh >/dev/null
      command -v zsh >/dev/null
      [ -d ~/.oh-my-zsh ]
      ctdev --force install jq >/dev/null
      command -v jq >/dev/null
      ctdev info >/dev/null
    '
}

# The powershell-lint job: install.ps1 parses, and declares the -Doctor and -DoctorArgs parameters
# the documented `& ([scriptblock]::Create((irm ...))) -Doctor` needs. Parsing runs none of it. Where
# pwsh is not installed, the official PowerShell image runs it, with install.ps1 mounted read-only.
pwsh_image=mcr.microsoft.com/powershell:7.5-ubuntu-24.04
# shellcheck disable=SC2016 # PowerShell expands these, not bash
pwsh_check='$e = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile(
  (Resolve-Path ./install.ps1), [ref]$null, [ref]$e)
if ($e) { $e | ForEach-Object { Write-Error $_.Message }; exit 1 }
if (-not $ast.ParamBlock) { throw "install.ps1 must declare a param() block" }
$names = $ast.ParamBlock.Parameters.Name.VariablePath.UserPath
foreach ($required in @("Doctor", "DoctorArgs")) {
  if ($names -notcontains $required) { throw "install.ps1 is missing the -$required parameter" }
}'
pwsh_job() {
  if command -v pwsh >/dev/null; then
    pwsh -NoProfile -Command "$pwsh_check"
  else
    docker run --rm --quiet --name "ctdev-checks-pwsh-$$" -v "$PWD/install.ps1":/w/install.ps1:ro -w /w \
      "$pwsh_image" pwsh -NoProfile -Command "$pwsh_check"
  fi
}

needs_docker() {
  command -v docker >/dev/null && docker info >/dev/null 2>&1 && return 0
  echo "docker is required: the Go and Ubuntu jobs never run on the host, nor the PowerShell job without pwsh" >&2
  exit 1
}

[ "$want_go$want_ubuntu" = "00" ] || needs_docker
[ "$want_pwsh" = 0 ] || command -v pwsh >/dev/null || needs_docker

total_start=$(date +%s)
logs_dir=$(mktemp -d)
failed=()

# Prepare cached images and build the static binary before fanning out jobs in parallel.
if [ "$want_go" = 1 ] || [ "$want_ubuntu" = 1 ]; then
  ensure_go_image
fi
ubuntu_list=""
if [ "$want_ubuntu" = 1 ]; then
  ubuntu_list=$(ubuntu_versions)
  for v in $ubuntu_list; do
    ensure_ubuntu_image "$v"
  done
fi

build_ok=1
if [ "$want_go" = 1 ] || [ "$want_ubuntu" = 1 ]; then
  bstart=$(date +%s)
  say "go-build"
  if go_build_job; then
    say "go-build: ok ($(( $(date +%s) - bstart ))s)"
  else
    say "go-build: FAILED ($(( $(date +%s) - bstart ))s)"
    failed+=("go-build")
    build_ok=0
  fi
fi

# Fan out every remaining check in parallel; each records its own elapsed time and captures logs.
job_names=()
job_pids=()
launch() {
  local name=$1; shift
  local safe=${name// /_}
  job_names+=("$name")
  (
    start=$(date +%s)
    rc=0
    "$@" >"$logs_dir/$safe" 2>&1 || rc=$?
    printf '%d\n' "$(( $(date +%s) - start ))" >"$logs_dir/$safe.secs"
    exit "$rc"
  ) &
  job_pids+=("$!")
}

[ "$want_shell" = 0 ] || launch "shellcheck" shell_job
launch "guidance" scripts/check-guidance.sh
[ "$want_pwsh" = 0 ] || launch "powershell" pwsh_job
[ "$want_go" = 0 ] || launch "go" go_test_job
if [ "$want_ubuntu" = 1 ] && [ "$build_ok" = 1 ]; then
  for v in $ubuntu_list; do
    launch "ubuntu $v" ubuntu_job "$v"
  done
fi

for i in "${!job_pids[@]}"; do
  name="${job_names[$i]}"
  safe=${name// /_}
  if wait "${job_pids[$i]}"; then
    secs=$(cat "$logs_dir/$safe.secs" 2>/dev/null || echo "?")
    say "$name: ok (${secs}s)"
  else
    secs=$(cat "$logs_dir/$safe.secs" 2>/dev/null || echo "?")
    say "$name: FAILED (${secs}s)"
    cat "$logs_dir/$safe"
    failed+=("$name")
  fi
done

if [ ${#failed[@]} -gt 0 ]; then
  say "failed: ${failed[*]} ($(( $(date +%s) - total_start ))s total)"
  exit 1
fi
say "all checks passed ($(( $(date +%s) - total_start ))s total)"
