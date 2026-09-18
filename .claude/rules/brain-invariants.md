---
paths:
  - "ctdev/component/brain*.go"
  - "ctdev/component/configs/brain/**"
  - "ctdev/cmd/configure_brain.go"
---
# The brain — load-bearing facts

Facts that are load-bearing; change any of them and something breaks quietly:

- **`systemd-creds`, not `op run`, holds the Claude token.** Unattended `op` needs
  `OP_SERVICE_ACCOUNT_TOKEN`, itself a long-lived secret that would have to sit on
  the node in plaintext to bootstrap the thing meant to keep plaintext off it —
  plus a network round-trip at 07:03 on the node that serves the household's DNS.
  1Password stays the system of record (`BRAIN_TOKEN_REF` records the `op://` URI,
  which is not secret); what lands here is encrypted to
  `/var/lib/systemd/credential.secret`. That file is **not** in the restic backup
  set, which is what makes the `.cred` inert inside a snapshot of `/etc`.
- **The token comes from `claude setup-token`** (one year, needs Pro/Max/Team/
  Enterprise). It **cannot fetch claude.ai connectors** — Gmail, Calendar, Drive,
  Notion are unavailable to scheduled runs. Locally-configured MCP servers work,
  which is what the tailnet mail server is. The `inbox` agent only uses
  `mcp__email__*`, so triage is unaffected.
- **Git auth is a repo deploy key generated on the node.** The only credential in
  the design with no transport problem. It needs *write* access; without it the
  node commits and strands.
- **Nothing is ever force-pushed and no conflict is auto-resolved.** A rejected
  push rebases and retries once; a real conflict aborts and **fails the unit**, so
  it surfaces in `systemctl --failed` / `ctdev status`. `brain-run` holds a
  `flock` so the two timers and a hand run cannot interleave. `brain_test.go`
  asserts the absence of `push --force`, `reset --hard`, `checkout --theirs`.
- **The prompt points, it never restates.** A prompt with the rules copied in is a
  snapshot that goes stale silently — it already happened on 2026-08-20. The test
  caps the shipped prompt's length for exactly this reason.
- **`brain-sync.service` deliberately loads no credential.** Git only, so the
  checkout keeps tracking origin after the token expires and a laptop's `git pull`
  still receives what the last triage committed. `BrainEnableTimers` gates the two
  separately for the same reason: sync starts as soon as a checkout exists, triage
  waits for the credential. The half that works is never withheld by the half that
  does not, and it is the half a phone depends on.
- **MCP is an allow-list, not a deny-list.** `brain-run` filters the servers the
  repo's setup registered down to `BRAIN_CLAUDE_MCP` (default `email`) and passes
  `--strict-mcp-config`. A deny-list would fail open when a server is added later.
  `--tools` separately removes Bash, WebFetch and WebSearch from the session —
  verified against a live session, not assumed.
- **The workspace is marked trusted in the service account's `~/.claude.json`.**
  Claude Code ignores a project's `.claude/settings.json` until a trust dialog is
  accepted, and a timer cannot answer a dialog. Without it the node silently runs
  with different settings from every laptop.
- **`Root: RootAlways`.** Every run redeploys units, writes `/etc`, and works as
  the service account. `DetectPath` is the runner, not the checkout, so a repo
  cloned by hand does not read as an installed component.
- **Existence checks go through `brainPathExists`.** `/srv/brain` (2770) and
  `/var/lib/brain` (0750) are not traversable by the operator's own account, so a
  bare `os.Stat` reports "missing" for files that are plainly there.

Uninstall stops the timers and removes the units and runner. It **keeps** the
checkout, `memory/`, the state directory, `brain.conf`, the credential, the
schedule drop-ins and the account.
