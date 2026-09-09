# ctdev platform interview — notes

Started 2026-09-09. Thomas's direction, in his words: sign in to the tool through a Conner
Technology auth system with roles and permissions; a web UI that shows every managed machine, the
actions you can take on it, status and doctor; audit every current command one by one; plan the
work as one larger project for managing the machines on the Conner Technology network.

One question per message. Each answer is recorded here as it is given, with the decision it
produced. Decisions are not re-litigated; a later session revises them only when it finds
something.

## Decisions

1. **Multi-user from the start, organisation-shaped.** One organisation (Conner Technology) with
   user accounts an admin adds, each carrying roles and permissions, the way a Google Workspace
   admin adds people who then get their own space inside it. Only Thomas today; staff, clients and
   possibly family later. The who is not what matters; the ability to add accounts and manage
   their roles is.
2. **Agent on each machine, connecting out.** ctdev gains a daemon mode that holds a connection to
   the server, reports status, and executes the commands the server hands it. No inbound access,
   works behind NAT, the machine keeps working offline. Rejected: server pushing over ssh; a
   per-machine API the server calls.
3. **The server runs on a cloud host with a public domain from day one.** The agent address is
   fixed forever, staff and client machines outside the tailnet can reach it, and the front door
   is authenticated from the first commit. Provider not chosen. Rejected: ctpi01, tailnet-only.
4. **One shared Conner Technology identity, trusted by every internal tool.** Accounts, roles
   and sign-in live in one place; this dashboard (and any later internal tool) asks it who a user
   is and what they may do. **Foil It Up is a separate product with its own users and is not
   part of this** (Thomas, Q15). Rejected: a user table owned by one tool.
5. **Google is the identity provider; Conner Technology keeps the accounts and roles.** Sign in
   with Google (OpenID Connect) for everyone: a `connertechnology.io` Workspace account is
   recognised as staff; clients and family sign in with their own Google account and are allowed
   only if an admin invited that email. Roles and permissions are Conner Technology's records,
   read by every tool, not Google's. A second sign-in method can be added later for someone with
   no Google account. Rejected: building an identity service; self-hosting Keycloak or Zitadel;
   a hosted provider. (Foil It Up does not migrate to this; it is separate.)
6. **Machines belong to exactly one group; roles are granted per group or organisation-wide.**
   Groups such as "home", "office", or a client's name. A client is a group its people can see.
   A client that later needs its own admins becomes a sub-organisation without changing how
   machines or permissions are stored. Rejected for now: per-machine grants; sub-organisations.
7. **Custom roles built from a permission checklist, four presets seeded.** Every action has a
   named, tool-prefixed permission (the command audit produces the vocabulary); a role is a set
   of them; a grant is (user, role, scope). Presets owner, admin, operator, viewer are seeded as
   built-in rows that cannot be deleted or edited, only copied. Rules: the last owner cannot lose
   owner; a user can grant only roles they hold. Ship the presets first, the role editor after.
8. **Local commands stay free; server commands need sign-in.** Install, configure, apply, doctor
   and status on the machine you are at keep working with nothing but the binary, offline
   included. Enrolling a machine, seeing other machines, or acting on one remotely needs a
   signed-in user (browser device-code flow, like `gh auth login`). An enrolled machine's agent
   runs with the machine's own credential, never a person's. Rejected: sign-in for everything.
9. **Remote means non-interactive with explicit parameters; every remote action is logged;
   destructive actions need a typed machine name and their own permission.** Anything that asks
   on the terminal gains a parameterised form or stays local. The action log (who, machine,
   command, start, finish, output) is the first thing a machine page shows. The command audit
   below decides each command against this rule.
10. **Everything runs remotely from the web UI; every command is plan, choose, apply over
    structured data, and the terminal and the web are two renderers of the same thing.** Pickers
    become pages with checkboxes; the configure wizard becomes a settings page fed by the setup
    registry's detect / choices / apply; backup paths moves into the main UI with the agent
    reporting the folder tree; secrets are entered in the web UI, stored encrypted on the server,
    and handed to the agent only for the action that needs them. Physically local, always: the
    first install and enrolment of a machine, the Secure Boot key prompt at reboot, and any
    machine that is offline. This supersedes the "local only" bucket in the audit: those rows are
    the ones whose parameterised or web form has to be built, not ones that stay local.
11. **One dashboard, `dashboard.connertechnology.io`; data sources feed a database; the web
    reads the database.** Thomas: "we don't want to reinvent the wheel but I do want things to
    all go through this web ui." A data source is an existing tool or API the agent or server
    reads (Pi-hole, Caddy, restic, Tailscale, UniFi, Synology, Proxmox, Docker); we write our own
    data source only when nothing exists for what we need. Data is stored as snapshots or time
    series where comparing over time makes sense (metrics, doctor runs, status, drift), so the
    dashboard can show how a machine is doing over time, not one screen. Beszel retires once the
    dashboard shows what it showed; its metric collection is reproduced in our agent with the
    same library (gopsutil), not by running a second hub. ntfy stays as the alert delivery
    channel; alerts are defined and evaluated in our system.
