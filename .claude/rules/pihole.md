---
paths:
  - "ctdev/component/pihole*.go"
  - "ctdev/component/configs/pihole/**"
  - "ctdev/cmd/pihole*.go"
---
# Pi-hole

- `ctdev install pihole` — Pi-hole as a Docker container (official image, host
  networking) deployed to `~/pihole/`; config/lists persist in `./etc-pihole`.
- `ctdev configure pihole` — upstream resolvers, listening mode, blocking on/off
  (runs against the container via `docker exec`, or a native install if present).
  Its **host resolver** setting exists because a node that accepts Tailscale DNS
  resolves through MagicDNS, whose global nameserver is this same Pi-hole: with
  FTL down the host cannot pull the image, reach B2, or run the brain. It runs
  `tailscale set --accept-dns=false`, drops `dns=none` for NetworkManager, writes a
  static `/etc/resolv.conf` (127.0.0.1, then 9.9.9.9 — a stopped FTL *refuses*, so
  the fallback is instant) and a `03-tailnet.conf` dnsmasq drop-in forwarding the
  MagicDNS suffix to `100.100.100.100`. The forward is load-bearing: the brain dials
  the mail server by its `.ts.net` name, which Unbound cannot resolve (NXDOMAIN,
  verified). tailscaled keeps answering on 100.100.100.100 with `--accept-dns=false`;
  that flag only governs who writes resolv.conf. Gated to NetworkManager hosts
  without systemd-resolved; never batch-applied by a profile (`pihole` defaults
  would also flip the upstream to Cloudflare).
  The upstream choices include "Local recursive (Unbound)" → `127.0.0.1#5335`,
  served by the `unbound` sidecar in the Pi-hole stack (recursive + DNSSEC).
  Pi-hole's lists, settings, and gravity.db persist in `~/pihole/etc-pihole`, which
  restic backs up. A restic snapshot restores a Pi-hole but doesn't show what its
  lists contain, so the allow/deny/regex lists and adlists are *also* version
  controlled as `component/configs/pihole/lists.toml` (one entry per line, with
  its comment and group): `ctdev pihole sync` diffs that file against gravity.db
  and applies what you check, `ctdev pihole export` records the live state back
  into it. Additions and updates are checked by default; **removals are not** —
  an unchecked removal keeps the entry on the Pi-hole, which is what makes a
  domain someone allowed in the web UI survive a sync. Writes are one SQL
  transaction against gravity.db (the `pihole allow --delete` CLI is broken on
  v6.4.3), then `pihole reloadlists`, or the much slower `pihole -g` only when an
  adlist changed. Keep semicolons out of entry comments: the `pihole` CLI rejected
  them (2026-09-08), and nobody has tried one through `sync`. Set the admin password with
  `docker exec -it pihole pihole setpassword`.
