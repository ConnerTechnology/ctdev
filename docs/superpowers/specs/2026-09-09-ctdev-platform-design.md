# ctdev platform — design

Written 2026-09-09 from the interview in
[`../plans/2026-09-09-ctdev-platform-interview-notes.md`](../plans/2026-09-09-ctdev-platform-interview-notes.md),
which holds every decision with Thomas's words. This document is the shape those decisions make.
Numbers in brackets are decision numbers there.

## What this is

ctdev grows from a CLI that manages the machine you are sitting at into Conner Technology's
system for managing every machine it owns or looks after: a dashboard at
`dashboard.connertechnology.io` that shows every managed machine, what it is doing, how it has
been doing over time, and the actions you may take on it, with everything that ctdev can do
today runnable from there. The CLI stays for local work. [10, 19, 20]

The name stays ctdev for the CLI, the agent, the dashboard and the repo. [19]

## Principles

- **One place to look.** Data from every source lands in one database and one dashboard. Thomas
  should not have to sign in to Beszel, Axiom, Pi-hole and Linear to know how a machine is. [11]
- **Do not reinvent the wheel.** An existing tool or API that captures data is a data source we
  read. We write our own only where nothing exists. [11]
- **Local work never depends on the server.** Install, configure, apply, doctor and status on the
  machine you are at work with nothing but the binary, offline included. The Pi that runs DNS
  must be able to repair itself. [8]
- **Every command is plan, choose, apply over structured data.** The terminal and the web are two
  renderers of the same thing, so they cannot drift. [10]
- **A person's credential never reaches a machine; a machine's credential never grants a person
  anything.** [14]

## Components

```
machine ──ctdev agent──▶ WebSocket ──▶ API (Node/Express/TS) ──▶ Postgres (managed)
   │                                      │        ▲
   │  logs ──────────────────────────────▶│  log store (Axiom)
   │                                      │
   └──ctdev CLI (local, unchanged)     worker (schedules, alerts, retention)
                                          │
                              web (React SPA/PWA on Cloudflare Pages)
```

- **Agent**: the existing Go code, with a daemon mode. Holds one outbound WebSocket to the API,
  sends heartbeat, status, inventory, metrics and doctor runs, executes the commands the API
  sends, streams their output, ships logs to the log store. Never listens. Updates itself from
  the CLI's release pipeline. [2, 12, 13, 14]
- **API**: Node, Express, TypeScript. Terminates the agents' WebSockets, dispatches commands,
  relays streamed output to browsers, owns identity and access, serves the web app's data. [12]
- **Worker**: scheduled doctor runs, alert evaluation, metric retention, secret rotation, using
  the async-services shape Foil It Up already uses. [12]
- **Web**: React and TypeScript, a single-page app built as a PWA, enterprise look with Supabase
  as the reference. [12, 17]
- **Schema**: one definition of every message between agent and API and of every command's
  plan / choose / apply shape; types generated for Go and TypeScript. The contract. [12]
- **Postgres**, managed on DigitalOcean. Plain tables first; TimescaleDB when the rows justify
  it. [12, 18]
- **Log store**: Axiom by default (self-hosted Loki or VictoriaLogs are the alternatives). The
  dashboard queries it; logs never go through Postgres. [13]

## Identity and access

- **Google is the identity provider.** Sign in with Google (OpenID Connect). A
  `connertechnology.io` Workspace account is recognised as staff. Anyone else signs in with their
  own Google account and is allowed only if an admin invited that email. A second sign-in method
  can be added later without changing anything else. [5]
- **Accounts, invitations, roles and grants are Conner Technology's records**, kept in this API
  as their own module with their own database schema, walled off from the machine code so they
  can be extracted into a Conner Technology identity service after this ships. [4, 15]
- **Permissions** are named and tool-prefixed, one per action (for example `ctdev:machine.update`,
  `ctdev:doctor.run`, `ctdev:machine.uninstall`). The command audit produces the vocabulary.
  Every check in code is "does this user hold this permission in this scope". [7]
- **Roles** are sets of permissions. Four presets are seeded as built-in rows that cannot be
  deleted or edited, only copied: **owner** (everything, users, billing), **admin** (machines and
  grants in scope), **operator** (actions that change a machine), **viewer** (read only). Custom
  roles are composed from the permission checklist; the role editor ships after the presets. [7]
- **Rules:** the last owner cannot lose owner; a user can grant only roles they hold; destructive
  permissions are their own, never bundled into operator. [7, 9]
- **Grants** are (user, role, scope), where scope is the organisation or one group. [6]

## Machines and groups

