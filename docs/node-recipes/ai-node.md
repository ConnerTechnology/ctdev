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
ctdev install brain                      # the agent org + its scheduled runs
ctdev configure brain                    # checkout, schedule, Claude credential
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

## The brain

`ctdev install brain` puts your agent org — knowledge, agents, skills, commands
and the memory they accumulate — on the node, and runs its scheduled work there.
It is a git checkout and two systemd timers, not a container.

The point is that **the node becomes the only machine running agents on a
schedule.** A schedule that lives in a laptop's Claude Code session dies when the
window closes; worse, two laptops both running scheduled agents write to the same
memory files and quietly disagree. One always-on writer removes both problems.

```bash
ctdev install brain          # clone, run the repo's setup.sh, deploy the timers
ctdev configure brain        # checkout path, schedule, Claude credential
ctdev configure brain --show # what is configured, and what is still missing
sudo systemctl start brain-triage.service    # run it now
journalctl -fu brain-triage.service          # watch it
```

**Where it lives.** These paths are deliberately stable and not under anyone's
home directory, because a service that is not the timer — an app, later — has to
find the brain without reverse-engineering a systemd unit:

| Path | What |
| --- | --- |
| `/srv/brain` | the checkout, `brain:brain`, mode 2770 |
| `/var/lib/brain` | service state: run digests, the lock, the deploy key |
| `/etc/ctdev/brain.conf` | the pointer file — paths, remote, schedule. World-readable, holds nothing secret |
| `/etc/ctdev/brain-claude-token.cred` | the Claude token, encrypted to this host |
| `/usr/local/bin/brain-run` | the one entry point both timers call |

**It runs as its own service account.** Not you — `brain`, a system user, so its
commits are attributable to the node rather than to a person, and so a second
service account can join group `brain` and read the checkout later without being
the timer's user. The checkout is not world-readable: it holds household
financial detail.

**Two timers.** `brain-triage.timer` at **07:03 and 15:07** local, and
`brain-sync.timer` hourly at `:23` to keep the checkout current for whoever SSHes
in — which is how a phone reaches the brain, since a tailnet-only service is
unreachable from the Claude app but perfectly reachable from Claude Code over
SSH. Both are `Persistent=true`, so a run missed while the node was down happens
at boot. Change either with `ctdev configure brain`; it writes a drop-in and
leaves the shipped unit as the recorded default.

The two are enabled independently. Sync starts as soon as the checkout exists,
because it needs nothing but git; triage waits until a Claude credential is
stored, because enabling it earlier would buy two failed units a day. So a node
with a deploy key but no token yet still keeps its checkout current.

**The scheduled prompt points, it never restates.** It names the agent, the
skill and the memory file to read, and nothing else — because a prompt with the
rules copied into it is a snapshot that goes stale silently, and on an unattended
node that drift goes unnoticed for weeks. Put a `scheduled/triage.md` in the repo
and it wins over the shipped default, so the prompt can live with the rules it
points at.

**Headless authentication.** Claude Code's normal login is a browser flow, which
a timer cannot do. Run `claude setup-token` once on a machine that has a browser
— it mints a one-year token against your Pro/Max/Team/Enterprise subscription —
keep the master copy in 1Password, and paste it into `ctdev configure brain`.
It is encrypted to this host with `systemd-creds` and decrypted by systemd into
a private in-memory directory that only the triage unit can read. **No plaintext
copy is written anywhere**, the blob is inert on any other machine (including
inside a restic snapshot of `/etc`), and nothing has to reach a vault over the
network at 07:03 — which matters on a node that also serves the household's DNS.
Record the `op://` reference with `--token-ref` so `--show` can say where the
master copy lives; the reference is not a secret.

One thing that token cannot do: **fetch claude.ai connectors.** Gmail, Calendar,
Drive and Notion are unavailable to scheduled runs. Locally-configured MCP
servers work normally, which is what the tailnet mail server is.

**Pushing.** The node authenticates to git with a **repository deploy key** it
generates itself, so no credential is ever transported to it. `install` prints
the public half and the URL to paste it at; it needs **write access**, because
the node pushes what its runs learn. Revoking it is one click and touches no
human's account.

**One writer, and failures are loud.** Every run rebases onto `origin`
immediately before it works and pushes immediately after, so the window where the
node holds unpushed commits is seconds. A `flock` serializes the two timers
against each other and against a hand-run `brain-run`. Nothing is ever
force-pushed and no conflict is ever auto-resolved: a rejected push is retried
once after a rebase, and a genuine conflict aborts and **fails the unit**, which
is how it shows up in `systemctl --failed` and `ctdev status` instead of
accumulating in silence.

**What the scheduled session can reach** is narrower than a normal session. The
built-in tool set is cut to delegation and file access — **no shell, no web
fetch, no web search** — because `brain-run` does the git work itself, so the
session never needs one. MCP is an allow-list built from what the repo's own
setup registered, not a deny-list: add a server next year and it is *not*
automatically in reach of a session that reads attacker-controlled email.

`ctdev uninstall brain` stops the timers and removes the units and the runner.
It keeps the checkout, `memory/`, the state directory, the config and the
credential — `memory/` is accumulated learning that exists nowhere else.
