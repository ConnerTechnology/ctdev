# ctdev

`ctdev` is Conner Technology's internal tool for managing and maintaining devices and the networks
they sit on: our own today, clients' later. It is a single Go CLI that installs components, applies
declarative machine profiles, configures system settings, runs backups, and diagnoses devices,
including unmanaged devices it can see but not install on. The Go module is `ctdev/`; everything else at the root is the installer, the docs
and CI.

## Guardrails

- The repo is public: anything committed is published. Never commit a secret. Every secret is
  entered at `configure` time and stored only on the host that needs it.
- No real family or client data: not as fixtures, test data or examples. Test data is synthetic
  and obviously fake (`Example Vendor`, `test@example.invalid`).
- Never invent a fact. The repo and Linear are the source of truth: read the code, the docs and the
  tickets for what is already known, and if they don't answer it, ask. When a doc and the code
  disagree, the code is what ctdev does; say so rather than pick silently.

## Where knowledge lives

This file is orientation only. It is read at the start of every session, so anything that is true
of one place in the tree belongs somewhere that loads when you open that place.

| Home | What is in it |
| --- | --- |
| This file | Orientation, and the conventions that bind everywhere |
| `.claude/rules/` | The facts about one component or one directory. Each file is scoped by a `paths:` glob and loads when you open a file it matches — Pi-hole, Caddy, Portainer, Beszel, restic, mcp-email-server, doctor, profiles, the install scripts, the Go conventions inside the module |
| `docs/README.md` | The human-facing docs: one page per capability, in README order |
| `docs/commands.md` | Every command and flag, with the examples `--help` leaves out |
| `docs/architecture.md` | The directory map of the Go module |
| `docs/adding-a-component.md` | The template for a new component, plus the `Root` / Phase 1-2 / `.deb` rules |
| `RECOVERY.md` | Disaster recovery: restoring a machine from restic |
| `TROUBLESHOOTING.md` | Symptoms and fixes on a live machine |
| `CHANGELOG.md` | What shipped in each version |

A fact that is true of one component goes in that component's rule, not here. If you find yourself
wanting to add a paragraph to this file, it almost certainly belongs in a rule or in `docs/`.

## Where things get recorded

Auto memory is off in this repo. A decision made in conversation lands, in the same session, in the
file the next session reads:

- **Work**: Linear, team CTDev (`CTD`) (`docs/agents/issue-tracker.md`).
  Update issues as work happens.
- **Vocabulary**: `CONTEXT.md`.
- **Decisions**: `docs/adr/`.
- **Rules for agents**: this file, the path-scoped rule the fact belongs to, or the skill.

## How we work

- Thomas delegates to the lead session. The lead interviews Thomas (`/grilling`), writes the spec
  and tickets together (`/to-spec`, `/to-tickets`), and reviews every diff in a fresh context against
  the plan, judging the diff and the evidence rather than the implementing agent's report.
- Ask Thomas when unsure or when the decision is Thomas's to make.
- Before a change, say what it does to the work after it on the board, and shape it so that work
  keeps it.
- Work that needs Thomas comes one item at a time: "what next" names a single item and what it
  unblocks. Work agents can do alone runs in parallel.
- Governing files (`AGENTS.md`, `.claude/settings.json`, `.mcp.json`, `.claude/skills/`,
  `.claude/rules/`) change by showing Thomas the diff first and landing it after approval.

## Talking to Thomas

- Concise and direct, in American English.
- One question or decision per message, with a proposed default. This holds inside `/grilling` too,
  where the skill would ask a numbered round. Reviews and audits go one item at a time. A list of
  already-scoped work can be approved in one go.
- Anything Thomas does personally comes as numbered steps, one action each (where to go, what to
  click, what to enter), checked against the vendor's current UI. When a step changes, restate the
  sequence from step 1.

## Git

- Changes reach `main` through a GitHub pull request. The release commit in "Releases" is the one
  exception.
- Commit messages are one line in conventional-commit form, `type: description`. Attribution is
  off, set in `.claude/settings.json`.

## Releases

1. Commit changes
2. Update CHANGELOG.md
3. Bump VERSION
4. Commit: `docs: update for vX.Y.Z`
5. Tag: `git tag vX.Y.Z`
6. Push: `git push && git push --tags`
7. CI builds and creates the GitHub Release automatically via `.github/workflows/ci.yml`

## Guidance is checked

`scripts/check-guidance.sh` runs in CI. It holds this file and `README.md` to byte ceilings, holds
each rule to 6,000 bytes, requires every rule to be path-scoped, and verifies that every repo path
named in a backticked token here, in a rule, or in `docs/agents/` actually exists. It also verifies
that every relative link in `README.md` and under `docs/` points at a file that exists, and that a
link's `#anchor` is a heading in it. A path or link that is dead on purpose goes in
`scripts/check-guidance.allow` with a reason.

## Agent skills

### Issue tracker

Linear, team CTDev (`CTD`), a sub-team of Conner Technology. See `docs/agents/issue-tracker.md`.

### Triage labels

Mapped onto Linear: `ready-for-agent` and `ready-for-human` are labels, while "needs triage" and
"won't fix" are the Backlog and Canceled states. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context. See `docs/agents/domain.md`.
