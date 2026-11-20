# ctdev

`ctdev` is Conner Technology's internal tool for managing and maintaining devices and the networks
they sit on: our own today, clients' later. It is a single Go CLI that installs components,
configures system settings, runs backups, and diagnoses devices, including unmanaged devices it can
see but not install on. The Go module is `ctdev/`; everything else at the root is the installer,
the docs and CI.

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
| `docs/agents/where-things-go.md` | Where each kind of fact goes, and how to trace a line back to its Linear issue |
| `.claude/rules/` | What no single file can say about one component or directory: conventions across files, traps, outside facts. Each file is scoped by a `paths:` glob and loads when you open a file it matches — Pi-hole, Beszel, the homelab node, doctor, the install scripts, the Go conventions inside the module |
| `docs/README.md` | The human-facing docs: one page per capability, in README order |
| `docs/commands.md` | Every command and flag, with the examples `--help` leaves out |
| `docs/architecture.md` | The directory map of the Go module |
| `docs/adding-a-component.md` | The template for a new component, plus the `Root` / Phase 1-2 / `.deb` rules |
| `TROUBLESHOOTING.md` | Symptoms and fixes on a live machine |
| `CHANGELOG.md` | What shipped in each version |

What ctdev does is what its code says: the why goes in a comment where the code acts on it. A
paragraph you want to add here almost certainly belongs in a home that
`docs/agents/where-things-go.md` lists.

## Where things get recorded

Auto memory is off in this repo. A decision made in conversation lands, in the same session, in the
file the next session reads:

- **Work**: Linear, team CTDev (`CTD`) (`docs/agents/issue-tracker.md`).
  Update issues as work happens.
- **Vocabulary**: `CONTEXT.md`.
- **Decisions**: `docs/adr/`.
- **Rules for agents**: this file, the path-scoped rule the fact belongs to, or the skill.

## How we work

- A human gives a goal; the lead session takes it to `main` and a release on its own, through
  "Working on a goal". It records the work in Linear (`/to-spec`, `/to-tickets` for a large goal),
  has every diff reviewed in a fresh context against the goal, and the human never merges or releases.
- A bug that reaches a release is fixed with a note on how development missed it and what now
  catches it (`docs/agents/issue-tracker.md`).
- Before a change, say what it does to the work after it on the board, and shape it so that work
  keeps it.
- Work that needs the human comes one item at a time: "what next" names a single item and what it
  unblocks. Work agents can do alone runs in parallel.
- Governing files (`AGENTS.md`, `.claude/settings.json`, `.mcp.json`, `.claude/skills/`,
  `.claude/rules/`) change by showing the human the diff first and landing it after approval.

## Who decides what

Ask the human only about architecture: the kind of datastore (document or relational); a new
service, process boundary or deploy target; a new vendor or anything paid; a change to the security
or permission model; breaking a public API or a stored data format; deleting data or publishing
anything. Decide the rest without asking (schema shape, code, libraries in the current stack, file
layout, tests, how to slice a goal) and record the larger choices in an ADR. Never stop to ask how
to build something. Ask one question with a proposed default, and keep building what it doesn't block.

## Working on a goal

`scripts/goal.sh`, run from the primary checkout (`docs/agents/goal-loop.md`):

1. `start <slug>`: goal branch `goal/<slug>` from `origin/main`, its worktree, a draft pull request.
   The slug starts with the Linear ID.
2. `slice <slug> <name>`: a worktree per slice, cut from the goal's head. One agent per worktree,
   in parallel; a slice that needs another starts after that one merges.
3. `merge <slug> <name>`: checks what it touched, merges it into the goal on this machine, pushes the
   goal, removes the slice. The session resolves conflicts on the goal branch.
4. A fresh-context reviewer reads the pull request's diff against the goal; fix what it finds.
5. `land <slug>`: full checks, then the pull request goes ready so GitHub's checks run once; merged
   when they pass, then released. It waits in the turn: a turn that ends on "once CI passes" stalls.
   `status <slug>` says what is left.

Never post a commit status by hand, merge around a failing check, or force-push `main`. Leave no
worktree or branch behind.

## Talking to the human

- Concise and direct, in American English.
- One question or decision per message, with a proposed default. This holds inside `/grilling` too,
  where the skill would ask a numbered round. Reviews and audits go one item at a time. A list of
  already-scoped work can be approved in one go.
- Anything the human does personally comes as numbered steps, one action each (where to go, what to
  click, what to enter), checked against the vendor's current UI. When a step changes, restate the
  sequence from step 1.

## Git

- Changes reach `main` through a GitHub pull request. The release commit in "Releases" is the one
  exception.
- A commit's subject is one line in conventional-commit form, `type: description`. Its body, a
  line or two, cites the Linear issue and says why (this overrides a user-level one-line rule);
  the release commit has no issue and no body.
  Attribution is off, set in `.claude/settings.json`.

## Releases

1. Commit changes
2. Run the prompt audit headless (`docs/agents/where-things-go.md`); file an issue for what it
   finds, which doesn't block the release
3. Update CHANGELOG.md
4. Bump VERSION
5. Commit: `docs: update for vX.Y.Z`
6. Tag: `git tag vX.Y.Z`
7. Push: `git push && git push --tags`
8. CI builds and creates the GitHub Release automatically via `.github/workflows/ci.yml`

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
