# AI / MCP Node

An always-on box that holds the credentials so your laptops don't. Each MCP
server runs as a container reachable **only over Tailscale** — nothing is
published to the LAN or the internet.

```bash
ctdev apply ai-node                      # ← the whole block below in one command
# — or step by step: —
ctdev install zsh git tailscale docker
sudo tailscale up                        # join the tailnet
ctdev install mcp-email-server           # MCP server that reads your mailboxes over IMAP
ctdev configure mcp-email-server         # add mailboxes, publish it to the tailnet
ctdev install restic && ctdev configure restic   # back the mailbox config up
```

`ctdev install mcp-email-server` brings up
[mcp-email-server](https://github.com/ai-zerolab/mcp-email-server) — an MCP
server that reads (and optionally sends) mail over IMAP, so a laptop running
Claude Code reaches your mail through the tailnet instead of storing an email
password. `ctdev configure mcp-email-server` walks through adding each mailbox
over SSH (upstream's `ui` subcommand needs a browser), writes them to
`~/mcp-email-server/config/config.toml` on the node only, and puts
`tailscale serve` in front of the service. Point a laptop at it with:

```bash
claude mcp add --transport http email https://<node>.<tailnet>.ts.net/mcp
```

On a node that already runs Caddy — like a `pihole-node` — `configure` publishes
on tailnet port **8443** instead of 443, and says so. 443 is not free there:
`ctdev configure caddy` points `*.<domain>` at that node's *Tailscale* IP, and a
serve rule on 443 would intercept it, so every `https://<svc>.<domain>` on the
tailnet would start hitting the email server instead of Caddy. Override with
`--serve-port`. The endpoint is then
`https://<node>.<tailnet>.ts.net:8443/mcp`.

**The upstream server has no authentication of its own** — anything that can
reach its port reads every configured mailbox in full. Two things keep that
safe, and both matter: the container port is published on `127.0.0.1` only
(never `0.0.0.0` — Docker's iptables rules sit ahead of UFW, so a bare port
mapping is LAN-wide even on a firewalled host), and `tailscale serve` is the
only route in, which means TLS plus tailnet authentication. Use `serve`, never
`funnel`.

Mailbox passwords live in the managed catalog at
`~/mcp-email-server/config/managed.sqlite3`, mode 0600, owned by you — the
container runs as your uid, not root. They are **not encrypted at rest**; the
file mode and the fact that the directory never leaves one tailnet-only host are
what protect them. Use app-specific passwords, which you can revoke without
touching the account. Install and `configure` both print the catalog's path and
its actual mode, and adding an account runs a real IMAP login test so a wrong
password fails immediately rather than as an empty inbox days later.

`ctdev uninstall mcp-email-server` stops the container but keeps the catalog;
deleting it means re-issuing an app password for every mailbox. Include
`~/mcp-email-server` in `ctdev backup paths` so it lands in an encrypted restic
snapshot.

`ctdev install` refuses to run if something it didn't create is already serving
this role — a container built from another image, a `./config` belonging to a
different email server, or a `tailscale serve` mapping already on the port. It
names what it found and stops, because replacing such a server orphans its
mailbox accounts rather than migrating them, and the replacement then reports
"no accounts configured" instead of an error. Pass `--force` to replace it
anyway; `./config` is never deleted either way. Re-running install over its own
stack is idempotent and leaves the catalog untouched.

The stack is **built from the Dockerfile**, not pulled: upstream's published
container images stop at `0.16.0` while the Python package is on 1.x, so the
image line is a major version behind. The Dockerfile pins
`mcp-email-server==1.4.1` on `python:3.12-slim`. Bump that pin deliberately
after reading the upstream release notes, then re-run
`ctdev install mcp-email-server` (compose rebuilds).
