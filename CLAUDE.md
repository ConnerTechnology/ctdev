# CLAUDE.md

`ctdev` is a single Go CLI that sets up and diagnoses a machine: it installs components, applies
declarative machine profiles, configures system settings, runs backups, and diagnoses hardware it
did not set up. The Go module is `ctdev/`; everything else at the root is the installer, the docs
and CI.

## Where knowledge lives

This file is orientation only. It is read at the start of every session, so anything that is true
of one place in the tree belongs somewhere that loads when you open that place.

| Home | What is in it |
| --- | --- |
| This file | Orientation, and the conventions that bind everywhere |
| `.claude/rules/` | The facts about one component or one directory. Each file is scoped by a `paths:` glob and loads when you open a file it matches — Pi-hole, Caddy, Portainer, Beszel, restic, mcp-email-server, the brain, doctor, profiles, the install scripts, the Go conventions inside the module |
| `docs/commands.md` | Every command and flag, with the examples `--help` leaves out |
| `docs/architecture.md` | The directory map of the Go module |
| `docs/adding-a-component.md` | The template for a new component, plus the `Root` / Phase 1-2 / `.deb` rules |
| `docs/decisions/` | ADRs. Created with the first one; if it is missing, there are none yet |
| `docs/agents/` | How the engineering skills use Linear, the triage labels, and the domain docs |
| Linear team `CON` | Specs, design documents and tickets. Not GitHub Issues |
| `RECOVERY.md` | Disaster recovery: restoring a machine from restic |
| `TROUBLESHOOTING.md` | Symptoms and fixes on a live machine |
| `CHANGELOG.md` | What shipped in each version |

A fact that is true of one component goes in that component's rule, not here. If you find yourself
wanting to add a paragraph to this file, it almost certainly belongs in a rule or in `docs/`.

## Conventions

These hold everywhere in the repo.

- Use `inst` as the Go receiver name, not a single letter.
- Commit messages are conventional format, one line. No body, no footer, no `Co-Authored-By`.
- Commit or push only when asked.
- Never commit a secret. Every secret is entered at `configure` time and stored only on the host
  that needs it.

## Releases

1. Commit changes
2. Update CHANGELOG.md
3. Bump VERSION
4. Commit: `docs: update for vX.Y.Z`
5. Tag: `git tag vX.Y.Z`
6. Push: `git push && git push --tags`
7. CI builds and creates the GitHub Release automatically via `.github/workflows/ci.yml`

## Guidance is checked

`scripts/check-guidance.sh` runs in CI. It holds this file to a byte ceiling, holds each rule to
6,000 bytes, requires every rule to be path-scoped, and verifies that every repo path named in a
backticked token here, in a rule, or in `docs/agents/` actually exists. It also verifies that
every relative link in `README.md` and under `docs/` points at a file that exists, and that a
link's `#anchor` is a heading in it. A path or link that is dead on purpose goes in `scripts/check-guidance.allow` with a
reason.

## Working with the skills

`docs/agents/` is what the engineering skills read: `docs/agents/issue-tracker.md` for how Linear is
used here, `docs/agents/triage-labels.md` for the label mapping, and `docs/agents/domain.md` for the
glossary and the ADRs.
