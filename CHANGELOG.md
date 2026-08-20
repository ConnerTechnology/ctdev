# Changelog

All notable changes to this project will be documented in this file.

## [12.16.1] - 2026-08-20

### Fixed
- **`ctdev update` no longer offers a caddy rebuild that rebuilds nothing.** On a
  Pi it would flag `caddy → caddy-homelab:local (rebuild)`, run the rebuild, and
  flag it again on the next run. The rebuild detector watched the *index* digest
  of the Dockerfile's base images, and Docker Hub re-pushes an official image's
  index whenever any architecture is rebuilt — or when only its annotations
  change. On ctpi01 the `caddy:2-builder` index moved from `c085abfb` to
  `4bdeabce` while the linux/arm64 manifest both indexes point at stayed
  identical (`527556fa`), so the rebuild was a total cache hit that produced the
  same image ID and never recreated the container.

  Base images are now tracked by the digest of the manifest built for the host's
  platform, which moves only when the image this machine actually runs does. The
  marker file carries a `# format: 2` stamp and an older marker is re-seeded
  rather than compared, so upgrading doesn't cost one last phantom rebuild per
  built stack.

  Pulled-image stacks (pihole, beszel, portainer) still compare index digests —
  a `docker compose pull` records the new one, so an index re-push costs a single
  no-op update rather than a loop.

## [12.16.0] - 2026-08-19

### Added
- **`ctdev backup paths --listen tailnet`.** The path picker binds to loopback,
  which makes it unusable in the situation it is most needed: SSH'd into a
  headless node. The old advice was to forward the port from your laptop, but
  the port is chosen fresh on every run — so you had to read it off the screen,
  open a second terminal, and get the forward up before giving up. The picker
  can now serve itself on the node's Tailscale address instead, printing a
  MagicDNS URL you can open from any device on the tailnet.

  Loopback remains the default and the flag is opt-in: the page browses the
  whole filesystem and writes root-owned files, so it is not something to widen
  automatically. Tailnet mode binds the node's Tailscale address specifically,
  never `0.0.0.0` — it does not become reachable from the LAN — and the
  single-use launch token and session cookie are unchanged. The request-origin
  guard accepts exactly the host the page is served from, so a lookalike name
  like `<node>.ts.net.example.com` is still refused.

### Changed
- **The loopback picker now says so when you are on a remote session.** It
  detects SSH, warns that the URL it just printed will not open from where you
  are, points at `--listen tailnet`, and prints the forward command using the
  node's MagicDNS name with a note that the port changes each run.

## [12.15.0] - 2026-08-19

### Added
- **`mcp-email-server` component — your mail, on the node, not on your laptops.**
  An MCP server that reads (and optionally sends) mail over IMAP, run as a Docker
  compose stack on an always-on box. Every laptop reaches it over Tailscale, so no
  email credential is ever stored on a laptop — that property is the whole point
  of the component.

  Upstream ships no authentication: `mcp-email-server streamable-http` accepts only
  `--host` and `--port`, so anything that reaches the port reads every configured
  mailbox in full. The container port is therefore published on `127.0.0.1` only
  (a bare mapping would be LAN-wide even behind UFW, because Docker's iptables
  rules are inserted ahead of it), and `tailscale serve` on the host is the only
  route in — TLS from the tailnet's own certificate, reachable only by
  authenticated tailnet peers. The image tag is pinned rather than tracking
  `latest`; a silent upgrade of the process holding mail credentials isn't
  something to discover afterwards.

