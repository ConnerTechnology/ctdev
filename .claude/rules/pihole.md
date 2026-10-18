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
  FTL down the host cannot pull the image or reach B2. The mechanics are in
  `ctdev/setup/pihole.go`.
