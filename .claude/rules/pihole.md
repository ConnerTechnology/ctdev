---
paths:
  - "ctdev/component/pihole*.go"
  - "ctdev/component/configs/pihole/**"
  - "ctdev/setup/pihole*.go"
  - "ctdev/sysutil/pihole*.go"
---
# Pi-hole

- The **host resolver** setting in `ctdev configure pihole` exists because a node that accepts
  Tailscale DNS resolves through MagicDNS, whose global nameserver is this same Pi-hole: with
  FTL down the host cannot pull the image or reach B2. Taking resolv.conf from Tailscale also
  takes MagicDNS from the host, and Unbound cannot resolve `.ts.net` names (NXDOMAIN, verified),
  so Pi-hole forwards the tailnet suffix to `100.100.100.100`. tailscaled keeps answering there
  with `--accept-dns=false`; that flag only governs who writes resolv.conf. The mechanics are in
  `ctdev/setup/pihole.go`.