- **`ctdev configure mcp-email-server`** — adds and removes mailboxes over SSH
  (upstream's `ui` subcommand needs a browser), then publishes the service to the
  tailnet. Presets for iCloud, Gmail/Workspace, Microsoft 365, and Fastmail;
  sending is opt-in per account, so a mailbox an agent can read isn't
  automatically one it can send from. `--show` reports the accounts, the
  credential file's actual mode, and the tailnet endpoint.

  The stack is built from a Dockerfile pinning `mcp-email-server==1.4.1` on
  `python:3.12-slim` rather than pulling upstream's image, which stops at 0.16.0
  — a major version behind the Python package. That also buys the headless setup
  path: `account add --password-stdin` (never argv), `--json` on every command,
  and an `account test` IMAP login check run right after you enter a password, so
  a typo fails on the spot instead of as an empty inbox days later. Removing or
  replacing an account passes the `--expected-revision` and `--confirm` flags
  upstream requires; without them removal failed silently, so re-running
  configure to fix a typo could not replace the old record.

  Mailbox passwords sit in the managed catalog at
  `~/mcp-email-server/config/managed.sqlite3`, mode 0600, owned by the invoking
  user — the container runs as that uid, not root. They are not encrypted at
  rest; the mode and the node's isolation are the protection. Install and
  configure both print the path and the mode they actually found rather than
  asserting it. `ctdev uninstall` stops the stack and keeps the catalog: losing it
  means re-issuing an app-specific password for every mailbox.

  On a node already running Caddy it publishes on tailnet port 8443 rather than
  443, because `ctdev configure caddy` points `*.<domain>` at that node's
  Tailscale IP — a serve rule on 443 would intercept it and quietly send every
  `https://<svc>.<domain>` to the email server instead of Caddy. `--serve-port`
  overrides it.

- **Install refuses to replace an email server it did not create.** The first
  version of this component did exactly that on `ctpi01`: a different
  implementation was serving the same role, `ctdev install` swapped it out, and
  the replacement answered `list_available_accounts` with an empty list and
  `isError: false`. To a client that reads as "no mailboxes configured", not
  "your accounts are gone" — an assistant triaging mail against it would have
  reported a clean inbox. Install now probes for a container it didn't build, a
  `./config` belonging to another server, and a `tailscale serve` mapping already
  bound to the port; on any of those it names what it found, says the accounts
  will not carry over, and stops unless `--force` is given. `--dry-run` previews
  the refusal. Re-running install over its own stack stays idempotent and never
  touches `./config`.

- **`ai-node` profile** — the box that runs the AI services: zsh, git, tailscale,
  docker, portainer, beszel, restic, and mcp-email-server. Built for a Raspberry
  Pi today and a Proxmox VM or LXC container later, with no change to the profile.

## [12.14.1] - 2026-08-14

### Fixed
- **`CTDEV_UNIFI_API_KEY` alone did nothing.** The set-up instructions said the
  key was enough, but the controller endpoint had to be supplied separately —
  so `doctor` would identify a UniFi gateway, print instructions promising the
  key would work, and then refuse to talk to it. The controller now defaults to
  the detected gateway; `--unifi <url>` still overrides it for the case where
  the controller isn't the gateway.
- **Set-up instructions printed even after the set-up had worked.** With
  credentials in hand, the network-equipment row sat above five rows of live
  controller data telling you to go and create an API key. It now points at the
  controller data instead.

## [12.14.0] - 2026-08-13

### Added
- **`ctdev doctor` — a health report for machines ctdev doesn't manage.** The
  rest of ctdev assumes it set the machine up; this assumes nothing. It's for
  the laptop someone hands you saying "my Wi-Fi doesn't work". Every check is
  read-only, root is never required (checks that need it report Skipped and say
  why), and no data leaves the machine beyond the diagnostic probes themselves.

  Roughly 35 checks across network, internet, hardware, system, and security:
  Wi-Fi signal in dBm with band and channel, gateway reachability, per-resolver
  DNS timing, captive-portal detection, TLS failures reported separately from
  connection failures, IPv6-present-but-unreachable, clock skew, disk and inode
  pressure, SMART health, thermal throttling, OOM kills, degraded RAID, stuck
  print queues, container restart loops, firewall and disk-encryption posture.

- **Verdicts, not just checks.** A column of red marks tells you twelve things
  are wrong; `doctor` tells you the one thing that is wrong and that the other
  eleven are downstream of it — "the router is fine, the connection to the
  internet is down", "the clock is wrong, which breaks every secure website",
  "the router isn't handing out addresses". Network rules are layered so one
  root cause produces one verdict.

- **Vendor deep-dives under `--deep`.** A laptop can see its own signal is poor;
  only the controller knows the access point it's on is relaying its uplink
  wirelessly over a channel it was thrown off by radar an hour ago. Reads UniFi
  (radar events, airtime, mesh uplinks, min-RSSI thresholds), Synology (disk and
  volume health), Proxmox (node and storage health), plus credential-free
  Pi-hole, Docker, and Tailscale checks. Read-only by construction — the
  provider interface has no method that changes anything.

  Without credentials it still fingerprints the gear from the gateway MAC, a
  Ubiquiti discovery probe, and the management certificate, then prints exact
  instructions for granting read-only access.

- **Run it without installing anything.** `install.sh --doctor` downloads ctdev
  to a temp directory, runs the report, and deletes it — no install directory
  touched, no PATH changed, no sudo. `install.ps1 -Doctor` on Windows.

- **`--report` writes shareable Markdown**, and `--redact` masks the SSID, MAC
  addresses, and this connection's public IP first. Public resolvers survive
  redaction, because "1.1.1.1 answers and yours doesn't" is the diagnosis.

- **Windows support, scoped to `doctor`.** Every other command refuses up front
  rather than failing halfway through. Release builds now cover
  `windows/amd64` and `windows/arm64`.

### Changed
- **`ctdev status` now shares the `doctor` check catalog**, filtered to the
  checks that touch nothing but this machine — so there's one implementation of
  "is the disk full" instead of two. Its no-network contract is unchanged.

### Fixed
- **`install.sh` documentation corrected**: it downloads a release binary and
  verifies it against `SHA256SUMS`. It has never had a source-build path,
  despite CLAUDE.md claiming otherwise.

## [12.13.0] - 2026-08-13

### Fixed
- **`ctdev update` and `ctdev install tailscale` hung on macOS with no visible
  prompt.** `ensureSudo()` — the one place ctdev pre-caches a sudo credential,
  and the reason a password prompt can't land inside a Bubble Tea program —
  began with `if runtime.GOOS != "linux" { return nil }`, so on a Mac all 17 of
  its call sites did nothing. Homebrew escalates on its own for casks that ship
  a pkg payload or a system extension (tailscale, docker, 1password), and with
  no cached credential its `sudo` prompted on `/dev/tty`: invisible, because
  ctdev captures step output through a pipe, and unanswerable, because the TUI
  held the terminal in raw mode. Ctrl-C was the only way out.
- **The update summary was truncated.** The progress TUI renders inline, and its
  final frame listed every step plus up to 15 replayed lines per failure.
  Bubble Tea's inline renderer drops lines from the *top* of a frame taller than
  the terminal, so a run with a few dozen brew packages silently lost its
  headline and first results. The final frame is now short — headline, tally,
  failed names — and the full per-item report prints as ordinary stdout text
  afterwards, where the terminal scrolls it and scrollback keeps it.
- **Homebrew versions were parsed wrong.** `brew outdated --verbose` prints
  `openssl@3 (3.3.2, 3.4.0) < 3.4.1` when two kegs are installed, and the old
  whitespace parser read the literal `<` as the new version. Scanning now uses
  `brew outdated --json=v2`, which also fixes multi-word current versions
  (`latest HEAD`) and `[pinned at X]` suffixes. Pinned formulae are skipped —
  `brew upgrade` errors on them, and one pinned formula failed the whole batch.
- **The runtime updaters fought Homebrew.** On a Mac with brew-installed Go, the
  scanner offered Go as a "runtime" update whose apply step ran
  `sudo rm -rf /usr/local/go` over the top of the Homebrew install — while the
  same Go also appeared under `brew`. `scanGo` and `scanRuby` now defer to the
  brew scanner, matching what the installers already did.
- **The Docker stack scan wasn't OS-gated.** Stacks were selected by a bare
  `os.Stat` on their compose file, so a Mac holding a restored `~/pihole/` was
  scanned for Linux-only stacks. The docker probes were also unbounded, so a
  stopped Docker Desktop stalled the scan behind its spinner; each now has a
  20-second timeout.
- **A cask install could report success without installing anything.** Homebrew
  marks a cask installed as soon as its Caskroom entry exists, independently of
  the artifact being put in place — so when the sudo prompt above interrupted
  the `.pkg` step for `tailscale`, brew went on believing the job was done. Every
  later `brew install --cask` then no-opped and exited 0, and ctdev faithfully
  reported a successful install of software that was never on the machine.
  Installs of the casks that escalate (tailscale, docker, 1password, cleanmymac,
  logi-options) now verify the component's own detection predicate afterwards
  and repair with `brew reinstall` when the files aren't there, failing loudly if
  they still aren't.
- macOS asked for a password it did not need. Root requirements were not
  package-manager aware, so the "a package install needs root" assumption from
  apt applied to Homebrew too, which installs into a user-owned prefix. Only
  components whose Homebrew payload actually escalates now pre-authorize.

### Added
- The sudo credential is refreshed in the background for the length of a run.
  macOS expires it after 5 minutes by default, which a large `brew upgrade`
  easily outlasts — and an expiry mid-run brought the hang straight back.
- `SUDO_ASKPASS` points at a helper that explains itself and supplies nothing,
  so a tool that escalates unexpectedly fails with an actionable message instead
  of blocking forever on a prompt nobody can see.
- The update checklist grows with the terminal instead of stopping at 15 rows,
  and takes PgUp/PgDn (and ctrl+u/ctrl+d).
- `brew update` runs before scanning, mirroring the existing APT index refresh,
  so suppressing Homebrew's mid-step auto-update can't leave results stale.
- CI runs `go test` and `go vet` on macOS. They only ran on Ubuntu, which is why
  all of the above shipped unnoticed.


## [12.12.1] - 2026-07-30

### Fixed
- **A devcontainer kept opening `bash` after `ctdev install zsh` said it
  succeeded.** The install ran `chsh` unprivileged, and unprivileged chsh
  authenticates the caller through PAM — so on the passwordless user a
  devcontainer runs as, it failed with `chsh: PAM: Authentication failure`. The
  login shell stayed `/bin/bash`, the freshly deployed `.zshrc` was never
  sourced, and the Pure prompt never appeared. Only a warning was printed, so
  the run still reported success. `chsh` now runs as root against the current
  user, and is skipped entirely when zsh is already the login shell.
- The shell is registered in `/etc/shells` before `chsh` runs. chsh rejects a
  shell that isn't listed there, which is where a zsh from Homebrew or a source
  build lands.
- When root is genuinely out of reach, the warning now names the command to
  finish the job by hand (`sudo chsh -s /usr/bin/zsh <user>`) instead of
  reporting a bare `exit status 1`.

## [12.12.0] - 2026-07-30

### Fixed
- **`ctdev install` aborted inside a devcontainer before installing anything.**
  Every install began by caching sudo credentials, so a container that runs as
  root with no sudo in the image — or one started with `no-new-privileges` —
  failed that probe and the run stopped at `sudo required for install`. It did
  this even for `zsh`, `claude-code`, and `git`, which only write files under
  `$HOME`. Sudo is now asked for only when the selected components will actually
  use it: each registry entry declares its need (`Root`), the default being that
  root is required to put the software in place but not to re-sync configs over
  an install that's already there. A shell-config-only install never touches
  sudo. When root is genuinely needed but out of reach, ctdev warns and
  continues rather than refusing to start — the step that needs it fails with
  its own error and everything else still lands.
- **Privileged commands no longer route through `sudo` when ctdev is already
  root.** `sysutil.SudoRun` — every package install, `/usr/local` write, and
  systemd action — prefixed `sudo` unconditionally, which fails in the one
  environment that needs it least. A command that does need root with no sudo
  available now reports `apt-get needs root, but there is no sudo to run it
  with` instead of `exec: "sudo": executable file not found in $PATH`.
- The prompt-free root probes behind `ctdev status` (SMART health) and the
  directory sizing in `ctdev cleanup` / `ctdev backup paths` also assumed
  `sudo -n`, so they reported nothing at all as root. They now run directly.

## [12.11.0] - 2026-07-29

### Added
- **`ctdev info` lists the machine's physical drives** in its Hardware section:
  model, capacity, and what each one carries. This is deliberately not the same
  view as `Usage` — that one lists mounted filesystems, so a drive holding a
  second OS is mounted nowhere and doesn't appear at all, despite being very
  much installed. A drive nothing mounts is reported by the filesystems found
  on it instead (`not mounted (ntfs, vfat)`). The reader walks the whole
  `lsblk` tree, so a root under LUKS→LVM is still attributed to the disk that
  holds it, and container types (`crypto_LUKS`, `LVM2_member`) are filtered out
  since they name the wrapper rather than its content.

### Fixed
- **The `go` component reported itself not installed when Go came from the
  distro.** Its registry entry pinned `DetectPath` to `/usr/local/go/bin/go`,
  and `IsInstalled` treats `DetectPath` as exclusive — filesystem check, no
  PATH fallback. That contradicted `goInstall`, which has always treated any
  `go` on PATH as installed, so `ctdev install go` would say "Go already
  installed" while `ctdev info` listed it missing and every profile carrying
  `go` showed permanent drift. Detection now falls through to a PATH lookup,
  matching the installer.
- `ctdev uninstall go` only ever removed the official tarball at
  `/usr/local/go`. When a distro-packaged Go is still on PATH afterwards it now
  says so, instead of reporting a removal that didn't happen.

## [12.10.1] - 2026-07-29

### Fixed
- **`ctdev info` was only ever showing two filesystems.** The disk scan
  hardcoded a filter to `/` and `/home`, so every other mounted volume was
  silently dropped — on a machine with a second drive, entire disks were
  missing from the inventory. It now lists every real filesystem (skipping
  tmpfs and friends, and anything under 1 GiB so the EFI partition doesn't earn
  a row), sorted root-first then alphabetically so the list is stable run to
  run. `/boot` is deliberately kept: it fills with old kernels and that's worth
  seeing.
- Sizes below a gibibyte now read in megabytes. A 372M `/boot` was rendering as
  `0.4G`, losing precision exactly where it mattered. This also sharpens the
  free-space figure in `ctdev status`'s disk-pressure warning.

### Changed
- The `Usage` section groups the filesystems under a **`Disk`** heading, labels
  root as `/ (system)` — a bare `/` doesn't say it's a filesystem, let alone
  which one — and spaces the bars out so they're readable at a glance rather
  than stacked flush. Disk sizes now go through the same formatter as memory,
  so both columns read in identical units.

## [12.10.0] - 2026-07-29

### Changed
- **`ctdev status` and `ctdev info` no longer overlap.** `status` is now only
  things that may need attention; `info` is the inventory. Uptime, load, memory
  and disk usage are gone from `status` — disk was a straight duplicate of
  `info`, and the rest were facts about the machine rather than problems with
  it. `info` picks up **Uptime** in its System section, and its `Disk` section
  becomes **`Usage`**, carrying memory alongside the disks with the same bar.
  Hardware still lists installed capacity (64 GB); Usage shows what's consumed
  of what's usable (17G / 62G) — the gap is firmware reservation, not a bug.

### Added
- **`ctdev status` flags a wedged apt.** The daily apt jobs hold
  `/var/lib/apt/lists/lock` while they run, so one that stalls blocks every
  later apt call until reboot — and `status` would cheerfully report "apt up to
  date" against an index that hadn't refreshed in days. It now warns when a
  daily job has been mid-run for over 30 minutes, or the package index is more
  than 48 hours old, and points at the fix. Pairs with the `apt daily job
  timeout` setting added in 12.9.0, which stops the wedge happening at all.
- **`ctdev status` warns on disk pressure** — any filesystem at 85% or above,
  with free space and a pointer to `ctdev cleanup`. Below the threshold it says
  nothing, so the always-on usage view stays in `info` alone.
- **`ctdev info` reports the machine's profile** — the one `ctdev apply`
  recorded (now saved to `~/.local/state/ctdev/applied-profile`), with how many
  of its components are present and a pointer to `ctdev diff` when any are
  missing. On a machine composed by hand it infers the closest match instead,
  labeled `(closest match)` so it's never mistaken for a recorded fact, and
  stays silent below 50% overlap rather than claiming a bad match.
- **`ctdev info` shows the kernel release and distro version** —
  `linuxmint 22.3 (noble)` and `7.0.0-28-generic` instead of a bare
  `linuxmint`. Both are the first thing you want when chasing a driver,
  suspend, or repo problem.

## [12.9.0] - 2026-07-29

### Added
- **`ctdev configure autoupdate` can now cap how long apt's daily jobs run.**
  `apt-daily.service` and `apt-daily-upgrade.service` ship with the `apt`
  package as `Type=oneshot`, and systemd gives that type a start timeout of
  `infinity` — so a mirror fetch that stalls instead of failing (a suspend or
  network switch black-holing the connection mid-download, say) holds
  `/var/lib/apt/lists/lock` until reboot. Every later apt run then fails to lock,
  and `ctdev update` silently scans a stale index. The new "apt daily job
  timeout" setting writes a `TimeoutStartSec=` drop-in to both units under
  `/etc/systemd/system/` (not `/usr/lib`, which apt overwrites on upgrade) and
  reloads systemd. Choices are 15min / **30min** (recommended) / 1h, plus
  "No timeout", which removes the drop-in and restores stock apt rather than
  writing a file that merely restates the default. Shown by `configure
  --show`, applied by `--batch`, and included in the `autoupdate` category that
  the built-in profiles already configure.

### Fixed
- The release runbook in `CLAUDE.md` pointed at `.github/workflows/release.yml`,
  which doesn't exist — the tag-triggered release job lives in `ci.yml`.

## [12.8.0] - 2026-07-29

### Changed
- **The `ctdev update`, `ctdev install`/`uninstall`, and `ctdev cleanup` pickers
  render inline instead of taking over the terminal.** They opened on the
  alternate screen, so the list covered whatever you had on screen and then
  vanished on exit, and `ctdev update` visibly flipped in and out of full screen
  between picking updates and applying them (the progress view was already
  inline). The list now scrolls inside a 15-row window in the normal terminal
  flow, and collapses to a one-line summary (`✓ Available Updates · 12
  selected`) once you confirm or cancel, so a run reads as ordinary command
  output and stays in the scrollback. The full-screen `ctdev configure` settings
  browser is unchanged.

### Removed
- **Components `chatgpt`, `codex`, and `ghostty`** — the registry is now 50.
  `ghostty` also drops its embedded `~/.config/ghostty/config` and, on apt, the
  pinned checksum-verified third-party installer it used (Ghostty has no official
  Ubuntu package). Run `ctdev uninstall <name>` on any machine that still has one
  **before** upgrading — ctdev can no longer remove them afterwards. Codex's npm
  package is unaffected by the uninstall path either way: remove it with
  `npm uninstall -g @openai/codex`.
- **The modern-CLI toolset from the `dev-workstation` profile** — `ripgrep`,
  `fd`, `fzf`, `bat`, `zoxide`, `direnv`, `lazygit`, and `ghostty`. The first
  seven remain in the registry and installable on demand (`ctdev install bat`);
  they are simply no longer part of the profile, so `ctdev diff dev-workstation`
  no longer reports a machine without them as drifted.
- **The MT7925E WiFi suspend-fix setting** from `ctdev configure` — the systemd
  sleep hook that performed a PCIe-level reset of the adapter around suspend,
  along with its hardware detection and reset path. Machines that already
  installed it keep the hook; remove it by hand with
  `sudo rm /usr/lib/systemd/system-sleep/wifi-mt7925`.

## [12.7.3] - 2026-07-08

### Fixed
- **`ctdev update` no longer re-offers node/ruby updates you've already applied.**
  The scanners read the version through `nodenv version` / the rbenv shim, which
  resolve the current directory's `.node-version` / `.ruby-version` — so running
  `ctdev update` inside a project pinned to an older version offered the same
  "update" forever (and applying it was a silent no-op, since the update step
  sets the *global* version). The scanners now compare against `nodenv global` /
  `rbenv global`, matching what the update actually changes.
- **`ctdev update` can no longer miss a Go update when run inside a module with a
  newer `toolchain` directive.** `go version` auto-switches to (and reports) the
  downloaded toolchain in that case; the scanner now runs it with
  `GOTOOLCHAIN=local` to see the real installed version.

## [12.7.2] - 2026-07-06

### Fixed
- **smartmontools no longer leaves a failed unit on SD-card/eMMC machines.**
  `ctdev install smartmontools` enabled smartd unconditionally, but smartd exits
  ("No devices to monitor", status 17) and lands in `systemctl --failed` when
  there is no SMART-capable disk — the norm on a Raspberry Pi booting from SD.
  The daemon is now enabled only when a `smartctl --scan` finds a device;
  otherwise the tool is installed but smartd is left off, with a clear note.

## [12.7.1] - 2026-07-06

### Changed
- `ctdev status` prints each system vital (uptime, load, memory, disk) on its
  own labeled line instead of one dense `·`-joined row, matching the rest of
  the layout.

## [12.7.0] - 2026-07-06

### Added
- **`ctdev status` reports more machine health**, all read locally so it stays
  fast and prompt-free:
  - **Memory** on the System line, and load now shows the core count for context.
  - **Reboot required** — flags a pending kernel/security reboot
    (`/var/run/reboot-required`), the natural companion to `configure autoupdate`.
  - **Failed systemd units** — a count when anything is in the failed state, the
    catch-all for a quietly broken service.
  - **Disk SMART health** — PASSED/FAILING per disk via smartctl when
    smartmontools is installed. Uses `sudo -n` only, so it reports when sudo is
    already cached and silently skips otherwise — status never prompts.
  - **CPU temperature** where the kernel exposes a thermal zone (Raspberry Pi,
    laptops), with a "hot" warning past 80°C.

### Fixed
- `ctdev status` no longer shows a confusing `integrity: timer not-found` line:
  the integrity sub-line is suppressed when the backup timer is disabled, and
  says "not deployed" (with the fix) rather than "timer not-found" when the
  monthly-check timer predates its introduction.

## [12.6.1] - 2026-07-06

### Fixed
- **No more `Failed to disable unit: restic-check.timer does not exist` noise.**
  On a node without the monthly check timer (any install predating v12.3.1),
  `ctdev backup disable`/`enable` and `ctdev uninstall restic` leaked systemctl's
  error to the screen — the failure was ignored internally, but SudoRun routes
  stderr to stdout. These now check whether the check-timer unit is deployed
  before touching it.

## [12.6.0] - 2026-07-06

### Added
- **`ctdev backup disable` / `ctdev backup enable`** — a first-class off switch
  for scheduled backups. `disable` stops and disables the nightly timer and the
  monthly integrity check (config and snapshots are kept; one-off `ctdev backup
  now` still works); `enable` turns them back on (requires restic installed and
  configured). Previously the only ways to stop scheduled backups were dropping
  to `systemctl` or uninstalling restic.

## [12.5.0] - 2026-07-06

### Added
- **`ctdev backup test`** — a preflight that checks the whole backup chain is
  set up correctly: restic installed, the repository configured and *reachable*
  (credentials, network, and password verified against the live repo with a
  read-only `restic cat config`), the selected paths exist on disk, and the
  nightly timer is enabled. Nothing is written; exits non-zero on any problem,
  so it doubles as a health check.
- **Configurable restic retention.** The keep policy (previously hardcoded
  7 daily / 4 weekly / 6 monthly) is now set in `ctdev configure restic` — it
  prompts for daily/weekly/monthly counts, defaulting to the current or
  recommended values. Set weekly and monthly to 0 to keep only the last N daily
  backups. Stored as `RESTIC_KEEP_*` in `/etc/restic/restic.env` and honored by
  the backup script's prune step; `configure restic --show` displays the policy.

### Changed
- **`ctdev configure restic` no longer makes you re-pick the backend when
  reconfiguring.** With a repository already set, it shows the current one and
  offers to keep it (Enter) — carrying its credentials forward — so changing
  retention or adding a healthcheck no longer means re-entering the bucket and
  keys. Secret prompts continue to show `[keep existing]`.

## [12.4.2] - 2026-07-06

### Added
- **Configurable restic retention.** The backup script's keep policy (previously
  hardcoded 7 daily / 4 weekly / 6 monthly) now reads `RESTIC_KEEP_DAILY`,
  `RESTIC_KEEP_WEEKLY`, and `RESTIC_KEEP_MONTHLY` from `/etc/restic/restic.env`,
  defaulting to the old values. (v12.5.0 adds interactive prompts for these in
  `ctdev configure restic`.)

