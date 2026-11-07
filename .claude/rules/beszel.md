---
paths:
  - "ctdev/component/beszel*.go"
  - "ctdev/component/configs/beszel/**"
---
# Beszel

- Keep it off any public network (Tailscale only).
- **Per-container CPU, memory *and* network stats all need the kernel memory cgroup.** The
  agent aborts its whole container-stats block when memory reads fail (henrygd/beszel#144), so
  all three columns read a flat zero, which looks like idle containers rather than broken
  instrumentation. Raspberry Pi OS prepends `cgroup_disable=memory` to the device-tree bootargs;
  `ctdev doctor`'s `sys.memcgroup` check (`ctdev/diagnose/linux.go`) detects it and gives the
  fix. Without it, `mem_limit` in any compose stack is silently ignored too, so a leaking
  container has no ceiling.
