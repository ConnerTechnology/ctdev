# ctdev CLI

`ctdev install <component>` installs the component (pulling in its `Dependencies`
first) and then runs its configuration step if it has one — a `configure <name>`
category (e.g. `pihole`) or a dedicated wizard (`caddy`, `mcp-email-server`; see
`componentWizards` in `cmd/install.go`). Re-running `install` on something
already installed says so and jumps straight to configuration. `ctdev configure
<name>` configures without installing. (Both skipped in `--batch`/`--dry-run`.)

```bash
ctdev apply [profile]           # Apply a machine profile; no args lists profiles
ctdev diff <profile>            # Show drift from a profile (non-zero exit on drift)
ctdev install <component...>    # Install components (then configure them)
ctdev uninstall <component...>  # Remove specific components
ctdev update [-y]               # Update system packages, components, and Docker stacks
ctdev update --check            # List available updates without installing
ctdev update --refresh-keys     # Refresh APT GPG keys before updating
ctdev info                      # Inventory: specs, kernel, uptime, profile, drives, usage, installed components
ctdev status                    # Needs attention: reboot/failed units, disk pressure/SMART, wedged apt, containers, backups, updates
ctdev configure                 # Full-screen settings browser (all categories)
ctdev configure <category>      # Configure a specific category
ctdev configure --show          # Show current system configuration
ctdev configure git             # Configure git user and SSH signing key
ctdev configure aws             # Configure AWS profile
ctdev configure ssh             # SSH server + key-based auth hardening
ctdev configure ufw             # UFW firewall (SSH/Mosh from private ranges)
ctdev configure sleep           # Never-suspend (mask sleep targets)
ctdev configure locale          # UTF-8 locale (for Mosh)
ctdev configure linger          # User-service lingering
ctdev configure tunnel          # VS Code tunnel service
ctdev configure autoupdate      # Automatic security updates + apt-daily job timeout
ctdev configure macos           # macOS defaults (Dock/Finder/keyboard) — macOS only
ctdev configure pihole          # Pi-hole DNS (upstreams, listening mode, blocking, host resolver)
ctdev configure caddy           # Caddy reverse proxy (domain, ACME email, CF token)
ctdev configure restic          # restic backups (repo, credentials, paths) — --show
ctdev configure mcp-email-server # mailboxes for the MCP email server (+ tailscale serve, attachment policy)
ctdev configure gpu             # NVIDIA driver/MOK signing + GPU settings (--show, --recover)
ctdev configure <category> --batch  # Apply a category's defaults non-interactively
ctdev pihole sync               # Apply the version-controlled lists.toml to this Pi-hole (interactive)
ctdev pihole sync --dry-run     # Show what would change, apply nothing
ctdev pihole export             # Write this Pi-hole's current lists back into lists.toml
ctdev backup now                # Run a restic snapshot of this machine now
ctdev backup test               # Check backups are set up correctly (config, connection, paths)
ctdev backup disable            # Pause scheduled backups (config + snapshots kept)
ctdev backup enable             # Resume scheduled backups
ctdev backup snapshots [primary|local]  # List this machine's restic snapshots
ctdev backup paths              # Pick what to back up in a local web UI
ctdev backup paths --listen tailnet  # serve the picker on this node's Tailscale address
ctdev restore ls|files|in-place|check   # Inspect/restore from restic (see RECOVERY.md)
ctdev cleanup                   # Reclaim disk space (scan, pick tasks, clean; --dry-run to preview)
ctdev verify                    # Verify the bootstrap installation
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