## [12.4.1] - 2026-07-06

### Fixed
- **`ctdev update` no longer errors on locally-built docker stacks.** The caddy
  stack builds `caddy-homelab:local`, which lives in no registry — but modern
  BuildKit stamps local images with a RepoDigest, so the update scanner couldn't
  tell it from a registry-pulled image and ran `imagetools inspect` on it,
  failing with "pull access denied". Built stacks (those with a Dockerfile) are
  now tracked by their Dockerfile base images, the RepoDigest path is used only
  for genuinely pulled stacks.
- **`ctdev status`/`verify`/`backup` now honestly report an unconfigured restic.**
  The configured-check only tested that `/etc/restic/restic.env` existed, so a
  config written before v12.0.0 renamed `RESTIC_REPO_B2` → `RESTIC_REPOSITORY`
  was green-lit while the backup script failed on it every night. It now requires
  `RESTIC_REPOSITORY` to actually be set. (Nodes configured pre-v12.0.0 need one
  `ctdev configure restic` to write the new schema.)
- **CI: release builds no longer race the branch push.** A release pushes main
  and a tag at the same commit; the SHA-keyed concurrency group made the two
  runs cancel each other, and when the tag run lost, the release never built
  (v12.4.0). The group is now keyed on the ref, so tag and branch runs are
  independent.

## [12.4.0] - 2026-07-06

### Added
- **Full-screen settings browser.** A bare `ctdev configure` now opens every
  applicable setting in one grouped, filterable screen: current value per row,
  an `≠ REC` badge when it differs from the recommended value, and the
  setting's description plus current/recommended (and slider range) in a detail
  pane. Editing is modeless — Enter cycles pickers/toggles, ←/→ steps sliders,
  `r` jumps to recommended, `u` reverts, `/` filters, `a` applies (with the
  usual change summary and confirm), `q` discards. One-way install-style
  settings queue their single honest action and show ✓ once applied. The line
  wizard remains for per-category runs (`ctdev configure ssh`), `--show`,
  `--batch`, and ACCESSIBLE/non-TTY sessions.
- **`ctdev configure macos`** — the macOS defaults set (Dock auto-hide/no
  recents, Finder path/status bars + list view, no .DS_Store on network/USB
  drives, smart quotes/dashes/autocorrect off, fast key repeat, expanded
  save/print dialogs, immediate password after screensaver) is now a real
  category on Macs. It previously existed as unreachable code.

### Fixed
- **Settings are gated to where they actually work.** Every setting now
  declares an OS/desktop gate (test-enforced): GRUB settings appear only where
  `/etc/default/grub` exists (no more boot-menu offers on Pi firmware boot),
  Cinnamon dconf settings only under Cinnamon (Ubuntu GNOME previously got
  silent no-op writes reported as success), desktop packages only in graphical
  sessions, WiFi power-save only with NetworkManager, the power profile only
  where powerprofilesctl exists, and Linux-only categories (ssh/ufw/locale/
  sleep/linger/tunnel/autoupdate) hide on macOS. A headless Pi's `ctdev
  configure` now shows only what applies to it.

## [12.3.1] - 2026-07-05

### Fixed
- **A backup killed mid-run (shutdown, suspend) no longer wedges every following
  night's prune.** The killed restic process leaves a stale repo lock; backups
  still succeed (shared lock) but `forget --prune` needs an exclusive lock and
  fails forever after. `restic-backup.sh` now runs `restic unlock` (which only
  removes locks whose owning process is gone) before each repo's run. Found in
  the wild by `ctdev status` on the first day it existed.
- **The restic systemd units now have a cache.** They run without `$HOME`, so
  restic was re-downloading repository metadata from the backend (B2 API calls +
  bandwidth) on every nightly run, logging "unable to locate cache directory".
  Both units use systemd's `CacheDirectory=restic` and the scripts point
  `RESTIC_CACHE_DIR` at it. Re-run `ctdev install restic` on existing nodes to
  deploy the fixed script and units.

## [12.3.0] - 2026-07-05

The fleet release: machine profiles, a health dashboard, and the additions a
family fleet actually needs (auto security updates, family DNS filtering,
backup alerting and verification, disk-health monitoring).

### Added
- **Machine profiles** — `ctdev apply <profile>` realizes a declarative machine
  role: shows the plan against the actual machine (installed ✓ / to-install →,
  dependencies resolved), confirms, installs, batch-configures categories at
  recommended values, then prints the profile's own next-steps notes. `ctdev
  diff <profile>` reports drift (missing components, settings off recommended)
  with a non-zero exit, so it works as a cron check. Three built-ins ship
  **embedded in the binary** — `pihole-node`, `dev-workstation`,
  `family-desktop` — so a freshly flashed machine can `ctdev apply pihole-node`
  with nothing but the installed binary; local files in
  `~/.config/ctdev/profiles/<name>.toml` add or override by name. Interactive
  wizards (restic/caddy/pihole/git) are never run by apply — each profile's
  notes list them — and the `gpu` category is rejected in profiles because MOK
  signing is interactive.
- **`ctdev status`** — one screen of machine health, read entirely from local
  sources (no network): uptime/load/disk, Tailscale connectivity, homelab
  containers, backup freshness and last result straight from systemd (a daily
  backup >26h old shows *overdue*, a failed run shows *FAILED* with the
  journalctl pointer), the monthly integrity-check age, and pending apt
  updates. Fast enough to run on every login.
- **10 new components** (43 → 53): `mosh` (long configured-for, finally
  installable), `ripgrep`, `fd`, `fzf`, `bat`, `zoxide`, `direnv`, `lazygit`,
  `smartmontools`, `syncthing`. Debian's renamed binaries get `~/.local/bin`
  symlinks (`fd`→fdfind, `bat`→batcat); lazygit installs the checksum-verified
  GitHub binary on Linux (brew on macOS); smartmontools enables smartd so disk
  pre-failure warnings hit the journal; syncthing prints its per-user start
  command instead of auto-starting.
- **`ctdev configure autoupdate`** — installs unattended-upgrades and enables
  the daily security-only run (normal updates stay manual via `ctdev update`).
  Recommended for always-on nodes and family machines.
- **Pi-hole "Cloudflare for Families" upstream** (1.1.1.3) — network-wide
  malware + adult-content filtering as a one-choice preset in
  `ctdev configure pihole`.
- **Backup alerting and verification**: `ctdev configure restic` offers an
  optional healthcheck ping URL (success pings it, failure pings `/fail` —
  healthchecks.io style) so a silently-broken backup gets noticed, and
  `ctdev install restic` deploys a **monthly `restic check` timer**. Re-run
  both on existing nodes to pick these up.

## [12.2.0] - 2026-07-05

Fixes from a second full-tool review (every command walked end-to-end), plus
internal restructuring (update.go split into scan/versions/apply files, a shared
composeStack helper for the docker stacks, ctx threaded through settings
detection so a wedged docker daemon can't hang `configure pihole`).

### Fixed
- **`ctdev restore in-place` never worked through ctdev** — the restore script's
  "Type YES" prompt read EOF (children run without stdin) and aborted every time.
  The confirmation now happens in ctdev itself (Ctrl-C-safe) and the script takes
  `--yes` from callers that already confirmed. The documented disaster-recovery
  path now functions.
- **`configure caddy` now wires a containerized Pi-hole.** It checked for a host
  `pihole` binary, which container installs don't have — so freeing port 443 and
  writing the wildcard-DNS record silently never happened on new nodes.
- **`ctdev verify` is container-aware**: it no longer demands a pihole binary and
  pihole-FTL unit on container installs (every Pi-hole node false-failed), and it
  now checks what matters on a homelab node — pihole/caddy/portainer/beszel
  containers running and restic-backup.timer enabled.
- **`--dry-run` violations**: `restore files|in-place --dry-run` really restored;
  `configure caddy --dry-run` still wrote `~/caddy/.env`; interactive
  `configure git --dry-run` really generated SSH keys and uploaded them to GitHub.
  All now preview only.
- `ctdev info` on a Raspberry Pi reported CPU "unknown" — arm64 kernels have no
  per-core "model name"; the board Model line (and /proc/device-tree/model) are
  now used ("Raspberry Pi 5 Model B Rev 1.1").
- Errors print exactly once, without a usage dump after runtime failures.
- git/tmux/zsh/btop/jq/shellcheck now skip cleanly on dnf/pacman systems instead
  of reporting as failed (unsupported-package-manager errors map to Skipped).
- Restore/backup repo args (`primary|local`) validate in ctdev with a clean
  message; `ctdev restore <unknown-verb>` exits non-zero; a dry-run backup no
  longer prompts for a sudo password.
- The helm update scanner no longer emits two identically-named rows (same-major
  first, then the major bump on a later scan); from-source "dev" builds no longer
  self-flag a ctdev update on every run.
- `configure git --signing-key` alone works non-interactively;
  `configure aws --show` shows the current profile instead of running the picker;
  the `backup paths` SSH port-forward hint is a valid `-L` spec; stale help text
  (restic "seeds the backup path list", verify "bootstrap") updated.

### Changed
- **The configure wizard is honest about one-way settings.** 19 install/enable
  actions (SSH server, UFW, audio stack, tunnel, TRIM, …) used to present a fake
  "1) installed 2) not installed" choice where picking "off" still installed;
  they now ask a single "Apply now (→ active)? [y/N]" or say "Already active —
  nothing to do."
- Wizard labels say **recommended** instead of "default" — matching what
  `--batch` actually applies — and the batch header says so explicitly.
- The full `ctdev configure` sweep **no longer springs the Secure Boot/MOK
  driver-signing flow**; it points to `ctdev configure gpu` instead (as the docs
  always claimed). `--show` still includes GPU status.

## [12.1.0] - 2026-07-05

Cleans up the remaining low-priority items from the v12 security and code review.

### Security
- **Third-party installers are pinned and verified.** The Ghostty Ubuntu installer
  (a community repo) is pinned by commit — tags can be force-pushed — and its SHA256
  is checked before the script runs; the NoMachine `.deb` (no vendor checksums, dpkg
  verifies no signatures) is verified against a recorded SHA256 before installing as
  root. Both hashes live next to the version pins and are bumped together.
- **`configure caddy` reads `CF_API_TOKEN` from the environment**, so the Cloudflare
  token never has to appear on a command line (shell history, `/proc/<pid>/cmdline`).
  The `--cf-token` flag still works but its help text and the README now steer to the
  env var or the masked wizard prompt.
- **Downloads refuse redirects that downgrade to plain http** (10-redirect cap), and
  `install.sh` downloads into the install directory so the final move is an atomic
  rename — a crash mid-install can't leave a truncated binary. Its old-install cleanup
  uses `sudo -n`, so a piped `curl | bash` never sits on a password prompt.
- **Secret file permissions tightened**: `~/beszel/.env` (hand-pasted KEY/TOKEN) is
  chmod'd 0600 on install, and the backup-paths/excludes files written by the web
  picker are 0600 to match everything else under `/etc/restic`.
- **`restic-backup.service` is sandboxed** with `NoNewPrivileges`, `PrivateTmp`,
  `ProtectKernelTunables`, and `ProtectControlGroups` — re-run `ctdev install restic`
  on existing nodes to redeploy the unit.

### Fixed
- The restic backup script's line trimming (`echo | xargs`) errored on paths
  containing quotes (e.g. `O'Brien/`) and silently dropped that backup path; it now
  trims in pure bash.
- `ctdev install docker` / `tailscale` no longer report success when the service
  fails to enable or start — a docker that won't run made every compose component
  (pihole, caddy, beszel, portainer) fail later with a much more confusing error.
- Shell-out failures name the command (`docker: exit status 1`, with the real
  command named through sudo) instead of a bare exit status.
- The MOK-enrollment `mokutil --import` now honors Ctrl-C (runs under the command
  context like the rest of the GPU flow).

## [12.0.0] - 2026-07-04

