#!/usr/bin/env bash
# Exercise the standalone bootstrap on fixture release files, including on a
# PATH that only offers the native macOS SHA-256 verifier.
set -euo pipefail
installer=$(cd "$(dirname "$0")/.." && pwd)/install.sh
scratch=$(mktemp -d)
trap 'rm -rf -- "$scratch"' EXIT
export FIXTURES="$scratch/release"
export INSTALL_DIR="$scratch/bin with spaces" CTDEV_VERSION=v1.2.3
unset CTDEV_SKIP_VERIFY
mkdir -p "$FIXTURES" "$scratch/tools"

case "$(uname -m)" in
    arm64|aarch64) arch=arm64 ;;
    amd64|x86_64) arch=amd64 ;;
    *) exit 1 ;;
esac
asset="ctdev-$(uname -s | tr '[:upper:]' '[:lower:]')-$arch"
cat > "$FIXTURES/$asset" <<'BINARY'
#!/bin/sh
printf '%s\n' "$0" "$@"
BINARY

# Isolate PATH so the downloader stays offline and each verifier runs alone.
for tool in awk bash chmod mkdir mktemp mv rm tr uname; do
    ln -s "$(command -v "$tool")" "$scratch/tools/$tool"
done
cat > "$scratch/tools/curl" <<'CURL'
#!/bin/sh
set -eu
[ "$1" = -fsSL ] && [ "$2" = -o ] || exit 1
file=${4##*/}
printf '%s\n' "$file" >> "$FIXTURES/downloads"
/bin/cat "$FIXTURES/$file" > "$3"
CURL
chmod +x "$scratch/tools/curl"

tested=0
for verifier in shasum sha256sum; do
    command -v "$verifier" >/dev/null || continue
    tested=1
    ln -s "$(command -v "$verifier")" "$scratch/tools/$verifier"
    hash=("$verifier")
    if [[ $verifier == shasum ]]; then hash+=(-a 256); fi
    digest=$("${hash[@]}" < "$FIXTURES/$asset")
    printf '%s  %s\n' "${digest%% *}" "$asset" > "$FIXTURES/SHA256SUMS"

    PATH="$scratch/tools" bash "$installer" >/dev/null
    cmp "$FIXTURES/$asset" "$INSTALL_DIR/ctdev"
    : > "$FIXTURES/downloads"
    PATH="$scratch/tools" bash "$installer" >/dev/null
    [[ $(cat "$FIXTURES/downloads") == SHA256SUMS ]]

    PATH="$scratch/tools" bash "$installer" --doctor --deep > "$scratch/report"
    ephemeral=$(head -n1 "$scratch/report")
    [[ $ephemeral != "$INSTALL_DIR/ctdev" && ! -e $ephemeral ]]
    [[ $(tail -n +2 "$scratch/report") == $'doctor\n--deep' ]]

    printf '%064d  %s\n' 0 "$asset" > "$FIXTURES/SHA256SUMS"
    if PATH="$scratch/tools" bash "$installer" > "$scratch/error" 2>&1; then
        echo "installer accepted a mismatched checksum" >&2
        exit 1
    fi
    grep -q 'Checksum mismatch' "$scratch/error"
    cmp "$FIXTURES/$asset" "$INSTALL_DIR/ctdev"
    rm "$INSTALL_DIR/ctdev" "$scratch/tools/$verifier"
done
[[ $tested == 1 ]]