12. **Stack:** Go agent (the existing ctdev code); Postgres; API in Node, Express and TypeScript,
    which also terminates the agents' WebSocket connections and dispatches commands; dashboard as
    a React and TypeScript SPA built as a PWA; a worker for scheduled and background jobs; a
    generated schema as the contract between agent and API (plan / choose / apply and every
    message defined once, types generated for Go and TypeScript). Time-series storage starts as
    plain Postgres; TimescaleDB when the rows justify it. Migrations, an encryption key for
    secret columns, Sentry on API and web, TLS through Caddy, CI building the agent per platform.
13. **Actions stream live; logs go to a log store the dashboard queries.** An action's output
    streams over the agent's WebSocket as it runs; the API relays it to open browsers and writes
    it to the action log at the same time, so a run is watched live and read back later. A
    machine running an action shows as busy. Logs ship from the agent straight to a log store
    (Axiom by default; self-hosted Loki or VictoriaLogs are the alternatives), never through
    Postgres. The dashboard queries the store's API to show recent logs, errors, and the logs
    around an action per machine; ad-hoc exploration stays in the log tool until it is worth
    building. Thomas: "I just hate having to log in to multiple applications to see all the
    data when I would like to just see everything in one place."
14. **Enrolment by one-time token; the machine holds its own revocable credential.** An admin
    adds a machine to a group in the dashboard and gets a short-lived token; `ctdev enroll
    <token>` on the machine exchanges it for a key pair the machine generates, and the token dies
    after one use. Revoking the credential from the dashboard disconnects the agent; rejoining
    needs a new token. A person's sign-in never reaches a machine. The agent executes only what
    the API sends, after the API has checked the user's permission; it reports back, never opens
    a shell, never accepts an inbound connection. The agent updates itself from the same release
    pipeline as the CLI. No extra approval step for a new machine.
15. **Accounts and roles live inside this API for now, as their own module with their own
    database schema**, walled off from the machine code. Follow-up after this ships: extract the
    Conner Technology identity and auth system into its own service used by this dashboard and
    future internal tooling.
16. **One repo, this one.** The Go CLI and agent, the API, the web app, the worker and the
    schema live together; one CI builds the agent per platform and the containers for the rest;
    a schema change and both sides' updates land in one commit. Layout to be decided in the
    design (something like `agent/`, `api/`, `web/`, `schema/`). Fits the intended rename (CON-31).
17. **First slice, UI first, then services, then schema.** Screens with fake data first: sign-in,
    machine list with status, one machine page with a live action stream and recent doctor
    results, users and roles. Then Google sign-in and invites, enrolment, heartbeat and status,
    and `doctor` as the first remote action, streamed live. Schema last, shaped by the screens.
    ctpi01 is the first enrolled machine, then the desktop. Every later command follows the same
    shape: its screen, its parameterised form, its permission. **Look: enterprise, Supabase as
    the reference, not the Mixpanel look used for Foil It Up.**
18. **Hosting: DigitalOcean for the API, worker and a managed Postgres; Cloudflare Pages for the
    web app; `dashboard.connertechnology.io` in the Cloudflare DNS already in use.** One small
    droplet runs the containers behind Caddy. Managed Postgres chosen over self-hosted so a system
    whose point is not babysitting machines does not need its own database babysat.
19. **The name stays ctdev, for the CLI, the agent, the dashboard and the repo.** Thomas: "I
    honestly like ctdev and have gotten used to it." Explored and set aside the same day: Steward,
    Warden, Keel, Foreman; then Ops with `ctops` as the command, which he liked for a moment
    before deciding to keep ctdev. Not to be reopened without a new reason. CON-31 (rename the
    local folder, module path and docs to match the GitHub repo, already `ConnerTechnology/ctdev`)
    stands.
20. **The dashboard is `dashboard.connertechnology.io`.**

## Questions and answers

**Q1. Who uses this a year from now?** Just Thomas for now; staff and clients later, maybe family.
"The who is not necessarily important. We just need to be able to add user accounts and manage the
roles and permissions around them." Model: Google Workspace, where the admin adds people who get
their own workspace inside it. → Decision 1.

**Q2. How does the server reach a machine?** Agent on each machine connecting out (Beszel and
Tailscale shape). → Decision 2.

**Q3. Where does the server live?** Cloud host. Provider left open. → Decision 3.

**Q4. Is Conner Technology identity shared across tools or per tool?** Shared. → Decision 4.

**Q5. Build, self-host, or buy the identity provider?** Thomas: "We already have a Conner
Technology Google Workspace. Can we use that?" Yes, as the identity provider, with invited accounts
and roles kept by Conner Technology. → Decision 5.

**Q6. How are machines grouped, and what does a permission attach to?** Groups. → Decision 6.