### Changed (breaking)
- **`ctdev backup` is now a per-machine restic flow, not a repo config exporter.** The
  old `ctdev backup <service>` / `ctdev restore <service>` (which exported Pi-hole lists
  to committed text files and SOPS-encrypted custom DNS) is **removed** — it duplicated
  what restic already captures (Pi-hole's `etc-pihole`/gravity.db lives under a backed-up
  path). `ctdev backup now` snapshots whatever machine you're on to its own restic repo;
  `ctdev backup snapshots [primary|local]` lists that machine's snapshots (tagged by
  hostname); `ctdev restore ls|files|in-place|check` wraps the restore helper. The
  `restic-backup.sh`/`restic-restore.sh` scripts are no longer hardcoded to the `ctpi01`
  homelab — they read paths from `/etc/restic/backup-paths`, work with any restic backend
  via `RESTIC_REPOSITORY` (+ optional `RESTIC_REPOSITORY_LOCAL`), and tag by `$(hostname)`.
- **Secrets are no longer stored in the repo.** All `*.sops.env` host secrets, the
  `*.sops.json` custom-DNS export, `.sops.yaml`, and `SECRETS.md` are deleted, along with
  the SOPS+age secret workflow. Each secret is now entered at its `configure` step and
  stored only on the host that needs it (`/etc/restic/restic.env`, `~/<svc>/.env`); if
  lost, you reconfigure. restic backs up the rendered `.env` files, so a restore brings
  them back. The `sops`/`age` *tool installers* remain available as components.

### Added
- **`ctdev backup paths`** — a local web UI (bound to `127.0.0.1`, token-guarded) to pick
  what restic backs up. Browse the machine's folders with lazily-computed sizes and
  Include/Exclude a folder (and its contents), exclude a single file, or exclude files
  like it (`*.ext`); saving writes `/etc/restic/backup-paths` and `backup-excludes`. Sort
  entries by name, size, or creation time (real `statx` birth time on Linux), ascending or
  descending. Runs as your user (browses `$HOME` fully); root-only dirs are shown locked
  but still sizeable and includable. Prints the URL for SSH port-forwarding on headless hosts.
- **`ctdev configure restic`** — interactive setup for restic backups: prompts for the
  repository (Backblaze B2 / S3 / SFTP / local), backend credentials, and a repository
  password (can generate one), writes `/etc/restic/restic.env` (0600), seeds default
  `backup-excludes`, runs `restic init`, and enables the daily timer. `--show` prints the
  current config (secrets redacted). Backups are **opt-in** — `configure restic` no longer
  auto-includes `$HOME`; nothing is snapshotted until you pick folders with `ctdev backup
  paths`, and the nightly job exits cleanly (not an error) while nothing is selected.
- `ctdev backup now` / `backup snapshots` now pre-check that restic is installed **and
  configured**, returning a clean "run `ctdev configure restic`" message instead of
  failing inside the shell script.

### Changed
- **Every configure wizard now cancels cleanly on the first Ctrl-C.** git, aws, caddy, and
  the category wizards moved onto the same context-aware prompt layer as `configure restic`
  (a bare stdin read used to swallow the first SIGINT and appear to hang). Invalid
  toggle/picker input now says "Invalid choice, keeping current value" instead of silently
  keeping it, and prompts show the default alongside the current value.
- **`ctdev update`'s apply phase runs in the progress TUI.** Each source (apt, per-flatpak,
  brew, runtimes, CLI tools, per-docker-stack, ctdev itself) is a step with a spinner,
  streamed output, and a summary — replacing the raw scrolled output that always ended in a
  green "Updates complete." banner. Failures now produce a non-zero exit, and Ctrl-C stops
  cleanly instead of cascading a warning per remaining source.
- **`ctdev cleanup` confirms before deleting**: after the picker it lists the selected tasks
  with the total size estimate and asks `Clean these? [Y/n]`.
- **`NO_COLOR` and piped output are honored everywhere** (previously only `ctdev info`):
  styling is disabled globally when `NO_COLOR` is set or stdout isn't a terminal, and a
  piped stdout now routes to the plain batch path instead of launching the alt-screen TUI
  into the pipe.
- TUI polish: the picker filter accepts spaces and non-ASCII (rune-safe backspace); `n` is
  no longer a surprise alias for select-none; `ctdev install` prints what dependency
  resolution added before installing; pickers and progress screens are labelled `(dry run)`;
  the progress list windows to the terminal height on long runs.

### Fixed
- **Interactive `ctdev install`/`uninstall`/`update` exit non-zero when something failed**
  (the TUI showed the failure but the process exited 0). The summary header is honest
  (`✗ Installation finished with N failure(s)`), and each failed component replays the last
  lines of its output so the reason (the apt/dpkg/compose error) survives — not just
  `exit status 1`. Quitting mid-run reports how many components never ran.

### Security
- **Secrets are read with masked input** (`term.ReadPassword`): the restic repository
  password, B2/S3 credentials, and Cloudflare token no longer echo to the terminal or land
  in scrollback/tmux capture. New restic passwords are confirmed with a second entry, and
  the terminal state is restored if you Ctrl-C mid-prompt.
- **`install.sh` fails closed on checksum verification**: a missing `SHA256SUMS`, missing
  `sha256sum`, or absent entry now aborts instead of warning-and-installing.
  `CTDEV_SKIP_VERIFY=1` is the explicit escape hatch for pre-checksum releases.
- **Portainer's and Beszel's plain-HTTP admin ports are no longer LAN-reachable.** Docker's
  iptables rules bypass UFW, so `9000:9000`/`8090:8090` listened network-wide even on
  firewalled hosts; they now bind to `127.0.0.1` and the docker bridge gateway (which is
  how Caddy reaches them). Portainer's TLS `9443` stays published for direct access.
  Re-run `ctdev install portainer` / `beszel` on existing nodes to apply.
- **`ctdev backup paths` no longer carries its session token in the URL.** The launch URL
  holds a single-use boot token that is exchanged for an `HttpOnly`, `SameSite=Strict`
  cookie and redirected away, so nothing sensitive lingers in browser history or `ps`
  argv; replaying the link gets a 403. The loopback-Origin check now parses the Origin and
  matches the hostname exactly (`localhost.attacker.com` no longer passes).

### Removed
- `ctdev backup <service>` / `ctdev restore <service>` (the repo config export/import).
- `component/configs/pihole/*.txt` list files; all `component/configs/*/hosts/*.sops.env`;
  `.sops.yaml`; `SECRETS.md`.

## [11.1.1] - 2026-06-30

### Fixed
- `ctdev update` no longer lists every system package twice on Linux Mint. Both `apt list --upgradable` and `mintupdate-cli list` enumerate the same APT database — once by binary name (`libnss3`, `libsqlite3-0`) and once by source name (`nss`, `sqlite3`) — so each pending update showed up under both "System Packages (apt)" and "System Packages (Mint)" (and would be upgraded twice). apt is now the sole system-package source; the mintupdate scanner is removed. apt already flags `linux-*` as `KERNEL` and upgrades kernels via `apt install --only-upgrade`, so kernel updates still surface — under the single apt group with the full current → new version display.
- `ctdev cleanup`'s "Orphaned packages & old kernels" task now pins the running kernel (`apt-mark manual linux-image-$(uname -r)` plus headers, when installed) before `apt-get autoremove --purge`. Mint omits apt's `apt-auto-removal` hook, so the protect-list that normally shields the booted kernel isn't generated; pinning guarantees `--purge` can't remove the kernel you're running even if you clean up before rebooting into a freshly-installed one.

## [11.1.0] - 2026-06-26

### Changed
- **`ctdev cleanup` is now a cross-platform disk reclaimer** (Linux + macOS), modeled on CleanMyMac but built in. It scans for reclaimable space without changing anything, shows the size next to each task and a total, then lets you pick what to clean in the grouped multi-select TUI — safe tasks preselected, riskier ones shown unchecked, and user data (e.g. iOS device backups) only reported, never deleted. `--dry-run` previews sizes and stops; `--batch` runs the safe tier non-interactively. Tasks are gated to the tools actually present on the machine.
  - **Linux:** apt autoremove --purge (orphans + old kernels, keeping a fallback), apt cache, `rc`-package config purge, journal vacuum (7d), coredumps/crash dumps, rotated logs, old snap revisions, unused flatpak runtimes, thumbnail cache, `~/.cache`, Docker prune, trash.
  - **macOS:** brew cleanup + autoremove, `~/Library/Caches`, `~/Library/Logs`, dev caches (npm/yarn/pip/go-build), Xcode (DerivedData, device support, simulator caches, `simctl delete unavailable`), Docker prune, trash, `.DS_Store` sweep, system caches, Time Machine local snapshots.
- Fixed the duplicate-APT-repository audit, which previously flagged false positives on modern deb822 `.sources` files (every stanza repeats `Types:`/`Components:`); it now compares repository identity (URI + suite) and understands both `.list` and `.sources`. The audit is report-only — it surfaces duplicates without editing your sources.

### Fixed
- `ctdev configure <unknown>` (e.g. `configure restic`) no longer silently runs the entire configuration sweep; it errors with the list of valid categories. Only a bare `ctdev configure` runs everything.

## [11.0.0] - 2026-06-26

### Changed (breaking)
- **`ctdev pihole export` / `ctdev pihole import` are gone**, replaced by general `ctdev backup` / `ctdev restore` verbs. `ctdev backup [service...]` exports a service's version-controllable state to text files you can commit (defaults to every backup-capable service — just Pi-hole today); `ctdev restore [service...]` re-applies them. For Pi-hole that's the same list/custom-DNS export and additive gravity rebuild as before — `ctdev backup pihole` / `ctdev restore pihole`. The `--out`/`--from` directory flags carry over to `backup`/`restore`.
- **`ctdev gpu` is gone**, folded into `ctdev configure gpu` (which already existed as a settings category). `ctdev configure gpu` now applies the GPU settings category **and** runs the NVIDIA/Secure Boot (MOK) driver-signing setup; `ctdev configure gpu --show` replaces `ctdev gpu info` (hardware + signing status), and `ctdev configure gpu --recover` replaces `ctdev gpu setup --recover`. The signing flow only runs on an explicit `configure gpu`, never in the full `ctdev configure` sweep.

### Added
- `ctdev backup` also fronts the restic data backups: `ctdev backup now` runs a snapshot immediately (`restic-backup.sh`) and `ctdev backup snapshots [b2|local]` lists snapshots (`restic-restore.sh snapshots`). Both shell out via sudo and report an actionable error when the restic component isn't installed.

## [10.16.0] - 2026-06-26

### Added
- The TUIs now **adapt to a light terminal background**. Secondary (dimmed) and primary (bright) text colors flip between dark- and light-theme values so near-white labels and washed-out gray detail stay legible on a light terminal; brand accent colors (green/red/etc.) stay fixed since they read on both. The interactive lists (`tui/multiselect`) and the install progress screen detect the background asynchronously via Bubble Tea's `BackgroundColorMsg`, so startup never blocks — important over Mosh and other layers that may not answer the query; the standalone `ctdev info` renderer detects synchronously (2s-bounded, dark default on no reply). The selection cursor bar keeps a fixed light-on-dark chip so it reads as a solid highlight on either theme.
- **Accessibility**: setting the `ACCESSIBLE` environment variable drops the live TUIs in favor of the plain, line-by-line path (the Charm-ecosystem convention) for screen-reader users — `ctdev update` applies non-interactively and `ctdev install`/`uninstall` take explicit component arguments.

