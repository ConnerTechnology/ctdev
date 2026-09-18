---
paths:
  - "ctdev/component/mcp_email_server*.go"
  - "ctdev/component/configs/mcp-email-server/**"
  - "ctdev/cmd/configure_mcp_email_server*.go"
---
# mcp-email-server — verified facts

Two non-obvious facts, both verified against the running container rather than
inferred — change either and tailnet requests break in a way that looks like a
network fault:

- `tailscale serve` forwards the client's **original Host header**
  (`r.Out.Host = r.In.Host` in `ipn/ipnlocal/serve.go`), and MCP's DNS-rebinding
  protection answers **421** for any Host it wasn't told about. So the node's
  MagicDNS name must be in `MCP_ALLOWED_HOSTS`, which
  `ctdev configure mcp-email-server` writes into `~/mcp-email-server/.env`.
  Unconfigured, the compose defaults are loopback-only — it fails closed.
- **The stack is built, not pulled.** `ghcr.io/ai-zerolab/mcp-email-server` stops
  at 0.16.0 (44 tags, single page, no 1.x; `manifests/1.4.1` → 404) while PyPI is
  on 1.x. So the Dockerfile installs the pinned package on `python:3.12-slim`,
  the same locally-built shape caddy uses (`build: .` + `image: <name>:local`).
  1.x is also what makes headless setup first-class: `account add
  --password-stdin` (upstream's own "never place credentials in argv"), `--json`
  on every command, and `account test` for a real IMAP login check.
- **The container runs as the invoking user, not root.** 1.x refuses to open its
  catalog unless the parent directory is owner-only *from the running user's*
  point of view — a root-run container against a user-owned `./config` fails with
  "Managed catalog parent must be owner-only". `ctdev install` writes `MCP_UID`/
  `MCP_GID` into the stack's `.env` before the first `compose up`, and the
  compose file uses the `${MCP_UID:?}` form so a missing value fails loudly
  instead of silently reverting to root. `.env` is therefore written by *both*
  install (uid/gid) and configure (tailnet settings) — `MCPEmailServerSetEnv`
  merges rather than truncates.
- **Install must not silently replace another email server.** It did once:
  `list_available_accounts` came back `[]` with `isError:false`, which reads as
  "no mailboxes", not "your accounts were orphaned". `mcpEmailServerConflicts`
  now gates the install on three probes — a container whose `.Config.Image`
  isn't `mcp-email-server:local` (use `.Config.Image`, not `docker ps`, which
  reports a bare hash once a local rebuild moves the tag), a non-empty
  `./config` without `managed.sqlite3`, and a `tailscale serve` handler on the
  serve port pointing somewhere other than our loopback port. The rules are a
  pure function over a `mcpEmailServerState` struct so they are testable without
  docker or a tailnet. `--force` replaces the container (never its volumes) but
  still keeps `./config`. Install also prints the account count every run, so an
  empty catalog is stated rather than inferred.
- **`account remove` needs `--expected-revision` and `--confirm`.** Upstream
  guards removal with optimistic concurrency; `account remove <name>` alone
  fails, and failed silently here until it was run against a real server.
  `MCPEmailServerRemoveAccount` reads the revision back from `account list`
  first. `account add` rejects a duplicate name outright, so replacing an
  account is remove-then-add.
- **Secrets are not encrypted at rest.** The managed catalog stores passwords in
  cleartext inside `managed.sqlite3` (verified with `strings`); 0600 and the
  node's isolation are the whole protection. Don't describe it as encrypted.

Credentials live in a **bind mount** (`./config`), not a named volume, so they
survive `docker compose down` *and* the host can verify the mode without
entering the container. `ctdev uninstall` stops the stack and keeps the catalog —
losing it means re-issuing an app password per mailbox.
