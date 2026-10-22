# ctdev CLI

`ctdev install <component>` installs the component (pulling in its `Dependencies`
first) and then runs its configuration step if it has one — a `configure <name>`
category (e.g. `pihole`) or a dedicated wizard (`caddy`; see
`componentWizards` in `cmd/install.go`). Re-running `install` on something
already installed says so and jumps straight to configuration. `ctdev configure
<name>` configures without installing. (Both skipped in `--batch`/`--dry-run`.)
The exception is `claude-code`: its drift review of `~/.claude/` runs after every install
that includes it, in every mode, so `--dry-run` shows the diff and `--force` says where the
backup went.

If a dependency fails, its dependents in the same run are skipped with the reason
(`ccstatusline skipped: node failed`) rather than run against a missing dependency. They
count as skipped, not failed, and on the progress screen the `Retry:` line lists them with
the dependency (`--batch` prints no `Retry:` line).
`uninstall` runs every selected component regardless.

```bash
ctdev install                   # Pick components to install from the registry
ctdev install <component...>    # Install components (then configure them)
ctdev uninstall <component...>  # Remove specific components
ctdev update [-y]               # Update system packages, components, and Docker stacks
ctdev update --check            # List available updates without installing
ctdev update --refresh-keys     # Refresh APT GPG keys before updating
ctdev info                      # Inventory: specs, kernel, uptime, drives, usage, installed components
ctdev configure                 # Full-screen settings browser (all categories)
ctdev configure <category>      # Configure a specific category
ctdev configure --show          # Show current system configuration
ctdev configure git             # Configure git user and SSH signing key (installs git first if missing)
ctdev configure aws             # Pick the AWS profile; offers `aws configure sso` when there is none (installs the AWS CLI first if missing)
ctdev configure ssh             # Remote access: SSH server, key-only login, keep programs running,
                                #   Mosh, vscode.dev tunnel, LAN-only firewall (UFW)
ctdev configure power           # Power profile, display/suspend/screensaver timers (minutes or never), never suspend
ctdev configure macos           # macOS defaults (typing/dialogs/screensaver password) — macOS only
ctdev configure desktop         # Dock, Finder, animations (macOS); Nemo view + Hide drives (Cinnamon)
ctdev configure keyboard        # Key repeat delay/speed (Cinnamon, macOS) + NumLock on boot
ctdev configure audio           # PipeWire sound system, Bluetooth audio (codecs, LDAC), alert sounds
ctdev configure bluetooth       # bluez + Blueman, and bluetooth.service on at startup
ctdev configure storage         # Weekly SSD TRIM (fstrim.timer) — only on machines with an SSD
ctdev configure pihole          # Pi-hole DNS (upstreams, listening mode, blocking, host resolver)
ctdev configure caddy           # Caddy reverse proxy (domain, ACME email, CF token)
ctdev configure claude-code     # Diff ~/.claude/settings.json and CLAUDE.md against ctdev's baseline; ask before replacing (--force, --dry-run)
ctdev configure restic          # restic backups (repo, credentials, paths) — --show
ctdev configure linear          # this repo's Linear MCP server (local scope), as this computer's Linear app (--show)
ctdev configure jira            # this repo's Atlassian MCP server (local scope), and your Jira API token for scripts (--show)
ctdev configure gpu             # NVIDIA driver/MOK signing + GPU settings (--show, --recover)
ctdev configure <category> --batch  # Apply a category's defaults non-interactively
ctdev repo setup                # install git, gh, Claude Code and the connertechnology plugin for this repo, then start /connertechnology:setup
ctdev backup now                # Run a restic snapshot of this machine now
ctdev backup test               # Check backups are set up correctly (config, connection, paths)
ctdev backup disable            # Pause scheduled backups (config + snapshots kept)
ctdev backup enable             # Resume scheduled backups
ctdev backup snapshots [primary|local]  # List this machine's restic snapshots
ctdev backup paths              # Pick what to back up in a local web UI
ctdev backup paths --listen tailnet  # serve the picker on this node's Tailscale address
ctdev restore ls|files|in-place|check   # Inspect/restore from restic (see RECOVERY.md)
ctdev cleanup                   # Reclaim disk space (scan, pick tasks, clean; --dry-run to preview)
ctdev doctor                    # Diagnose any machine: network, hardware, OS, security
ctdev doctor --deep             # + vendor APIs (UniFi/Synology/Proxmox), gear fingerprint
ctdev doctor --network          # network and internet checks only
ctdev doctor --root             # prompt once for sudo so root-only checks run
ctdev doctor --report [path]    # also write a shareable Markdown report
ctdev doctor --redact           # mask SSID/MAC/public IP so the report can be shared
ctdev doctor --strict           # exit non-zero on failure (for cron)
ctdev doctor --no-integrations  # never call a vendor API, even with credentials

# Vendor deep-dive: the key alone is enough — the controller defaults to the gateway.
CTDEV_UNIFI_API_KEY=<key> ctdev doctor --deep
ctdev doctor --deep --unifi https://10.2.2.1   # when it isn't the gateway
```

**Flags that work on any command:** `--help`, `--dry-run`, `--verbose`, `--force`, `--batch`, `--version`

Run `ctdev install` with no arguments for an interactive component picker.