### Changed
- The `ctdev update` checklist no longer pre-selects **MAJOR** version bumps or **KERNEL** updates — they start unchecked and you opt into them, so hitting Enter on the defaults can't apply a kernel/major upgrade. Batch/`-y` "update everything" is unchanged.
- The install/uninstall progress bar now **resizes to the terminal width** (clamped) instead of a fixed 40 columns, and long list rows are clipped to the terminal width so they can't wrap and desync the scroll on narrow terminals.
- `ctdev info` now honors `NO_COLOR`: it skips the background-color query and strips all color escapes from its output (the interactive TUIs already respect `NO_COLOR` via Bubble Tea's renderer).

## [10.15.0] - 2026-06-26

### Changed
- The interactive `ctdev update` checklist and the `ctdev install`/`uninstall` picker now share a single grouped multi-select widget (`tui/multiselect`), so they look and behave identically. The big fix: long lists (e.g. a 30-package apt run) now **scroll** — a sticky title/header and footer with a scrollable middle and `↑ N more` / `↓ N more` indicators, instead of overflowing off-screen. Other improvements both screens gain: a full-width cursor highlight bar; incremental `/` filtering; `a` all · `A`/`n` none · `i` invert (scoped to the filtered subset); `space` to toggle an item or tri-state-select a whole group from its header, `tab` to collapse/expand a group; `g`/`G` jump to top/bottom; per-group counts (`· N ✓` / `· n/N`); aligned name/version columns with ellipsis truncation; and a discoverable help bar (`?` toggles compact↔full) built from the keymap. Update severity now renders as `KERNEL`/`MAJOR` badges (colored block with the label inside, so it reads without color), and the update screen labels every source (including `docker`, `mintupdate`, `brew-cask`).

## [10.14.0] - 2026-06-25

### Added
- `ctdev update` now updates the Docker compose stacks it manages (pihole, caddy, beszel, portainer) alongside system packages, via a new `docker` update source. It discovers installed stacks from the registry (any component whose `DetectPath` is a `docker-compose.yml`) and checks each for a newer image **without pulling**: registry images compare the local `RepoDigests` index digest against the registry's current index digest (`docker buildx imagetools inspect`), and locally-built images (caddy) are tracked through their Dockerfile base images — the remote base digests are recorded in a `~/<stack>/.ctdev-base-digests` marker (seeded on first sight, flagged when a base moves, refreshed after a rebuild) so the stack isn't flagged on every run. Selected stacks update through the same checklist as everything else: `docker compose pull && up -d` for registry stacks, `docker compose build --pull && up -d` for built ones. `ctdev update --check` lists container updates read-only (no pulls, no marker writes). Note: for caddy this tracks base-image bumps, not new versions of the xcaddy-built `caddy-dns/cloudflare` plugin.

## [10.13.0] - 2026-06-25

### Changed
- Pi-hole stack: ship a ctdev-managed Unbound tuning drop-in (`num-threads: 2`, `so-reuseport: yes`, `incoming-num-tcp: 100`, `outgoing-num-tcp: 100`), single-file bind-mounted into the image's `custom.conf.d/` so the baked-in `private-domains.conf` is preserved. Stock Unbound runs single-threaded with `incoming-num-tcp: 10`, so a burst of concurrent TCP fall-back queries (answers exceeding `edns-buffer-size` 1232, retried over TCP) could overflow the cap and get dropped, surfacing as FTL `CONNECTION_ERROR` / "Connection prematurely closed by remote server" warnings against `127.0.0.1#5335`. More threads plus a larger TCP backlog absorb the bursts.

## [10.12.0] - 2026-06-23

### Added
- New `nomachine` component (Linux/amd64): installs the NoMachine remote desktop server from the pinned NoMachine `.deb`, then scopes its NX port (4000) to the `tailscale0` interface via UFW so the desktop is reachable across the tailnet but never exposed to the LAN or internet. Uninstall removes the rule and the package. The version is pinned in `component/nomachine.go` (no apt repo / working "latest" URL upstream); bump the constants to upgrade.

## [10.11.0] - 2026-06-21

### Added
- **SECRETS.md**: documents the SOPS + age encryption workflow — what's encrypted, where the age key lives, and how to view/edit/deploy/rotate secrets (including the special case of rotating the restic repo password via `restic key`).
- SOPS-encrypted per-node secrets for **restic** (`/etc/restic/restic.env`: repo password, B2 keys, repo paths), **beszel** (`~/beszel/.env`: agent KEY/TOKEN), and **pihole** (`~/pihole/.env`: admin password, TZ) under `component/configs/<svc>/hosts/<node>.sops.env`, with matching `.sops.yaml` rules. Caddy's host secret was already encrypted; now every node secret is recoverable from the repo with just the age key.

### Changed
- restic now keeps its password **in** `/etc/restic/restic.env` as `RESTIC_PASSWORD` (single SOPS-encryptable file) instead of a separate `/etc/restic/password` file; the backup/restore scripts `set -a` before sourcing so the values export to restic. `RECOVERY.md` updated so a rebuild restores the age key and decrypts the committed secrets rather than recreating them by hand.

## [10.10.0] - 2026-06-21

### Added
- New **restic** component: encrypted backups with a daily systemd timer. Installs restic and deploys `/usr/local/bin/restic-backup.sh` (snapshots the stack dirs under `$HOME` plus the Docker named-volume data dirs to an offsite Backblaze **B2** repo, and to a **local USB** repo at `/mnt/backup` when mounted, then prunes to 7 daily / 4 weekly / 6 monthly), a `restic-restore.sh` helper (`snapshots`/`ls`/`restore`/`restore-in-place`/`check`), and `restic-backup.{service,timer}`. Repo locations, B2 credentials, and the repository password live in `/etc/restic/` (root-only, **never committed**); the timer is enabled only once that config exists.
- **RECOVERY.md**: a detailed, copy-paste disaster-recovery runbook — single-file/folder restores, per-service restores, and a full bare-metal rebuild (flash OS → ctdev → restore `/etc/restic` → `restic-restore.sh restore-in-place` → bring stacks up), plus backup-verification steps and a command cheat sheet.

## [10.9.0] - 2026-06-21

### Changed
- The **pihole** component now deploys a dnsmasq tuning drop-in (`~/pihole/etc-dnsmasq.d/01-ctdev.conf`) that raises `dns-forward-max` to `500` (dnsmasq default: 150). With a recursive Unbound upstream, each in-flight query holds a forward slot longer, so a burst can transiently exhaust the default and log "Maximum number of concurrent DNS queries reached"; 500 gives headroom at negligible cost. Applied when the Pi-hole container (re)starts.

## [10.8.0] - 2026-06-20

### Added
- New **beszel** component: Beszel server/container monitoring (hub + agent) as Docker containers deployed to `~/beszel/`. Depends on `docker`; the hub (web UI + data) is published on `8090` and reverse-proxied by Caddy at `https://beszel.<domain>` (new `@beszel` route alongside `@portainer`), and the agent monitors the host over a shared unix socket (no extra TCP port). `ctdev install beszel` brings the hub up first; create the admin user, click "Add System", put the issued `BESZEL_KEY`/`BESZEL_TOKEN` in `~/beszel/.env`, then re-run to start the agent. Keep it off any public network (Tailscale only). Per-container memory stats require the kernel memory cgroup, which is off on some customized Pi boot images (`cgroup_disable=memory` in the device-tree bootargs).

### Changed
- Added container **healthchecks** to the stacks whose images support one: `caddy` (admin API on `127.0.0.1:2019`), and `beszel`/`beszel-agent` (the binaries' built-in `health` subcommand). Pi-hole already ships its own; Portainer and Unbound are distroless (no shell/health tool) so they rely on the `restart: unless-stopped` policy.

## [10.7.0] - 2026-06-20

### Added
- New **portainer** component: Portainer CE as a Docker container (deployed to `~/portainer/`, users/settings persist in the `portainer_data` volume) giving a web UI to view and manage the host's containers, images, volumes, and compose stacks. Depends on `docker`; serves plain HTTP on `9000` (reverse-proxied by Caddy at `https://portainer.<domain>`) and its own self-signed HTTPS on `9443` for direct LAN/Tailscale access. The Caddyfile gains a `@portainer` route alongside `@pihole`. No `configure` step — create the admin user on first web login.

## [10.6.0] - 2026-06-20

### Changed
- Trimmed the bundled Pi-hole adlists (21 → 10): HaGeZi Pro++ stays the primary all-round list, plus unique-category lists (gambling, spam-TLDs, smart-TV, fakenews, first-party trackers) and authoritative threat feeds (HaGeZi TIF + Hoster, ThreatFox, URLhaus). Dropped overlapping general lists (StevenBlack, oisd, AdAway, Disconnect, EasyList, redundant crypto/malware mirrors) per HaGeZi's "don't stack" guidance, and removed HaGeZi's referral *allowlist* that was mistakenly loaded as a blocklist.

## [10.5.0] - 2026-06-20

### Added
- Optional **Unbound** recursive resolver in the Pi-hole stack: an `unbound` sidecar (klutchell/unbound) bound to host loopback `127.0.0.1:5335`, plus a "Local recursive (Unbound)" upstream choice in `ctdev configure pihole` that points Pi-hole at `127.0.0.1#5335` for recursive, DNSSEC-validating resolution from the root servers (no third-party upstream sees your queries).

### Changed
- The Pi-hole container no longer pins `dns.upstreams`/`dns.listeningMode` via `FTLCONF_` env vars — `ctdev configure pihole` owns them and the choices now persist across container restarts (Pi-hole v6 reverts an env-set value to default once the env var is removed, so these are written to `./etc-pihole` instead).

## [10.4.0] - 2026-06-20

### Changed
- The `pihole` component now runs Pi-hole as a **Docker container** (official `pihole/pihole` image, host networking) instead of the native unattended installer — matching the `caddy` model, with `docker` as a prerequisite. The stack lives in `~/pihole/`; gravity.db and config persist in `./etc-pihole`. `ctdev configure pihole`, `ctdev pihole import`/`export`, and the Caddy Pi-hole wiring all detect a containerized Pi-hole and run against it via `docker exec`, falling back to a native host install when present.

## [10.3.0] - 2026-06-20

### Added
- `ctdev pihole export` / `ctdev pihole import` to version-control Pi-hole's configuration. Export writes the adlists, allow/deny lists, and allow/deny regex filters as plain-text files under `ctdev/component/configs/pihole/` (diffable in git); import applies them back and rebuilds gravity, so a node's lists are reproducible. Custom DNS records (internal hostnames → private IPs) are SOPS-encrypted to `hosts/<node>.sops.json` instead of committed in the clear.

### Fixed
- `.sops.yaml` now points at the `caddy` host-config path (it was left at the old `homelab` path after the component was renamed).

## [10.2.0] - 2026-06-19

### Added
- `ctdev update --no-refresh` to skip the pre-scan APT index refresh.

### Fixed
- `ctdev update` now refreshes the APT package index (`apt-get update`) before scanning, so it can no longer report a machine is current while security updates are actually pending. Skipped in dry-run and with `--no-refresh`.
- `ctdev update` surfaces scanner failures instead of silently dropping them: a source that errors or panics (rate-limited GitHub API, broken `apt list`, failed `git fetch`) is reported as "N update source(s) failed to check" rather than masquerading as "everything is up to date" (`-v` for per-source detail).
- Update scanners distinguish a real check failure (tool present but its latest-version lookup failed) from "can't determine" (tool absent, or no nodenv/ruby-build definitions), so a transient network error no longer hides available updates.
- `ctdev verify` derives its checks from the components actually installed on the machine, so a headless/homelab node (e.g. Pi-hole) no longer fails for desktop tooling it was never meant to have. Adds `pihole`/`pihole-FTL` checks; `ufw` and never-suspend are now informational rather than failures.
- Uninstalling an APT-repo component (gh, vscode, terraform, dbeaver, docker, 1password) now removes its `signed-by` source list and keyring instead of leaving a dangling repo behind — and additionally drops the user from the `docker` group (docker) and removes the debsig policy/keyring (1password).
- Checksum-file verification tolerates the `sha256sum -b` binary marker (`hash *file`), uppercase hashes, and trailing columns, so an upstream checksum-format change won't silently break verified installs/updates.
- `1password` and `slack` install-time detection now uses the registry's own detection (the app bundle on macOS) instead of a command-name lookup, avoiding needless reinstall churn.

## [10.1.0] - 2026-06-18

### Added
- New `pihole` component: installs Pi-hole via its official unattended installer and applies base config (Cloudflare upstreams, listen-on-all).
- New `ctdev configure pihole` category: pick upstream resolvers, listening mode, and blocking on/off (shown only on nodes where Pi-hole is installed).
- New `caddy` component (depends on `docker`): deploys a Caddy reverse-proxy stack to `~/caddy/` serving `https://*.<domain>` with a Let's Encrypt wildcard (Cloudflare DNS-01, nothing exposed to the internet) and brings it up.
- New `ctdev configure caddy`: sets the domain, ACME email, and Cloudflare token (`~/caddy/.env`) and, when Pi-hole is present, frees port 443 and points `*.<domain>` at the node's Tailscale IP. Optional per-node SOPS host configs live at `ctdev/component/configs/caddy/hosts/<node>.sops.env`.

### Changed
- `ctdev configure remote` is split into focused categories — `ssh`, `ufw`, `locale`, `sleep`, `linger`, and `tunnel` — so each system concern is configured on its own rather than as one bundle. WiFi power-save moved under `ctdev configure network`.
- `ctdev install <component>` now runs that component's configuration step afterward when it has one (e.g. `install pihole` → `configure pihole`, `install caddy` → `configure caddy`). Re-running `install` on an already-installed component says so and goes straight to configuration. `ctdev configure <component>` still configures without installing. Declared prerequisites (component `Dependencies`, e.g. caddy → docker) are installed first.

### Removed
- `bootstrap.sh` and `bootstrap-homelab.sh`, and the `homelab` umbrella component. There's no all-in-one machine bootstrap anymore — install the `ctdev` binary (`install.sh`) and compose the machine from individual `ctdev install <component>` and `ctdev configure <category>` calls.

### Fixed
- APT keyrings are now written world-readable (0644) so Debian 13's sandboxed `sqv` verifier can read them; previously `ctdev install gh` (and other apt-repo components) failed on trixie with "repository is not signed".

## [10.0.0] - 2026-06-03

### Security
- vscode and dbeaver GPG keys are now written to `/usr/share/keyrings/` and scoped via `signed-by=` instead of `/etc/apt/trusted.gpg.d/`, so a compromise of either vendor key can no longer authenticate packages for unrelated repos.
- The ghostty installer is pinned to a tagged release of the third-party `ghostty-ubuntu` script instead of tracking `HEAD`, and `install.sh` verifies the downloaded `ctdev` binary against a published `SHA256SUMS` (CI now emits it), hardening the self-update path.

### Fixed
- Ctrl-C / SIGTERM now cancel in-flight installs, updates, and shell-outs everywhere — `Execute()` binds a `signal.NotifyContext` and the progress/update runners derive from it (previously batch mode and `update` had no signal wiring).
- `ctdev update` no longer offers version downgrades: scanners use a dotted-numeric `versionNewer` comparison instead of string inequality, so a locally-newer tool isn't "updated" backwards.
- A failed `apt` upgrade during `ctdev update` no longer aborts the other selected sources (brew/flatpak/runtime/cli/npm); it warns and continues like every other manager.
- The component picker now scrolls long lists (uses terminal height) and keeps the cursor on a real, visible match while filtering — previously the highlight could land on an off-screen or filtered-out row and toggle the wrong component.
- `ctdev info` detects the real terminal width and strips color when its output is piped or redirected (was emitting ANSI escapes and ignoring width).
- Skipped components now appear in the progress view and the summary tally, and a clean run no longer prints a red "0 failed".
- Terminal state is reset after the picker and checklist programs too, for consistent cleanup across all TUI entry points.

### Changed
- **Supported package managers narrowed to apt and brew.** The untested dnf and pacman branches were removed; components on those systems report as Skipped.
- Removed the write-only install-marker store (`state.MarkerStore`) — nothing read it; install state is derived from live detection.
- Internal refactors: `InstallAPTRepoPackage` helper for the keyring+repo+install pattern; shared download/verify skeleton across the helm/kubectl/terraform updaters; consolidated header/label/value styles into the `styles` package; replaced the `Executor` struct with package functions.
- Removed completed-migration design/plan docs under `docs/`.

## [9.2.0] - 2026-04-23

### Fixed
- **Ctrl-C now actually cancels in-flight work.** `sysutil.Run`/`SudoRun` and every wrapper (`InstallPackage`, `BrewCask*`, `APTUpdate`, `AddAPTKeyring`, `AddAPTSource`, `Service*`, `InstallBinary`, `SudoWriteFile`, `DownloadGitHubBinary`) now take `ctx` and use `exec.CommandContext`. `sysutil.DownloadFile`, `sysutil.GitHubLatestVersion`, `fetchGoReleases`, `fetchGitHubReleaseTags`, `fetchLatestKubectlVersion`, and `isGitSpiceInstalled` all honor ctx too — previously any of these could ignore Ctrl-C for up to 60s.
- `ctdev update --refresh-keys` was silently ineffective for VS Code — `aptKeyRefreshers["vscode"]` wrote the keyring to `/usr/share/keyrings/packages.microsoft.gpg` while APT's `signed-by=` points at `/etc/apt/trusted.gpg.d/packages.microsoft.gpg`. Paths now match, plus a test locks in the invariant across all refreshers.
- `fonts` Linux install used a predictable `/tmp/<name>.zip` path with no cleanup on extract failure. Now uses `os.MkdirTemp` with a `defer RemoveAll`.
- `fonts` component's `IsInstalled()` was always false on macOS because `DetectPath` is exclusive — Linux and macOS font paths both live in `DetectApps` now.
- `nodenv install` / `rbenv install` under `--force` no longer fail when the target runtime version is already present (added `--skip-existing`).
- Chrome and Slack `.deb` installs no longer silently treat an `apt-get -f`-fixed-but-still-broken install as success. Shared `installDebWithDepFix` helper verifies via `dpkg -s` after the fix.
- `ghostty` (dnf) and `tailscale`/`chrome`/`docker`/`dbeaver`/`gh`/`vscode`/`ruby`/`terraform` unsupported-PM branches now return wrapped `ErrUnsupportedOS` via `unsupportedPMError`, so the executor reports them as Skipped instead of Failed.
- `git-spice` uninstall no longer deletes `/usr/local/bin/gs` without confirming it's actually git-spice (vs Ghostscript).
- `terraform` uninstall correctly handles pacman via explicit switch; `IsPackageInstalled` now supports pacman (`pacman -Qi`).
- `sysutil.DownloadFile` removes the partial destination file on copy failure instead of leaving zero/partial bytes behind.
- `state.MarkerStore.Save` writes atomically (temp file + rename), so a kill mid-write no longer corrupts the marker and breaks the next `Load`.
- macOS `defaults write` failures propagate — previously every `run(...)` error was silently dropped; now each failure is logged and `ApplyMacOSDefaults` returns a joined error.
- GRUB config append now guarantees a trailing newline on the preceding line, so the appended variable can't merge with the last existing line.
- Progress TUI progress bar now reaches 100% when the final component is skipped (was stuck because `InstallSkipMsg` didn't call `SetPercent`).
- `update --refresh-keys` no longer runs under `--check` (which is read-only).
- `configure <category>` now prints a notice when hardware gating yields zero applicable settings (was a silent no-op).
- `git uninstall` only prints `.gitconfig.local preserved` when that file actually exists.
- `picker.GetResult()` returns selected components in display order (was random map iteration).
- `Go` tarball download now verifies sha256 from `go.dev/dl/?mode=json` before replacing `/usr/local/go`.
- `TestRun_CancelInterruptsChild` strengthened: previously could pass even when ctx-cancel didn't kill a running child.

### Changed
- **ApplyFunc / PostApplyHook signatures**: `setup.Setting.ApplyFunc` now `(ctx, sysutil.Opts, value) error` (was `(value) error`); `PostApplyHook` now `(ctx, sysutil.Opts) error`. Setup apply helpers go through `sysutil.Run`/`SudoRun` instead of a private package-level `run`/`sudoRun`.
- `ctdev update` flow and `updateHelm`/`updateKubectl`/`updateTerraform`/`updateGo` helpers all route shell-out through `sysutil` — `--dry-run` is now honored for update paths that previously ignored it, and output routing is uniform.
- `configure` wizard subcommands now derived from `setup.Slugs(setup.Registry)` at init time instead of a hand-kept `slugOrder` list — adding a new setting slug auto-registers the subcommand.
- `cleanup` prompt reads stdin through the shared `promptLine` scanner instead of `fmt.Scanln`, matching every other prompt path.
- `applyXbindkeys` deploys `.xbindkeysrc` + `.desktop` via `sysutil.DeployFileFromFS` so pre-existing user customizations get backed up on diff instead of silently overwritten.
- `progress` TUI extracted `runOneComponent` + `msgSender` interface for testability; `runWithProgress` now waits on a `workerDone` channel so install goroutines can't outlive the function.
- Shared `installDebWithDepFix`, `unsupportedPMError`, and `alreadyInstalled` helpers in `component` — consolidated duplicate patterns.

### Added
- `component.RegisterForTest`, `cmd.captureSender` (fake `tea.Program.Send`), PATH-stubbed fake-binary helper for component tests, and a `captureStdout` helper for wizard output.
- Tests: ctx-cancel termination (sysutil), chrome/slack/terraform install paths, git-spice PATH-based detection, picker determinism, progress bar skip-percent, deploy-order stability for zsh, APT keyring path invariant.

## [9.1.0] - 2026-04-08

### Added
- `ctdev configure` — interactive wizard that walks through all system configuration categories
- `ctdev configure <category>` subcommands: gpu, boot, power, keyboard, mouse, audio, bluetooth, desktop, network, system
- `ctdev configure --show` — show current system configuration (read-only)
- WiFi suspend fix now uses PCIe-level reset with dynamic PCI address discovery

### Fixed
- `nodenv version` returning empty output would panic and crash `ctdev update`
- Ruby installer interactive prompt deadlocked when running under the progress TUI
- `kubectl` stable version fetch had no HTTP timeout — could hang indefinitely
- Scanner goroutines in `ctdev update` now recover from panics instead of crashing the process
- `promptLine` scanner leak — was creating a new `bufio.Scanner` per call, could drop buffered input

### Changed
- **Breaking:** Removed `ctdev setup` command — replaced by `ctdev configure`
- Collapsed `applyDconfInt`/`applyDconfBool`/`applyDconfDouble` into single `applyDconf`
- Removed dead code: `internal/shell` package, `state.LoadConfig`, `Setting.TechDetail`, duplicate `nvidiaLoaded()`
- Settings registry now uses `Slug` field for category routing
- Added tests for `formatSliderVal`, `FilterBySlug`, `Slugs`, `SudoWriteFile`, registry slug coverage

## [9.0.20] - 2026-04-08

### Fixed
- `scanAll` race condition — printer goroutine could write to stdout after caller returned
- `updateGo` now extracts to a temp directory and verifies the binary before replacing `/usr/local/go`
- `sed` metacharacters in GRUB edits — switched to `|` delimiter and escape regex metacharacters (e.g. `.` in `nvidia.NVreg_*` params)
- `updateHelm`/`updateKubectl`/`updateTerraform` now verify SHA256 checksums before installing
- VS Code DNF installer no longer uses `bash -c echo` shell string — uses temp file + `sudo cp` instead
- `IsInstalled` detection logic simplified — `DetectPath` is now a clear early exclusive return

### Changed
- Added `execOpts()` helper — replaced 66 identical `sysutil.Opts{...}` lines across all 33 component files
- Added `SudoWriteFile` to sysutil for safely writing root-owned files
- Extracted `cpsToIntervalMs` and `sedEscape` as testable pure functions
- Removed dead code: `detectGsettingsString`, `BoolEnv`, `joinArgs`, `CacheDir`, `UpdateInterval`
- Simplified `detectArch()` no-op switch to `return runtime.GOARCH`

## [9.0.19] - 2026-04-07

### Changed
- Replaced static zsh completion file with Cobra-generated completions — commands, flags, and component names stay in sync automatically

### Fixed
- `ctdev update` upgrading all mintupdate packages instead of only the ones selected in the checklist — now uses `-i` ignore flag for unselected packages

## [9.0.18] - 2026-04-07

### Fixed
- `ctdev update` upgrading all mintupdate packages instead of only the ones selected in the checklist — now uses `-i` ignore flag for unselected packages

## [9.0.17] - 2026-04-07

### Fixed
- NVIDIA suspend setup failing on static systemd units (nvidia-persistenced) — skip services without an [Install] section instead of erroring

## [9.0.16] - 2026-04-07

### Fixed
- Middle mouse button window overview — use dbus ShowOverview instead of xdotool ctrl+alt+Down (which was mapped to workspace switch, not exposé)
- xbindkeys dbus commands now set DBUS_SESSION_BUS_ADDRESS explicitly so they work from the xbindkeys daemon
- Removed redundant symlinked xbindkeys config in favor of the embedded config deployed by `ctdev setup`

## [9.0.15] - 2026-04-04

### Added
- Logitech KVM mouse fix — udev rule + systemd user service to restart Solaar when Logi Bolt receiver reconnects after KVM switch, fixing middle-click
- Hide drives — udev rule to hide Windows/secondary NVMe partitions from the file manager
- New "Peripherals & KVM" category in `ctdev setup`
- `mintupdate-cli` scanner in `ctdev update` — detects kernel and security updates managed by Linux Mint's Update Manager
- Reset support for new udev rules and systemd service
- Tests for new registry entries, embedded configs, and mintupdate parser

## [9.0.14] - 2026-04-03

### Fixed
- Viper config path not expanding `$HOME` — config file was never loaded
- Goroutine race in TUI progress — scanner could read from closed pipe, losing final output lines
- Symlink removal error silently swallowed in `DeployFile`
- `defer os.Remove` inside loop in fonts installer — zip files not cleaned up on early return
- `install.sh` `download()` missing error when neither curl nor wget available
- `os.Getenv("HOME")` replaced with `os.UserHomeDir()` in setup apply/reset

### Added
- Comprehensive test coverage: ~100 new test cases across all packages
- Tests for setup settings (NeedsApply, FilterByHardware, InitStates, Categories)
- Tests for component detection (IsInstalled, GroupByCategory, SupportsOS)
- Tests for TUI components (checklist keyboard nav, picker filtering/selection, disk bar rendering)
- Tests for platform parsing (snapToStandardSize, parseIPLink, parseMacNetworkSetup)
- Tests for sysutil (exec dry-run, APT dry-run, deploy symlink handling)
- Tests for GPU info helpers (mibToGB, orDefault)
- Dry-run path tests for macOS defaults and Linux reset
- Race detector (`-race`) enabled in CI test step

## [9.0.13] - 2026-04-03

### Changed
- Combined Test and Release workflows into single CI workflow — releases now require all tests to pass first
- Updated GitHub Actions to latest versions (checkout v6, setup-go v6, upload-artifact v7, download-artifact v8) — fixes Node.js 20 deprecation warnings
- Fixed go.sum cache path for setup-go action
- Release workflow handles existing releases by replacing them with fresh builds

## [9.0.12] - 2026-04-03

### Fixed
- macOS app detection: add DetectApps field so IsInstalled() checks /Applications .app bundles for 13 desktop components (1password, chrome, claude-desktop, ghostty, slack, etc.)
- Fonts detection on macOS now checks ~/Library/Fonts in addition to Linux font path
- Linear installer now skips if already installed
- Network info on macOS filters inactive interfaces (Thunderbolt, virtual ethernet)

### Added
- Brew cask update support: `ctdev update` now scans and upgrades outdated brew cask apps (ghostty, chrome, slack, docker, etc.)

## [9.0.10] - 2026-04-02

### Fixed
- Components with configs (tmux, ghostty, claude-code, zsh) now always deploy configs even when already installed — re-running `ctdev install <component>` keeps dotfiles in sync
- Use `all:configs` in go:embed to include dot-files (`.zshrc`, `.gitconfig`, `.tmux.conf`, `.xbindkeysrc`)
- Remove dangling symlinks from old bash-based install before deploying config files
- Fix DECRPM terminal response leak after TUI exit (switch to raw mode before draining stdin)
- Platform-specific terminal ioctls for macOS compatibility
- Skip sudo prompt during --dry-run
- Add batch/non-TTY progress mode for install/uninstall (CI compatibility)

## [9.0.0] - 2026-04-02

### Changed
- Complete rewrite from bash to Go — ctdev is now a single compiled binary
- All 35 component installers ported to native Go with proper error handling
- Bubble Tea TUI for interactive component picker, setup wizard, and update checklist
- Embedded config files via go:embed (no more symlinks)
- Install via GitHub Releases binary download instead of cloning the repo
- Self-update via `ctdev update` checks GitHub Releases for new versions

### Added
- `ctdev configure git` — interactive SSH key picker, signing key setup, GitHub upload
- `ctdev configure aws` — AWS profile picker from ~/.aws/config
- `ctdev gpu info` — GPU hardware info with NVIDIA signing status
- `ctdev gpu setup` — MOK enrollment and DKMS signing for Secure Boot
- Batch/CI mode — all commands work non-interactively when stdin is not a TTY
- Zsh completions for all commands including configure and gpu subcommands
- Progress TUI shows elapsed time and per-component output during install/uninstall

### Fixed
- APT keyring and source operations now use sudo correctly
- Component detection uses app bundles on macOS instead of CLI commands
- Setup key repeat rate correctly converts cps to milliseconds for gsettings
- Setup reset now undoes the same dconf paths that apply uses
- Kernel cleanup scoped to versioned packages only (won't remove meta-packages)
- GRUB backed up before any sed modifications
- Update functions use safe temp directories instead of predictable /tmp paths
- Pre-release versions filtered from GitHub release tag scanning

## [8.4.0] - 2026-03-08

### Changed
- Keyboard repeat rate set to 200ms delay / 50 cps via `xset r rate 200 50` in Linux Mint setup
- Updated gsettings keyboard delay (200ms) and repeat interval (20ms) to match xset values
- `ctdev setup --show` displays current xset repeat rate
- `ctdev setup --reset` resets xset to X11 defaults

## [8.3.1] - 2026-03-07

### Changed
- GRUB setup defaults to `menu` style with 10s timeout and OS prober enabled

## [8.3.0] - 2026-03-07

### Added
- xbindkeys mouse button bindings in `ctdev setup` (middle click → Expo via xdotool)
- Installs `xbindkeys` and `xdotool` packages, symlinks config, creates autostart entry
- xbindkeys status in `ctdev setup --show`
- xbindkeys cleanup in `ctdev setup --reset`

## [8.2.0] - 2026-03-05

### Removed
- Swapfile creation and `vm.swappiness` tuning from `ctdev setup` (not needed for Linux Mint)

## [8.1.0] - 2026-03-05

### Added
- Bluetooth/audio/camera support in `ctdev setup` (LDAC, PipeWire Bluetooth, v4l-utils, linux-firmware)
- WirePlumber LDAC Bluetooth audio config deployed from repo template
- Bluetooth service enablement in `ctdev setup`
- `vm.swappiness=10` tuning for high-memory desktops
- `ctdev install list` to show available components with install status (filtered by OS)
- New component: `solaar` (Logitech Unifying/Bolt receiver manager, Linux-only)
- New component: `tailscale` (Tailscale VPN, macOS + Linux)
- Bluetooth & audio status in `ctdev setup --show`

### Changed
- Simplified README to focus on getting started and commands
- `ctdev setup --reset` now removes WirePlumber config and swappiness tuning

## [8.0.0] - 2026-03-04

### Breaking Changes
- `ctdev list` removed (use `ctdev install --help` to see available components)
- `ctdev configure macos` and `ctdev configure linux-mint` removed (use `ctdev setup` instead)
- `ctdev cleanup kernels` and `ctdev cleanup apt` subcommands removed (use `ctdev cleanup` which runs all tasks sequentially)

### Added
- `ctdev setup --show` to display current system configuration
- `ctdev setup --reset` to reset system configuration to defaults

### Changed
- `ctdev setup` simplified to GPU setup + system configuration (update/cleanup removed from setup flow)
- `ctdev cleanup` now runs all tasks sequentially with prompts instead of requiring a subcommand

### Fixed
- `ctdev update` silently failing to update zsh, node, and ruby components (git pull used wrong branch name when `origin/HEAD` was unset)
- `ctdev update` showing bun as updatable even when already on latest version (now checks GitHub releases)
- `ctdev update` suppressing bun upgrade output with `2>/dev/null`
- `ctdev update` redundantly re-checking for component updates during the update phase

## [7.12.0] - 2026-03-03

### Added
- Comprehensive GPU driver signing for Secure Boot (`ctdev gpu status` and `ctdev gpu setup`)
  - Detect open vs closed NVIDIA driver variant
  - MOK clutter detection and cleanup (flags unexpected files in `/var/lib/shim-signed/mok/`)
  - DKMS `framework.conf` configuration for automatic module signing
  - Module signature verification against enrolled MOK certificate
  - `--recover` flag for re-enrolling MOK key after CMOS/firmware reset
  - DKMS rebuild with post-rebuild signature verification

### Changed
- `ctdev gpu` reduced to 2 subcommands: `status` and `setup` (removed `sign` and `info`)
- Zsh completion updated for new GPU subcommand structure

### Fixed
- CI test failures: removed references to nonexistent `cli` component, `--skip-system` flag, `~/.gitignore`, and `~/.gitconfig.local`
- CI `shellcheck components/cli/*.sh` step removed (directory doesn't exist)
- `ctdev info` crash on CI due to `pipefail` + `grep` in GPU and disk detection pipelines
- ShellCheck SC2034 warning for unused variable in `cmds/list.sh`
- `--dry-run install` now requires a component name (matches CLI behavior)
- Git configure `--skip` no longer ignores explicit `--name`/`--email` arguments

## [7.11.0] - 2026-03-03

### Added
- `earlyoom` component: Early OOM killer for Linux (apt, dnf, pacman)
- Swap file management in `ctdev configure linux-mint` (creates 8GB swap file, adds to fstab)
- Swap info display in `ctdev configure linux-mint --show`
- Swap reset in `ctdev configure linux-mint --reset`

### Changed
- Repository URLs updated to `ConnerTechnology/dotfiles`
- Git config: added SSH signing key, removed `compactionHeuristic`
- Claude Code settings: added `Bash(journalctl:*)` permission
- Component count updated to 35

## [7.10.1] - 2026-03-02

### Removed
- Dock auto-hide delay and animation speed overrides from macOS defaults (restores default macOS hide/show animation)

## [7.10.0] - 2026-03-01

### Added
- Dock auto-hide to macOS defaults
- Volume change feedback to macOS defaults

### Removed
- Design and implementation plan documents from `docs/plans`

## [7.9.1] - 2026-02-28

### Removed
- `AppleShowAllExtensions` from macOS defaults (prevents .app extensions showing on applications)
- `minimize-to-application` from macOS defaults (windows now minimize as separate dock tiles)

## [7.9.0] - 2026-02-25

### Changed
- Renamed `ctdev upgrade` to `ctdev update` — all flags and behavior preserved

### Removed
- `ctdev upgrade` command (replaced by `ctdev update`)

## [7.8.0] - 2026-02-19

### Added
- `ctdev upgrade --check` flag to list available updates without installing
- `ctdev upgrade --refresh-keys` flag (absorbed from `ctdev update`)
- macOS `softwareupdate` as an upgrade source
- Flatpak packages as an upgrade source
- Bun self-upgrade as an upgrade source
- NVIDIA kernel module re-signing after system upgrades (Linux, Secure Boot)

### Changed
- Merged `ctdev update` into `ctdev upgrade` — single command for all upgrade operations
- `ctdev update` now shows deprecation warning and forwards to `ctdev upgrade --check`

## [7.7.0] - 2026-02-18

### Added
- `check_installed_cmd` and `check_installed_app` utility functions in `lib/utils.sh`
- FORCE flag support for 9 macOS GUI app installs (1password, chrome, claude-desktop, cleanmymac, dbeaver, linear, logi-options, slack, vscode)
- Documented `ctdev gpu`, `ctdev configure linux-mint`, and `--refresh-keys` flag in CLAUDE.md

### Changed
- Refactored 23 component install scripts to use shared `check_installed_cmd`/`check_installed_app` utilities
- Standardized initial logging to `log_info` across all components (was mixed `log_step`/`log_info`)

### Fixed
- TradingView macOS install now uses Homebrew cask instead of manual DMG download
- Ghostty and tmux install scripts had wrong `DOTFILES_ROOT` path (`../../..` instead of `../..`)
- Ghostty macOS install now uses `install_brew_cask` instead of raw `brew install --cask`
- TradingView quarantine flag removed after install to prevent macOS Gatekeeper hang

## [7.6.1] - 2026-02-17

### Fixed
- Removed SSH URL rewrite from gitconfig that forced HTTPS clones to use SSH

## [7.6.0] - 2026-02-14

### Added
- Zsh tab completion for `ctdev` CLI
  - Completes all commands, global flags, and command-specific options
  - Component names with descriptions for `install`, `uninstall`, and `upgrade`
  - Configure targets (`git`, `macos`, `linux-mint`) with per-target flags
  - GPU subcommands (`status`, `setup`, `sign`, `info`)
  - Dynamically reads components from `lib/components.sh`
  - Installed via `ctdev install zsh`, symlinked to `~/.zfunc/_ctdev`

## [7.5.1] - 2026-02-14

### Fixed
- `ctdev configure --show` output now human-readable for both macOS and Linux Mint
  - Booleans display as `yes`/`no` instead of `true`/`false`/`0`/`1`
  - Time values display as `30 min`, `1 hr` instead of raw seconds with `uint32` prefixes
  - Mouse speed displays as `65%` instead of raw float
  - macOS Finder codes translated to names (`current folder`, `list`)
  - Quotes and type annotations stripped from all values

## [7.5.0] - 2026-02-14

### Added
- NVIDIA suspend stability in `ctdev configure linux-mint` (auto-detected when NVIDIA driver is loaded)
  - Adds `nvidia.NVreg_PreserveVideoMemoryAllocations=1` to GRUB kernel parameters
  - Enables `nvidia-suspend`, `nvidia-resume`, `nvidia-hibernate` systemd services
  - Warns if `amdgpu` module is loaded on dual-GPU systems
  - Supported in `--show`, `--reset`, and `--dry-run` modes
- Desktop freeze troubleshooting guide for NVIDIA + dual-GPU systems in TROUBLESHOOTING.md

## [7.4.0] - 2026-02-13

### Added
- `ctdev update --refresh-keys` to re-download expired APT repository GPG keys
  - Supports optional component filter: `ctdev update --refresh-keys docker gh`
  - Central key registry in `lib/keys.sh` for 6 components (docker, gh, 1password, terraform, vscode, dbeaver)
  - Respects `--dry-run` flag

## [7.3.0] - 2026-02-05

### Added
- `ctdev configure linux-mint` command for configuring Linux Mint (Cinnamon) system defaults
  - Power: performance profile, display/inactive sleep timers, lock on suspend
  - Screensaver: idle delay, lock settings
  - Keyboard: repeat rate, repeat delay, numlock state
  - Mouse: flat acceleration, speed, natural scroll
  - Sound: disable event sounds
  - Nemo: list view default
- `ctdev configure linux-mint --show` to display current settings
- `ctdev configure linux-mint --reset` to reset to Cinnamon defaults

## [7.2.0] - 2026-02-04

### Added
- Per-component `uninstall.sh` scripts for all 34 components
- `ctdev list` now shows unsupported components with "not supported" status
- `ctdev uninstall` summary now shows skipped (unsupported) components separately

### Changed
- `ctdev uninstall` refactored to dispatch to component scripts (464 → 140 lines)
- `ctdev upgrade` no longer auto-upgrades bun (no reliable update detection)
- `ctdev upgrade` now shows reminder: "To upgrade bun: bun upgrade"
- Uninstall scripts now use `apt remove` on Linux instead of just logging
- Updated README and CLAUDE.md with correct component count (34)

### Removed
- Monolithic uninstall functions from `cmds/uninstall.sh`

## [7.1.1] - 2026-02-04

### Fixed
- `ctdev configure git` no longer fails with "unbound variable" when called without arguments
- `ctdev info` now shows accurate disk usage on macOS APFS volumes (uses diskutil instead of df)

## [7.1.0] - 2026-02-03

### Added
- `ctdev configure git --show` to display current git user configuration (global and local)
- `ctdev configure macos --show` to display current macOS defaults (Dock, Finder, Keyboard, Dialogs, Security)

## [7.0.1] - 2026-02-03

### Fixed
- Git component now copies `.gitconfig` instead of symlinking (prevents global config changes from modifying repo)

## [7.0.0] - 2026-02-02

### Added
- `ctdev configure` command for post-install configuration
  - `ctdev configure git` - Configure git user name/email (global by default)
  - `ctdev configure git --local` - Configure git per-repository
  - `ctdev configure macos` - Apply macOS system defaults
- `bleachbit` component for Linux system cleaning (counterpart to `cleanmymac` on macOS)
- Hardware info section in `ctdev info`:
  - CPU model with core/thread count
  - Memory with snap-to-standard sizes (8/16/32/64/128 GB)
  - GPU with VRAM via nvidia-smi
  - Disk usage with GB/TB formatting and percentages
- Source guards to prevent multiple sourcing of bash library files
- Linux Mint support for TradingView installer

### Changed
- `ctdev list` now hides components not supported on current OS
- `ctdev upgrade` output cleaned up to only show packages with actual updates
- `ctdev upgrade` now shows version info: `package: 1.0.0 → 1.1.0`
- Exit code 2 now indicates skipped/unsupported installs (0=success, 1=error, 2=skipped)
- Consistent warning messages across all components for unsupported platforms
- Simplified claude-code detection (just checks for `claude` command)

### Removed
- `ctdev macos` command (replaced by `ctdev configure macos`)
- `.gitconfig.local.template` from git component (use `ctdev configure git --local` instead)
- `.gitignore` symlink from git component

## [6.1.0] - 2026-02-02

### Added
- Bun JavaScript runtime installer (macOS via Homebrew, Linux via official installer)

## [6.0.1] - 2026-01-31

### Fixed
- `devcontainer.sh` now sets `FORCE=true` to ensure full installation in containers with pre-installed zsh

## [6.0.0] - 2026-01-30

### Added
- `ctdev uninstall` with no args uninstalls all installed components (with confirmation)
- `--force` flag for `ctdev install` to reinstall components even if already installed
- Root `uninstall.sh` script for removing ctdev itself
- `uninstall_claude` function for claude component
- `list_installed_components` helper in lib/components.sh

### Changed
- All component install scripts now support FORCE environment variable
- All CLI tool scripts now support FORCE environment variable
- Simplified CLAUDE.md (205 → 89 lines)
- Simplified TROUBLESHOOTING.md (343 → 83 lines)
- Removed redundant comments across component scripts

## [5.11.1] - 2026-01-27

### Fixed
- `ctdev info` now checks `claude` CLI in CLI Tools section (moved from Node.js npm check)
- `ctdev info` now checks `path.zsh` in correct location (`~/.zsh/` instead of `~/.oh-my-zsh/custom/`)
- Added Claude component health check for config symlinks

### Changed
- Updated project CLAUDE.md with `claude` and `macos` components
- Added Claude Code troubleshooting section to TROUBLESHOOTING.md

## [5.11.0] - 2026-01-27

### Added
- New `claude` component for syncing Claude Code configuration across machines
- Symlinks `~/.claude/CLAUDE.md`, `settings.json`, and `settings.local.json` from dotfiles repo

## [5.10.2] - 2026-01-27

### Fixed
- Consolidated all PATH setup in `path.zsh` to fix "nodenv: command not found" error
- nodenv/rbenv bin directories are now added to PATH before running their init scripts

## [5.10.1] - 2026-01-26

### Fixed
- PATH ordering: source path.zsh last so `~/.local/bin` takes precedence over nodenv/rbenv shims

## [5.10.0] - 2026-01-26

### Added
- Claude Code CLI installation via native installer (`curl -fsSL https://claude.ai/install.sh | bash`)

### Removed
- Deprecated npm-based Claude Code installation from node component

## [5.9.1] - 2026-01-18

### Removed
- Linux Mint version upgrade detection from `ctdev update`

## [5.9.0] - 2026-01-16

### Added
- Linux Mint version upgrade detection in `ctdev update` (checks if mintupgrade is available)

### Changed
- Ghostty config: set default window size to 180x80, removed copy/paste keybinds (use defaults)

## [5.8.0] - 2026-01-12

### Added
- One-liner install script: `curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/install.sh | bash`

## [5.7.1] - 2026-01-12

### Fixed
- `ctdev gpu status` now correctly detects loaded NVIDIA driver (fixed SIGPIPE issue with pipefail)
- `ctdev gpu status` now correctly detects enrolled MOK keys (fixed fingerprint matching)

## [5.7.0] - 2026-01-12

### Added
- Ghostty terminal emulator installer (cross-platform)
- Ghostty configuration with `ctrl+c`/`ctrl+v` copy/paste keybindings
- `--force` flag to bypass already-installed checks
- Ghostty health check in `ctdev info`

### Changed
- Replaced iTerm2 with Ghostty as the default terminal app
- Updated Nerd Fonts instructions for Ghostty configuration

### Fixed
- Use official ghostty-ubuntu install script for Ubuntu/Debian
- Detect Ubuntu base codename for Linux Mint in Ghostty install

## [5.6.0] - 2026-01-03

### Added
- `ctdev update` command for updating system packages and installed components
- Update detection in `ctdev install` - checks git repos for available updates
- Interactive prompt when updates are available during install

### Changed
- `ctdev install` now only installs new components (no longer auto-updates)
- When already-installed components have updates, user is prompted to update now or defer
- `--skip-system` flag moved to `ctdev update` (deprecated in install)

## [5.5.5] - 2025-12-28

### Fixed
- Silence ShellCheck SC2140 false positive for git URL rewrite syntax in `lib/github.sh`
- Updated CLAUDE.md to reflect current `lib/` directory structure (7 modular files)
- Removed hardcoded version from CLAUDE.md overview

### Changed
- Simplified README.md DevContainers section

## [5.5.4] - 2025-12-28

### Fixed
- `ctdev info` now filters all `/boot*` mounts on Linux (was only filtering `/boot/efi`)
- Simplified disk mount labels (changed "Root (/)" to just "/")

## [5.5.3] - 2025-12-27

### Fixed
- `devcontainer.sh` now runs install script directly (bypasses partial install detection)
- Git clone/pull in devcontainers now forces HTTPS to avoid SSH key issues with URL rewrites

## [5.5.2] - 2025-12-27

### Fixed
- `devcontainer.sh` now skips system package updates that require sudo
- `maybe_sudo` gracefully handles containers with "no new privileges" flag
- zsh install skips `chsh` in devcontainers (shell managed by container)
- Clear error message when zsh not installed in devcontainer

### Added
- `is_devcontainer()` helper detects VS Code devcontainers, Codespaces, and custom containers

## [5.5.1] - 2025-12-27

### Fixed
- `ctdev info` now filters out system volumes on macOS (was showing 10+ unnecessary mounts)
- `ctdev info` now filters out snap/loop/docker mounts on Linux
- `ctdev info` now shows memory used/available on macOS (was only showing total)
- `ctdev info` network section now correctly displays active interfaces on macOS

### Added
- `.zshrc` now sources `~/.secrets` for sensitive credentials (keeps secrets out of git)

## [5.5.0] - 2025-12-26

### Changed
- Merged `ctdev update` into `ctdev install` - single command now handles both installation and updates
- Components not installed will be installed; components already installed will be updated
- System package updates (apt/brew/dnf/pacman) run by default with `--skip-system` flag to skip
- All dry-run messages now visible without `--verbose` flag (use `log_info` instead of `log_debug`)
- Standardized `[DRY-RUN]` prefix (with hyphen) across all scripts

### Added
- `--skip-system` flag for `ctdev install` to skip system package updates
- ShellCheck linting job in GitHub Actions workflow

### Removed
- `ctdev update` command (functionality merged into `ctdev install`)
- `cmds/update.sh` file

## [5.4.0] - 2025-12-26

### Added
- Installation marker files (`~/.config/ctdev/<component>.installed`) for reliable component detection
- TROUBLESHOOTING.md documentation for common issues and solutions
- Enhanced hardware info in `ctdev info`:
  - GPU details via nvidia-smi (model, memory, power, temperature, driver, CUDA version)
  - All mounted disks with usage statistics
  - Network interfaces with IP, MAC address, and state
- Checksum verification for CLI tool downloads (git-spice, doctl, helm, sops)
- Input validation for git user configuration
- ShellCheck static analysis tool to CLI component

### Changed
- Refactored `lib/utils.sh` into modular files:
  - `lib/logging.sh` - Color configuration and log functions
  - `lib/platform.sh` - OS/architecture detection, package management
  - `lib/packages.sh` - Dependency management helpers
  - `lib/github.sh` - GitHub API, checksums, git repository functions
- Memory display now uses binary units (GB = 1024³) for accurate reporting
- Hardware info sections now use consistent indented formatting

### Fixed
- Memory calculation now correctly shows binary units (was showing ~64.8 GB for 64 GB RAM)
- Added 30-second timeout on interactive prompts (prevents CI/CD hangs)
- Non-interactive environments skip update prompts gracefully
- All scripts now pass ShellCheck static analysis

## [5.3.2] - 2025-12-23

### Fixed
- git-spice installer now correctly detects git-spice vs Ghostscript (both use `gs` command)

## [5.3.1] - 2025-12-23

### Added
- git-spice CLI tool for stacked branches workflow (macOS via Homebrew, Linux via GitHub releases)

## [5.3.0] - 2025-12-23

### Changed
- Merged `ctdev doctor` into `ctdev info` - single command now shows system info and health checks
- Moved tmux from `apps` to `cli` component (where it belongs as a CLI tool)

### Removed
- `ctdev doctor` command (functionality merged into `ctdev info`)
- Cursor AI editor app (removed from apps component)

### Fixed
- tmux no longer shows redundant "installation complete" message when already installed

## [5.2.0] - 2025-12-23

### Added
- Cursor AI editor installation for macOS (Homebrew) and Linux (AppImage)
- Comprehensive app checks in `ctdev doctor` for all installed apps (Cursor, Claude, 1Password, DBeaver, TradingView, Linear, CleanMyMac, Logi Options+, tmux)
- CLI tool checks for age, sops, terraform, docker in `ctdev doctor`
- Editor version display (code, cursor) in `ctdev info`
- CLI tool version display for age, sops, terraform in `ctdev info`

## [5.1.3] - 2025-12-19

### Fixed
- Logi Options+ app detection path (bundle name is `logioptionsplus.app`)
- TradingView installer now downloads directly from official URLs instead of using Homebrew cask

### Changed
- TradingView installer supports macOS (DMG) and Debian/Ubuntu (deb) with proper installation flows

## [5.1.2] - 2025-12-14

### Added
- `ctdev doctor` now checks apps, fonts, and macOS defaults components
- `log_check_pass` and `log_check_fail` helpers in `lib/utils.sh` for consistent status output

### Changed
- Unified logging across `ctdev info` and `ctdev doctor` using shared check helpers
- Consistent colored checkmark output (green for pass, yellow for fail)

## [5.1.1] - 2025-12-14

### Changed
- `ctdev update` now checks if components are installed before updating
- Shows helpful skip message with install instructions for non-installed components
- Components without update support (git, fonts, apps, macos) show "No update needed" message

### Fixed
- Variable scope bug in `lib/components.sh` that caused incorrect component names in output

## [5.1.0] - 2025-12-14

### Added
- New `macos` component for configuring macOS system defaults (Dock, Finder, keyboard, dialogs, security)
- GitHub CLI extensions update in `ctdev update cli`

## [5.0.6] - 2025-12-12

### Added
- `devcontainer.sh` for VS Code dotfiles integration (supports `dotfiles.installCommand` setting)

## [5.0.5] - 2025-12-11

### Fixed
- Nerd Fonts installation on macOS with Bash 3.2 (replaced `${VAR,,}` with `tr` for lowercase conversion)
- Added detection for manually installed fonts to avoid Homebrew cask conflicts

### Added
- Terminal configuration instructions printed after fonts installation (iTerm2, Terminal.app, VS Code)

## [5.0.4] - 2025-12-11

### Added
- Auto-update check: ctdev now checks if the repo is behind origin before running any command and prompts to pull the latest changes

## [5.0.3] - 2025-12-10

### Added
- DBeaver Community Edition installer with macOS (Homebrew), Debian/Ubuntu (apt), Fedora (dnf), and Arch (pacman) support

## [5.0.2] - 2025-12-10

### Fixed
- Fonts installation failing with SIGPIPE (exit 141) due to find | head pipeline

## [5.0.1] - 2025-12-10

### Fixed
- gh.sh and terraform.sh no longer require lsb_release
- Git component verification in CI uses correct file paths
- DevContainers documentation expanded with examples

### Changed
- GitHub Actions workflow updated for ctdev CLI
- Removed ShellCheck (false positives on zsh config files)

## [5.0.0] - 2025-12-10

### Added
- `ctdev` unified CLI with subcommands (install, update, doctor, list, info, uninstall, setup)
- `ctdev setup` command to symlink ctdev to ~/.local/bin
- `ctdev doctor` command to check installation health
- `ctdev list` command to show available components
- Modular CLI tools installation (individual scripts per tool)
- Git user config with interactive prompts and CLI arguments
- Support for Homebrew-installed nodenv/rbenv (skips git updates)
- Nerd Fonts installation via Homebrew casks on macOS

### Changed
- Reorganized directory structure: components/, cmds/, lib/
- Moved all component installers to components/ directory
- CLI tools now installed individually (age, btop, docker, doctl, gh, helm, jq, kubectl, sops, terraform)
- Node.js uses `brew install nodenv` on macOS, git clone on Linux
- Ruby uses `brew install rbenv` on macOS, git clone on Linux
- Fonts use Homebrew casks on macOS, GitHub releases on Linux
- PATH configuration prioritizes ~/.local/bin
- Symlink resolution in ctdev supports being called via symlink

### Removed
- Old top-level install.sh, update.sh, report.sh, devcontainer.sh
- Old directory structure (apps/, cli/, fonts/, git/, node/, ruby/, zsh/ at root)
- scripts/utils.sh (moved to lib/utils.sh)

## [4.0.0] - 2025-12-10

### Added
- Initial ctdev CLI structure
- Component-based architecture

## [3.0.0] - 2024

### Added
- Cross-platform support (Ubuntu, Debian, Fedora, Arch, macOS, FreeBSD)
- Structured logging functions
- Dry-run mode
- maybe_sudo for Docker compatibility

## [2.0.0] - 2024

### Added
- Modular component installers
- Shared utilities (scripts/utils.sh)

## [1.0.0] - 2024

### Added
- Initial dotfiles implementation
