---
paths:
  - "ctdev/component/mcp_email_server*.go"
  - "ctdev/component/configs/mcp-email-server/**"
  - "ctdev/cmd/configure_mcp_email_server*.go"
---
# AI / MCP nodes

An always-on node that holds credentials so laptops don't have to. Same shape as
the homelab node — compose it from components; `ctdev apply ai-node` is the
built-in profile.

- `ctdev install mcp-email-server` — [mcp-email-server](https://github.com/ai-zerolab/mcp-email-server)
  as a Docker compose stack in `~/mcp-email-server/`, exposing IMAP mailboxes to
  MCP clients over streamable-HTTP.
- `ctdev configure mcp-email-server` — adds/removes mailboxes and publishes the
  service to the tailnet. Also `--show`.

**Upstream ships no authentication.** `mcp-email-server streamable-http` takes
only `--host` and `--port`; whatever reaches the port reads every mailbox. Three
invariants carry the whole security model, and all three are load-bearing:

1. **The published port is `127.0.0.1:9557` only.** Never `0.0.0.0`, never a bare
   mapping — Docker's iptables rules are inserted ahead of UFW, so a bare mapping
   is LAN-wide even on a firewalled host. `TestMCPEmailServerPublishesOnLoopbackOnly`
   asserts this against the embedded compose file.
2. **`tailscale serve` on the host is the security boundary** — TLS from the
   tailnet's own cert, reachable only by authenticated peers. On the host rather
   than in a sidecar: the node is already on the tailnet, so a sidecar would add
   a second node identity plus an auth key to store, and its serve config would
   die with the container. `serve`, never `funnel`.
3. **The version is pinned.** `latest` on the process holding mail credentials
   is a silent-upgrade risk. Bump `mcp-email-server==<x.y.z>` in
   `component/configs/mcp-email-server/Dockerfile` deliberately.

**Do not serve on 443 next to Caddy.** `ctdev configure caddy` points
`*.<domain>` at the node's *Tailscale* IP, and Caddy answers there on 443. A
`tailscale serve --https=443` rule intercepts that port for the node's own
tailnet addresses, so every homelab site would silently start hitting the email
server. `MCPEmailServerServePort` therefore returns 8443 when the caddy stack is
present, 443 otherwise, and an explicit `MCP_SERVE_PORT` in the stack's `.env`
(written by `--serve-port`) wins over both. It probes for the caddy compose file
by path rather than through `FindByName`, because the registry references this
component's uninstaller — reading `Registry` from there is an initialization
cycle. A bare hostname in `MCP_ALLOWED_HOSTS` covers every port (upstream
expands it to `<name>` and `<name>:*`), so a non-default port needs no extra
allowlist entry.

The rest is in `.claude/rules/mcp-email-server-facts.md`.
