# Linear app credentials are per computer

Claude Code reaches Linear as an app, not as Thomas, so the app's comments and status changes
notify him the way a teammate's would. Each computer has its own Linear app, and that app's client
ID and secret live only on that computer, in `~/.config/ctdev/linear/<workspace>.env`. They are
entered with `ctdev configure linear` and never committed, not even encrypted. This replaces the
earlier plan (CON-102, CTD-60) to keep one app's secret in a per-repo `.env.sops`.

A secret shared by every computer means one app shared by every computer. Rotating that secret
would revoke every computer's tokens at once, so a leak on one machine would break all of them. One
app per computer contains a rotation or a revoked app to the machine it belongs to, and Linear's
history shows which computer acted. It also keeps to the rule in `AGENTS.md` that every secret is
entered at configure time and stored only on the host that needs it.

## Consequences

Setting up a new computer means creating its Linear app and running `ctdev configure linear` once
per workspace. Retiring a computer means deleting its app in Linear. A secret is rotated in Linear,
and then `ctdev configure linear` takes the new one. ctdev has no rotate flow of its own.

Each repo names its workspace in its own `.mcp.json`
(`ctdev linear mcp-headers --workspace <name>`). Nothing goes in `~/.claude.json` or other global
Claude Code config. A repo that isn't Conner Technology's keeps its `.mcp.json` out of git through
`.git/info/exclude`, so the same computer can reach a second Linear workspace without touching that
repo's history.
