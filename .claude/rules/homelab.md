---
paths:
  - "ctdev/component/pihole*.go"
  - "ctdev/component/caddy.go"
  - "ctdev/component/portainer.go"
  - "ctdev/component/beszel*.go"
  - "ctdev/component/compose_stack.go"
  - "ctdev/component/configs/**"
  - "ctdev/cmd/update_docker*.go"
---
# Homelab / Pi-hole nodes

ctdev has no "homelab mode" — you compose a node from individual components and
`configure` categories, the same way you would a desktop. The recipe for a Raspberry Pi
running Pi-hole behind a Caddy reverse proxy is `docs/node-recipes/pihole-node.md`; the
per-component traps are in `pihole.md` and `beszel.md`.

Secrets are **never stored in the repo**. Each is entered at `configure` time and
stored only on the host that needs it; if lost, just reconfigure:
- **caddy** (domain/ACME email/CF token) → `~/caddy/.env`, written by `ctdev configure caddy`.
- **pihole** (admin password) → set with `docker exec -it pihole pihole setpassword`.
- **beszel** (agent KEY/TOKEN) → `~/beszel/.env`, pasted from the hub's "Add System" dialog.

A brand-new node re-enters them from your password manager. **Never commit a secret.**
