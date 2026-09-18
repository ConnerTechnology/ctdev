---
paths:
  - "ctdev/component/caddy.go"
  - "ctdev/component/configs/caddy/**"
  - "ctdev/cmd/configure_caddy.go"
---
# Caddy

- `ctdev install caddy` — deploys the Caddy stack from `component/configs/caddy/`
  to `~/caddy/` and runs `docker compose up`. The stack is generic; the domain,
  ACME email, and Cloudflare token come from `~/caddy/.env`.
- `ctdev configure caddy` — writes `~/caddy/.env` (domain / ACME email / CF token)
  and, when Pi-hole is present, wires it (frees port 443, points `*.<domain>` at
  this node's Tailscale IP). Run this before `ctdev install caddy`.
