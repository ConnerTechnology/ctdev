---
paths:
  - "ctdev/component/beszel*.go"
  - "ctdev/component/configs/beszel/**"
---
# Beszel

- `ctdev install beszel` — Beszel server/container monitoring (hub + agent) as
  containers deployed to `~/beszel/`. The hub (web UI + data, port 8090) is
  reverse-proxied by Caddy at `https://beszel.<domain>` via a `@beszel` route;
  the agent monitors this host over a shared unix socket. The install brings the
  hub up first; create the admin user, click "Add System", put the issued
  KEY/TOKEN in `~/beszel/.env`, then re-run `ctdev install beszel` to start the
  agent. Keep it off any public network (Tailscale only). **Per-container CPU,
  memory *and* network stats all need the kernel memory cgroup** — the agent
  aborts its whole container-stats block when memory reads fail
  (henrygd/beszel#144), so all three columns read a flat zero, which looks like
  idle containers rather than broken instrumentation. Raspberry Pi OS prepends
  `cgroup_disable=memory` to the device-tree bootargs; append
  `cgroup_enable=memory cgroup_memory=1` to `/boot/firmware/cmdline.txt` (keep
  it ONE line — the last value wins) and reboot. Verify with `cat
  /sys/fs/cgroup/cgroup.controllers`, which must list `memory`. `ctdev doctor`
  checks this on any Linux host running Docker. Without it, `mem_limit` in any
  compose stack is silently ignored too, so a leaking container has no ceiling.
