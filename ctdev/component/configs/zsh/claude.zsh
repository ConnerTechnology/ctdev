# Run every Claude Code session inside a systemd scope with a hard memory cap.
# Everything the session spawns (test suites included) shares the cap, so a
# runaway `vitest` is OOM-killed inside the scope and the desktop keeps running.
#
# The cap is a fixed target per machine size (see the table below). Set
# CLAUDE_MEMORY_MAX (e.g. 32G) in exports.local.zsh to pick a number by hand.
# Sessions already open are not covered until they are restarted.
#
# Check it took: systemctl --user status "run-*.scope" | grep -E "scope|Memory"

# Installed RAM in GB -> cap. Roughly half on small machines, where the desktop's
# fixed baseline matters, and a smaller share on large ones. A machine reports
# a little under its nominal size (64 GB shows as 62.8 GiB), so the lookup
# snaps to the nearest entry; anything past either end takes the end entry.
_claude_memory_targets=(
  8:4G
  12:6G
  16:8G
  24:12G
  32:16G
  48:20G
  64:24G
  96:36G
  128:48G
  192:64G
  256:96G
)

# Prints the cap for a MemTotal value in kB (the /proc/meminfo unit).
_claude_memory_max() {
  local total_gb=$(( ($1 + 524288) / 1048576 ))
  local entry best_cap best_diff=-1
  for entry in "${_claude_memory_targets[@]}"; do
    local diff=$(( ${entry%%:*} - total_gb ))
    (( diff < 0 )) && diff=$(( -diff ))
    if (( best_diff < 0 || diff < best_diff )); then
      best_diff=$diff
      best_cap=${entry#*:}
    fi
  done
  print -r -- "$best_cap"
}

claude() {
  local bin
  bin=$(whence -p claude) || { print -u2 "claude: command not found"; return 127; }

  # Not Linux, no user systemd session (plain ssh into a node), or no meminfo:
  # run it uncapped rather than fail.
  if ! command -v systemd-run >/dev/null 2>&1 \
     || [[ ! -S "${XDG_RUNTIME_DIR:-/nonexistent}/bus" ]] \
     || [[ ! -r /proc/meminfo ]]; then
    "$bin" "$@"
    return
  fi

  local max=${CLAUDE_MEMORY_MAX:-}
  if [[ -z $max ]]; then
    local total_kb
    total_kb=$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)
    max=$(_claude_memory_max "$total_kb")
  fi

  systemd-run --user --scope -q -p MemoryMax="$max" -p MemorySwapMax=0 "$bin" "$@"
}
