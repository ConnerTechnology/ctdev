---
paths:
  - "ctdev/component/pihole*.go"
  - "ctdev/component/caddy.go"
  - "ctdev/component/portainer.go"
  - "ctdev/component/beszel*.go"
  - "ctdev/component/restic*.go"
  - "ctdev/component/mcp_email_server*.go"
  - "ctdev/component/compose_stack.go"
  - "ctdev/component/configs/**"
  - "ctdev/cmd/update_docker*.go"
---
# Homelab / Pi-hole nodes

ctdev has no "homelab mode" — you compose a node from individual components and
`configure` categories, the same way you would a desktop. For a Raspberry Pi
running Pi-hole behind a Caddy reverse proxy:

The per-component facts are in `.claude/rules/pihole.md`, `caddy.md`,
`portainer.md`, `beszel.md` and `restic.md`.

Keeping a node current: `ctdev update` refreshes these compose stacks along with
system packages. It checks each managed stack (pihole, caddy, beszel, portainer,
mcp-email-server) for a newer image by digest — without pulling — and updates the ones you select
(`docker compose pull && up -d`, or a `build --pull` rebuild for the locally-built
caddy image). `ctdev update --check` lists what's available read-only.

Note: a Pi-hole/DNS host should usually **not** run `ctdev configure ufw` — UFW's
default-deny blocks DNS (53) and the proxy (80/443) unless you open them first.

Secrets are **never stored in the repo**. Each is entered at `configure` time and
stored only on the host that needs it; if lost, just reconfigure:
- **restic** (repo + credentials + password) → `/etc/restic/restic.env`, written by
  `ctdev configure restic`. restic itself can't restore its own credentials, so they
  live only here — keep a copy in your password manager.
- **caddy** (domain/ACME email/CF token) → `~/caddy/.env`, written by `ctdev configure caddy`.
- **pihole** (admin password) → set with `docker exec -it pihole pihole setpassword`.
- **beszel** (agent KEY/TOKEN) → `~/beszel/.env`, pasted from the hub's "Add System" dialog.
- **mcp-email-server** (one app-specific password per mailbox) →
  `~/mcp-email-server/config/config.toml` (0600, cleartext — a headless node has no
  keyring), written by the container during `ctdev configure mcp-email-server`.

restic snapshots the rendered `~/<svc>/.env` files (they're under the backed-up paths),
so a restore brings them back; a brand-new node re-enters them from your password manager.
**Never commit a secret.** See `RECOVERY.md` (disaster recovery).
