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
(`ccstatusline skipped: nodenv failed`) rather than run against a missing dependency. They
count as skipped, not failed, and on the progress screen the `Retry:` line lists them with
the dependency (`--batch` prints no `Retry:` line).
`uninstall` runs every selected component regardless. A component named on the command line
that isn't installed is reported (`kopia isn't installed`) and skipped; `--force` runs its
uninstaller anyway, for a leftover the check misses, such as a kopia older than the pinned one.

```bash
ctdev install                   # Pick components to install from the registry
ctdev install <component...>    # Install components (then configure them)
ctdev uninstall                 # Pick components to remove from the installed ones
ctdev uninstall <component...>  # Remove specific components (skips any not installed)
ctdev update [-y]               # Update system packages, components, and Docker stacks
ctdev update --check            # List available updates without installing
ctdev update --refresh-keys     # Refresh APT GPG keys before updating
ctdev info                      # Inventory: specs, kernel, uptime, drives, usage, installed components
ctdev configure                 # Full-screen settings browser (all categories)
ctdev configure <category>      # Configure a specific category
ctdev configure --show          # Show current system configuration
ctdev configure git             # Configure git user and SSH signing key (installs git first if missing)
ctdev configure aws             # Pick the AWS profile; offers `aws configure sso` when there is none (installs the AWS CLI first if missing)
ctdev configure node install [major|version] [--global|--project]
                                # Install a Node version (22 means its latest release) and use it for every folder or
                                #   this project (the .node-version here, else one at the git top); making it global offers to copy the old
                                #   global's npm packages. --batch needs the version
ctdev configure node update [--global|--project]
                                # Update the Node in use here (a .node-version, else global) to the latest of its major,
                                #   moving global npm packages and deleting the old version unless something here uses it
ctdev configure node uninstall [version]  # Delete a Node version and its npm packages; never the global one
ctdev configure ssh             # Menu: Your keys (copy to a server, upload to GitHub, clipboard, delete;
                                #   pick several to act on each), Create a key, Server settings
ctdev configure ssh --batch     # Server settings at their defaults: SSH server, key-only login
ctdev configure power           # Power profile, display/suspend/screensaver timers (minutes or never), never suspend,
                                #   password right after the screensaver (macOS)
ctdev configure desktop         # Dock, Finder, animations, save/print dialogs (macOS); Nemo view + Hide drives (Cinnamon)
ctdev configure keyboard        # Key repeat delay (ms) and rate (cps) on Cinnamon and macOS, macOS typing, NumLock on boot
ctdev configure audio           # PipeWire sound system, Bluetooth audio (codecs, LDAC), alert sounds, volume click (macOS)
ctdev configure bluetooth       # bluez + Blueman, and bluetooth.service on at startup
ctdev configure storage         # Weekly SSD TRIM (fstrim.timer, only with an SSD), swapfile size and swappiness (Linux)
ctdev configure caddy           # Caddy reverse proxy (domain, ACME email, CF token)
ctdev configure gpu             # NVIDIA driver/MOK signing + GPU settings (--show, --recover)
ctdev configure <category> --batch  # Apply a category's defaults non-interactively
ctdev configure storage --batch --swapfile-size 2  # Same, with a 2 GiB swapfile; also resizes ctdev's own
ctdev backup                    # The folder you're in, full screen: its status and a key per action: b Back up now,
                                #   l List backups (details, z Download as zip, D Delete this backup), c Configure,
                                #   s Stop backing up, D Delete all, q Quit
ctdev backup --batch            # Status only (also when piped); exits non-zero if not backed up, or the last backup failed or is overdue
ctdev backup --all              # From any directory, full screen: l every backed-up folder in the destination, by computer,
                                #   to list and download; c Computers (remove one, I lost this computer); e Emergency Kit and password
                                #   (show the kit, change the master password, new Secret Key, replace backend credentials,
                                #   Recovery Code, paste an updated kit); connects this computer to a destination first if it has none
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
