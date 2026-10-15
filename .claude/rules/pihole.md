---
paths:
  - "ctdev/component/pihole*.go"
  - "ctdev/component/configs/pihole/**"
---
# Pi-hole

- `ctdev install pihole` — Pi-hole as a Docker container (official image, host
  networking) deployed to `~/pihole/`; config/lists persist in `./etc-pihole`.
- `ctdev configure pihole` — upstream resolvers, listening mode, blocking on/off
  (runs against the container via `docker exec`, or a native install if present).
  Its **host resolver** setting exists because a node that accepts Tailscale DNS
  resolves through MagicDNS, whose global nameserver is this same Pi-hole: with
  FTL down the host cannot pull the image or reach B2. It runs
  `tailscale set --accept-dns=false`, drops `dns=none` for NetworkManager, writes a
  static `/etc/resolv.conf` (127.0.0.1, then 9.9.9.9 — a stopped FTL *refuses*, so
  the fallback is instant) and a `03-tailnet.conf` dnsmasq drop-in forwarding the
  MagicDNS suffix to `100.100.100.100`. Without the forward, `.ts.net` names stop
  resolving on the host once MagicDNS no longer writes resolv.conf, because Unbound
  cannot resolve them (NXDOMAIN, verified); it also gives every LAN client tailnet
  names. tailscaled keeps answering on 100.100.100.100 with `--accept-dns=false`;
  that flag only governs who writes resolv.conf. Gated to NetworkManager hosts
  without systemd-resolved; never batch-applied by a profile (`pihole` defaults
  would also flip the upstream to Cloudflare).
  The upstream choices include "Local recursive (Unbound)" → `127.0.0.1#5335`,
  served by the `unbound` sidecar in the Pi-hole stack (recursive + DNSSEC).
  Pi-hole's lists, settings, and gravity.db persist in `~/pihole/etc-pihole`, which
  restic backs up when `~/pihole` is in `ctdev backup paths`. Set the admin password with
  `docker exec -it pihole pihole setpassword`.