- One organisation, Conner Technology. Staff, clients and family are all users of it. [1]
- Every machine belongs to exactly one **group** ("home", "office", a client's name). A client is
  a group its people can see. A client that later needs its own admins becomes a
  sub-organisation without changing how machines or permissions are stored. [6]
- A machine record holds: name, group, platform, agent version, credential, enrolment date,
  last heartbeat, current profile and drift, installed components, and the latest status and
  doctor summary. History lives in the time-series tables.

## Enrolment and agent security

1. An admin adds a machine to a group in the dashboard and receives a short-lived, single-use
   token. [14]
2. `ctdev enroll <token>` on the machine (the one physically local step) generates a key pair,
   exchanges the token for the machine's credential, and starts the agent. [10, 14]
3. The credential is the machine's own and is revoked from the dashboard; revocation disconnects
   the agent and rejoining needs a new token. [14]
4. The agent executes only commands the API sends after the API has checked the requesting
   user's permission; it never opens a shell or accepts an inbound connection. [14]

## Actions

- **Shape.** Every command exposes `plan` (what would happen, as structured data), `choose` (the
  subset or parameters the user picks) and `apply`. The CLI renders these in the terminal; the
  web renders them as pages. Pickers become checklists (pihole sync, install, uninstall,
  cleanup); the configure wizard becomes a settings page fed by the setup registry's detect,
  choices and apply; backup paths becomes a folder tree the agent reports. [10]
- **Remotely runnable** means non-interactive with explicit parameters. Everything in the audit
  gets a parameterised form; nothing stays terminal-only except the three physical exceptions:
  first install and enrolment, the Secure Boot key prompt at reboot, and a machine that is
  offline. [9, 10]
- **Streaming.** Output streams over the agent's WebSocket as it runs; the API relays it to open
  browsers and writes it to the action log at the same time. A machine running an action shows
  as busy. [13]
- **Action log.** Who asked, which machine, the exact command and parameters, start, finish,
  exit status, output. The first thing a machine page shows. [9]
- **Destructive actions** (`uninstall`, `restore in-place`) need their own permission and a typed
  machine name in the UI. [9]
- **Secrets** (Cloudflare token, restic credentials, brain token) are entered in the web UI over
  HTTPS, stored encrypted in Postgres, and handed to the agent only for the action that needs
  them. They never appear in the action log. [10]

## Data sources, monitoring and alerts

- The agent collects host metrics (CPU, memory, disk, network, containers) with gopsutil, the
  library Beszel itself uses, and sends them on a fixed interval. Beszel retires once the
  dashboard shows what it showed. [11]
- The agent or the server reads existing tools as data sources: Pi-hole, Caddy, restic,
  Tailscale, Docker, UniFi, Synology, Proxmox. A new data source is written only when nothing
  exists. Vendor API keys for `doctor --deep` live on the server, not on every agent. [11]
- **Time series and snapshots** are kept wherever comparing over time makes sense: metrics,
  status, doctor runs, drift, inventory changes. The dashboard shows how a machine is doing over
  time, not one screen. [11]
- **Heartbeat and status** a few times a minute; **doctor** daily and on demand, stored per run;
  **inventory** on change. [11]
- **Alerts** are defined and evaluated in our system by the worker against the rows, and
  delivered through ntfy (kept as the channel), email, or whatever comes later. The existing
  ntfy alerts on ctpi01 move here once the dashboard evaluates the same conditions. [11]

## Logs

The agent ships logs straight to the log store. The dashboard queries the store's API to show,
per machine, recent logs, errors, and the logs around an action. Ad-hoc exploration with a
query language stays in the log tool until it is worth building. [13]

## Command audit

The full table is in the interview notes. Buckets: **read** (viewer), **change** (operator),
**destructive** (own permission). Every command that is interactive today (configure wizard,
install and uninstall pickers, backup paths, pihole sync picker) or takes a secret on the
terminal (configure restic, caddy, brain, gpu) is a change-bucket command whose web form has to
be built, not one that stays local. [9, 10]

## Repository

One repo, this one, which is already `ConnerTechnology/ctdev` on GitHub. Proposed layout, to be
settled in the implementation plan: [16]

```
agent/    the Go module that is today's ctdev/: CLI, agent daemon, shared command plan/apply
api/      Node/Express/TypeScript API and worker
web/      React SPA/PWA
schema/   the contract; generates Go and TypeScript types
docs/     this document, the interview notes, engineering standards
```

CON-31 (rename the local checkout, module path and docs to match) is part of this project.

## Hosting

- API, worker and managed Postgres on DigitalOcean, one small droplet running containers behind
  Caddy, under the account that already holds Foil It Up's databases. [18]
- Web on Cloudflare Pages; `dashboard.connertechnology.io` in the Cloudflare DNS already in use. [18, 20]
- Sentry on API and web; CI builds the agent per platform the way the CLI already builds, and
  the containers for the rest. [12]

## Testing

- Agent and CLI: Go unit tests with fake executors, as today; the command plan / apply shapes are
  tested once and serve both renderers.
- API: unit tests per module; permission checks tested as a table (every permission × every
  preset role); the WebSocket protocol tested against a fake agent.
- Web: component tests; one end-to-end run of the first slice against a fake agent.
- The schema is the contract: generated types on both sides fail the build when they diverge.

## First slice [17]

1. Screens with fake data: sign-in, machine list with status, one machine page with a live
   action stream and recent doctor results, users and roles. Reviewed by Thomas before any
   service exists.
2. Services: Google sign-in and invitations; enrolment; the agent's heartbeat and status; `doctor`
   as the first remote action, streamed live.
3. Schema, shaped by what the screens needed.
4. ctpi01 enrolled first, then the desktop.

Every later command is the same shape: its screen, its parameterised form, its permission.

## Later, decided but not in the first slice

- The role editor (custom roles); presets only at first. [7]
- Extract identity and access into a Conner Technology identity service used by this and future
  internal tooling. [15]
- Sub-organisations for a client that needs its own admins. [6]
- TimescaleDB once the metric rows justify it. [12]
- Beszel and the ctpi01 ntfy alerts retired once the dashboard covers them. [11]
- A second sign-in method for someone with no Google account. [5]

## Not part of this

Foil It Up. It is a separate product with its own users; nothing here touches it. [4, 15]

## Open questions for the implementation plan

These do not change the design; they are choices the plan makes.

- Schema tooling: protobuf, JSON Schema with generators, or TypeSpec.
- WebSocket message framing and reconnect and backfill behaviour when a machine was offline.
- Encryption key management for the secrets column (DigitalOcean has no KMS; a key in the
  droplet's environment from 1Password, or SOPS/age per the AI repo's conventions).
- Metric interval and retention numbers.
- When CON-31's rename lands relative to the new directories.
