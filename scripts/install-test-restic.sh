#!/usr/bin/env bash
# Install the same checksum-pinned restic for local and hosted tests.
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: $0 ARCHIVE_DIRECTORY BINARY_DIRECTORY" >&2
  exit 2
fi

version=0.19.1
cache=$1
bin=$2
case "$(uname -s)/$(uname -m)" in
  Linux/x86_64)
    platform=linux_amd64
    sum=f415415624dcc452f2a02b8c33641791a8c6d6d3b65bbb3543fcf9a25151585c
    ;;
  Linux/aarch64)
    platform=linux_arm64
    sum=a5f64aaab53d51e311fa3829124c5b703f2d14cf187d8640b6be3b2b49376465
    ;;
  Darwin/x86_64)
    platform=darwin_amd64
    sum=c38d579622cf602f665234c5a8c315030b6cf70656028fe6dc29a786b60e5f35
    ;;
  Darwin/arm64)
    platform=darwin_arm64
    sum=7be0a144ccc377880f294204aa271d76e4b79554b42a751151d425ce6ebac143
    ;;
  *)
    echo "unsupported restic test platform: $(uname -s)/$(uname -m)" >&2
    exit 1
    ;;
esac

asset="restic_${version}_${platform}.bz2"
mkdir -p "$cache" "$bin"
archive="$cache/$asset"
check_archive() {
  # Hash stdin so filenames with spaces work on both GNU and BSD systems.
  local actual
  if command -v sha256sum >/dev/null; then
    actual=$(sha256sum < "$1") || return 1
  else
    actual=$(shasum -a 256 < "$1") || return 1
  fi
  [ "${actual%% *}" = "$sum" ]
}

if [ ! -f "$archive" ] || ! check_archive "$archive"; then
  download=$(mktemp "$cache/.restic.XXXXXX")
  trap 'rm -f -- "$download"' EXIT
  curl -fsSL --retry 4 --retry-max-time 90 --connect-timeout 10 --max-time 30 \
    "https://github.com/restic/restic/releases/download/v$version/$asset" -o "$download"
  if ! check_archive "$download"; then
    echo "restic archive checksum mismatch for $asset" >&2
    exit 1
  fi
  mv -f -- "$download" "$archive"
fi

bzip2 -dc -- "$archive" > "$bin/restic"
chmod +x "$bin/restic"
"$bin/restic" version
