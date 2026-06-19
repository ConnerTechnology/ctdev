# Caddy built with the Cloudflare DNS module so it can solve ACME DNS-01
# challenges for a wildcard cert without exposing any inbound ports.
FROM caddy:2-builder AS builder
RUN xcaddy build --with github.com/caddy-dns/cloudflare

FROM caddy:2
COPY --from=builder /usr/bin/caddy /usr/bin/caddy