**Q7. Fixed roles or custom roles from a permission checklist?** Thomas asked how much work
custom-with-presets is; answer: the permission vocabulary is needed either way, custom adds two
tables, three rules and a role-editor screen, roughly two or three sessions beyond the fixed set,
and splits so presets ship first. Chosen: custom with presets. → Decision 7.

**Q8. Does the CLI keep working without an account?** Yes for local commands; sign-in for
anything that touches the server. → Decision 8.

**Q19. What is the whole thing called?** Two rounds of alternatives, a brief Ops / `ctops`, then
Thomas kept ctdev. → Decision 19.

**Q20. The dashboard's address?** dashboard.connertechnology.io. → Decision 20.

**Q18. Which cloud host?** DigitalOcean (already pays for Foil It Up's Postgres) plus Cloudflare
Pages (already builds the other sites), managed Postgres. → Decision 18.

**Q17. What ships first?** The proposed slice and order. Thomas: "let's not use Mixpanel as the
design for this UI. I want a more enterprise look. Supabase is a good example." → Decision 17.

**Q16. One repo or several?** One repo. → Decision 16.

**Q15. Where do the accounts and roles live?** Thomas first corrected scope: Foil It Up is
separate and has nothing to do with this (decisions 4 and 5 amended). Then: inside the API for
now; extracting the Conner Technology identity service is a follow-up after shipping. → Decision 15.

**Q14. How does a machine join, and what is its identity afterwards?** As proposed. → Decision 14.

**Q13. Live action output and logs.** Thomas described sending actions and watching results
stream back in real time, and shipping logs to Axiom or similar while the dashboard stays the
central place to look. → Decision 13.

**Q12. What is the server and web built from?** Thomas named the four: Go agent, Postgres,
Node/Express/TypeScript API, React/TypeScript SPA PWA. Added: the API holds the agent connection,
a schema replaces shared code, a worker, time-series storage, the boring layer. → Decision 12.

**Q11. What does a machine report on its own, and does this replace Beszel and ntfy?** Thomas:
leverage tools that capture data, store rows, read them in the dashboard; write our own only when
no tool exists; "we connect datasources and can also create our own datasource if one doesn't
exist"; snapshots or time series over time where comparison makes sense; one login at
dashboard.connertechnology.io rather than Beszel and others. → Decision 11.

**Q10. Can everything run remotely from the web UI?** Thomas: "I would love to be able to run
everything remotely from the web ui instead of some being locally only." Yes, with the three
physical exceptions, by making every command plan / choose / apply and the web the interactive
layer. → Decision 10.

**Q9. Which commands may run remotely, and what protects the dangerous ones?** Rule accepted;
Thomas wants the full command list to check for exclusions. → Decision 9 and the audit.


## Command audit (ctdev v12.23.0, every command, proposed against decision 9)

Buckets: **read** (viewer; safe anywhere), **change** (operator; non-interactive with parameters),
**destructive** (own permission, typed machine name), **local only** (interactive or handles a
secret on the terminal; never offered remotely as it stands).

| Command | Bucket | Note |
| --- | --- | --- |
| `info` | read | inventory: specs, kernel, uptime, profile, drives, components |
| `status` | read | needs-attention screen; the agent's heartbeat payload |
| `doctor` (all flags) | read | `--report` output stored with the run; `--deep` needs vendor keys the server holds, not the agent |
| `verify` | read | bootstrap check |
| `diff <profile>` | read | drift check; exit code is the result |
| `update --check` | read | pending updates; a good scheduled poll |
| `backup snapshots`, `backup test` | read | |
| `restore ls`, `restore check` | read | |
| `configure --show`, `configure <category> --show` | read | current settings |
| `pihole sync --dry-run` | read | the diff only |
| `update -y` | change | already non-interactive |
| `apply <profile> --batch` | change | already non-interactive |
| `install <components> --batch` | change | already non-interactive |
| `configure <category> --batch` | change | applies the category's defaults; the flagged ones (`git`, `aws`, `caddy`, `brain`, `mcp-email-server`) take parameters |
| `backup now`, `backup enable`, `backup disable` | change | |
| `restore files <snap> <dir>` | change | writes into a directory, overwrites nothing live |
| `cleanup --batch` | change | deletes caches; remote form must show the scan before the clean, so it needs a two-step (scan, then clean chosen tasks) |
| `pihole sync` | change, needs a parameterised form | today a picker; remote form: apply all adds and updates, removals only when named |
| `uninstall <components> --batch` | destructive | removes software |
| `restore in-place <snap>` | destructive | overwrites live files |
| `configure` (wizard), `install` / `uninstall` (pickers), `backup paths` (web UI), `pihole sync` (picker) | local only | interactive; the parameterised forms above are the remote equivalents |
| `configure restic`, `configure caddy --cf-token`, `configure brain --token-ref`, `configure gpu` | local only | secrets on the terminal, or Secure Boot MOK signing which needs a reboot and a keyboard; remote later only with server-held secrets |
| `pihole export` | local only | writes a file into a checkout |

Decision 10 folds the local-only bucket into the change bucket: each of those rows needs its web form. Thomas did not move any command between read, change and destructive.
