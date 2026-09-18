---
paths:
  - "ctdev/component/portainer.go"
  - "ctdev/component/configs/portainer/**"
---
# Portainer

- `ctdev install portainer` — Portainer CE Docker management web UI as a
  container deployed to `~/portainer/` (users/settings persist in the
  portainer_data volume). The Caddy stack reverse-proxies it at
  `https://portainer.<domain>` (port 9000); it is also reachable directly at
  `https://<node>:9443`. No `configure` step — create the admin user on first
  web login. The Docker socket is mounted, so keep it off any public network.
