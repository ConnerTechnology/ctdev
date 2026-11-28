#!/usr/bin/env bash
# Run in a disposable Linux container before installing ctdev system-wide:
# uninstall.sh also checks /usr/local/bin regardless of INSTALL_DIR.
set -euo pipefail
if [[ -e /usr/local/bin/ctdev || -L /usr/local/bin/ctdev ]]; then
    echo "test-uninstall.sh requires a disposable container with no system installation" >&2
    exit 1
fi
repo=$(cd "$(dirname "$0")/.." && pwd)
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
mkdir -p "$fixture/bin"
cat > "$fixture/bin/systemctl" <<'SYSTEMCTL'
#!/bin/sh
exit 0
SYSTEMCTL
chmod +x "$fixture/bin/systemctl"
export PATH="$fixture/bin:$PATH"
for layout in default custom; do
    # Redirect the uninstaller's host paths into the fixture.
    export HOME="$fixture/$layout"
    export XDG_STATE_HOME="$HOME/state"
    export INSTALL_DIR="$HOME/bin"
    unset XDG_CONFIG_HOME
    roots=("$HOME/.config")
    if [[ "$layout" = custom ]]; then
        export XDG_CONFIG_HOME="$HOME/custom config"
        roots+=("$XDG_CONFIG_HOME")
    fi
    for config in "${roots[@]}"; do
        units="$config/systemd/user"
        mkdir -p "$units/timers.target.wants" "$config/ctdev/backup"
        touch "$units/ctdev-backup.timer" "$units/ctdev-backup.service" "$config/ctdev/config.yaml"
        ln -s ../ctdev-backup.timer "$units/timers.target.wants/ctdev-backup.timer"
        echo 'synthetic fixture' > "$config/ctdev/backup/example"
    done
    bash "$repo/uninstall.sh" >/dev/null
    for config in "${roots[@]}"; do
        units="$config/systemd/user"
        [[ ! -e "$config/ctdev/config.yaml" && -f "$config/ctdev/backup/example" ]]
        [[ ! -e "$units/ctdev-backup.timer" && ! -e "$units/ctdev-backup.service" ]]
        [[ ! -L "$units/timers.target.wants/ctdev-backup.timer" ]]
    done
    bash "$repo/uninstall.sh" --purge >/dev/null
    for config in "${roots[@]}"; do
        [[ ! -e "$config/ctdev" ]]
    done
done
