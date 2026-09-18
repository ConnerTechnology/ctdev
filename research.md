# Overview-style READMEs: what well-regarded repos actually do

Research date: 2026-09-18. Every README below was fetched from GitHub on that date
(`gh api repos/<owner>/<repo>/readme`) and measured, not recalled. Byte counts are the
GitHub blob `size` field; line counts are `wc -l` on the decoded file.

Baseline for comparison: `ConnerTechnology/dotfiles/README.md` is **24,021 bytes,
446 lines, 28 headings at `#`–`###`**. That puts it between ripgrep (21.6 KB) and
lazygit (38 KB) — i.e. squarely in the group this study classifies as "the README
became the manual". The repo currently has no user-facing `docs/` tree (only
`docs/superpowers/`), plus root-level `RECOVERY.md`, `TROUBLESHOOTING.md`, `CHANGELOG.md`.

---

## 1. Comparison table

| Repo | Bytes | Lines | Ordered top-level headings | First screen (~25 lines) | Deep docs live | Install |
| --- | --- | --- | --- | --- | --- | --- |
| twpayne/chezmoi | 547 | 17 | Contributors; License | Logo+title, 1 release badge, one-sentence description, link to chezmoi.io, link to developer guide | `assets/chezmoi.io/docs/` **in repo** (mkdocs → chezmoi.io) | Linked only |
| tailscale/tailscale | 2,977 | 87 | Overview; Using; Other clients; Building; Bugs; Contributing; About Us; Legal | Title, bare URL, 4-word tagline, then Overview: what's in this repo, sibling repos, link to opensource policy | Product docs off-repo (tailscale.com/kb); `docs/` in repo holds contributor/platform notes (`cli.md`, `commit-messages.md`, `k8s/`, `windows/`) | Linked (pkgs.tailscale.com) |
| portainer/portainer | 4,089 | 61 | Intended use; Latest version; Getting started; Getting help; Reporting bugs and contributing; Security; Work for us; Licensing | Banner image, 2 bold paragraphs CE vs BE, 3 links to the commercial product | docs.portainer.io (separate) | Linked (`Deploy Portainer`) |
| henrygd/beszel | 4,262 | 73 | Features; Architecture; Getting started; Screenshots; Supported metrics; Help and discussion; License | Title, 2-sentence what-it-is, 4 badges, one wide screenshot | beszel.dev (separate) | Linked (quick start guide) |
| restic/restic | 5,700 | 109 | Introduction; Quick start; Backends; Design Principles; Reproducible Builds; News; License; Sponsorship | 3 badges above the H1, then a 2-sentence description and a link to readthedocs | `doc/` **in repo**, Sphinx `.rst`, numbered `010_`…`110_` + `index.rst` (→ restic.readthedocs.io) | Linked; a runnable quick start is inline |
| nix-community/home-manager | 5,282 | 124 | (setext) Home Manager using Nix; Usage; Releases; Words of warning; Contact; Installation; Translations; License | Title, 2-sentence what-it-is, then "read the warning below" and three links to the manual/options search | `docs/` in repo (mdbook + `manual/`, `release-notes/`), published manual | Section present but mostly links to the manual |
| cli/cli (gh) | 6,262 | 122 | GitHub CLI; Documentation; Agent skills; Contributing; Installation; Comparison with hub | Callout note, one-sentence "gh is GitHub on the command line", one screenshot, a support-scope sentence | `docs/` **in repo** = *developer* docs; the user manual is off-repo (cli.github.com/manual) | Per-OS **stubs that link** to `docs/install_*.md` |
| jdx/mise | 8,439 | 205 | What is mise?; Quickstart; Check your project setup; Where to go next; Demo; GitHub Issues & Discussions; Special Thanks; Contributors | Centered logo, 4 badges, one-line pitch, a 5-link nav row, sponsor block | `docs/` **in repo** (VitePress → mise.jdx.dev), incl. `docs/cli/` one file per command + `index.md` | One `curl` line inline, everything else linked |
| pi-hole/pi-hole | 9,419 | 170 | One-Step Automated Install; Alternative Install Methods; Post-install (linked heading); Pi-hole is free…; Getting in touch; Breakdown of Features | Logo, tagline, then a 10-bullet "what it is" list with inline links | docs.pi-hole.net (separate repo) | **Inline** (curl-pipe-bash plus alternatives) |
| caddyserver/caddy | 12,661 | 225 | Features (linked heading); Install; Build from source; Quick start; Overview; Full documentation; Getting help; About | HTML-centered logo, tagline, a 3-link nav row (Releases · Documentation · Get Help), badge row | caddyserver.com/docs, source in `caddyserver/website` | Short inline + link; **build-from-source is long and inline** |
| charmbracelet/bubbletea | 14,892 | 402 | By the way; Tutorial; Initialization; What's Next?; Debugging; Libraries we use…; Bubble Tea in the Wild; Contributing; Feedback; Acknowledgments; License | Title, badges, animated GIF, 3 short paragraphs | No `docs/` — `examples/`, `UPGRADE_GUIDE_V2.md`, pkg.go.dev | n/a (library) |
| mikefarah/yq | 18,338 | 480 | yq; Quick Usage Guide; Install; Community Supported Installation methods; Features; Usage (linked heading); Troubleshooting; Known Issues | Title, 5 badges, 2 paragraphs of what-it-is | mikefarah.gitbook.io/yq; `mkdocs.yml` + `how-it-works.md` at root | **Inline and very long** (~270 lines of install variants) |
| BurntSushi/ripgrep | 21,599 | 541 | (setext) ripgrep (rg); then `###` CHANGELOG, Documentation quick links, Screenshot, Quick examples…, Why should I use ripgrep?, …, Installation, Building, Running tests, Related tools, Vulnerability reporting, Translations | Setext title, 9-line description paragraph, 3 badges, license line, then a "Documentation quick links" list | Root-level `GUIDE.md`, `FAQ.md`, `CHANGELOG.md` (no `docs/`) | **Inline**, ~215 lines |
| jesseduffield/lazygit | 38,288 | 643 | Sponsors; Elevator Pitch; Table of contents; Features; Tutorials; Installation; Usage; Configuration; Contributing; Donate; FAQ; Shameless Plug; Alternatives | ~50 lines of sponsor banners before any project content | `docs/` **in repo** with `docs/README.md` index, topic files, `docs/dev/`, `docs/keybindings/` | **Inline**, ~300 lines, ~30 package managers each with its own `###` |
| junegunn/fzf | 41,829 | 1,153 | (heavy `###` nesting) Using Homebrew … Preview window … Fish shell | Centered image, 7 badges, then a merch banner | Root-level `ADVANCED.md`, `BUILD.md`, `README-VIM.md`, plus `man/` | **Inline**, every platform |

Fetch note: all 15 READMEs resolved on first try; none had moved. `gh api
repos/<repo>/contents/docs` returned 404 for the repos listed above as having no
docs folder, which is how "no docs/ in repo" was established.

---

## 2. Patterns that recur in the overview-style READMEs

**Nothing but identity comes before the first heading.** chezmoi, tailscale, beszel,
cli/cli and mise all put the same four things above the fold and nothing else: name,
one-sentence description, badges or a nav row, and a picture or a link to the docs
site. caddy and mise do it in raw HTML to center it; tailscale does it in three plain
lines of Markdown. The counter-example is lazygit, where ~50 lines of sponsor banners
run before "Elevator Pitch".

**The "what is this" statement is one or two sentences and it is the first prose.**
chezmoi: "Manage your dotfiles across multiple diverse machines, securely." (one line,
whole README is 17 lines). tailscale: "Private WireGuard® networks made easy."
cli/cli: "`gh` is GitHub on the command line. It brings pull requests, issues, and
other GitHub concepts to the terminal…". beszel: two sentences. Standard Readme makes
this a hard rule — the Short Description is Required, "Must be less than 120
characters", "Must be on its own line", and must match the GitHub repo description
(spec.md, Short Description).

**A picture or a demo, once, near the top — and only for tools with a visible
surface.** cli/cli: one screenshot at line 8. beszel: one dashboard screenshot at line
12, with a longer `## Screenshots` section further down. bubbletea: an animated GIF at
line 15. mise puts its demo GIF in a dedicated `## Demo` section near the bottom rather
than the top, and links a text transcript next to it. tailscale, restic, chezmoi and
portainer's CE README have no product screenshot at all; tailscale is a daemon, restic
is a CLI with no TUI.

**The command surface is not listed in an overview README.** Not one of the short
READMEs contains a table of every subcommand. Three substitutes appear:
- *A capability list with each bullet linked into the docs site.* caddy's `## Features`
  is 20 bullets, most of them links to caddyserver.com/docs; beszel's `## Supported
  metrics`; pi-hole's 10 adjective-led bullets; mise's four linked bullets
  (Tools / Environments / Tasks / Bootstrap).
- *A short runnable sequence.* restic's `## Quick start` is a single `init` → `backup`
  → `snapshots` transcript, about 25 lines, then stops. mise's `## Quickstart` is four
  numbered steps ending in "Where to go next".
- *A link to generated reference.* mise ships `docs/cli/` with one Markdown file per
  command plus an `index.md`, and the README only links to `mise.jdx.dev/cli/`.

**"Where to go next" is an explicit routing device, not an implied one.** mise has a
literal two-column table — "I want to… / Read" — with six rows covering first setup,
adding to an existing project, configuration, CI, CLI reference/troubleshooting, and
contributing. cli/cli's `docs/README.md` uses the same shape ("Task / Guide", nine
rows) for developers. ripgrep, despite being an over-long README, opens with
"Documentation quick links" for the same reason. This is the single most transferable
pattern found for a tool with a wide surface.

**Install is a stub that links, once the number of platforms exceeds a handful.**
cli/cli gives three linked per-OS headings with 2–3 bullets each and pushes the rest to
`docs/install_macos.md`, `docs/install_linux.md`, `docs/install_windows.md`, explicitly
splitting official from "community-supported". caddy: one sentence ("download from
GitHub Releases and place it in your PATH") plus a link. mise: one `curl` line plus a
link to the installation guide. The READMEs that kept it inline are exactly the ones
that blew up — lazygit ~300 lines, ripgrep ~215, yq ~270.

**"Where this is going" is usually absent, and where present it is scope, not
roadmap.** None of the 15 has a `## Roadmap` heading. What they have instead:
- tailscale's `## Overview` states *what is and is not in this repo* (daemon and CLI
  yes, mobile GUI no) and links a page explaining which parts are open source and why.
- portainer's `## Intended use` says plainly that CE is for homelabs and "is not
  intended for, or supported in, business, production, or any other environment where
  uptime, security posture, or data integrity matter", and that there is "no roadmap
  commitment".
- home-manager has `Words of warning` as a top-level section and tells the reader in
  the second paragraph to go read it before using the tool.
- beszel's `## Architecture` is four lines that name the two components (hub, agent)
  and what each does — enough for a reader to picture the deployment.

**Trust signals cluster into five recurring items.**
- *Security policy*, as a section or a root file: portainer `## Security` → SECURITY.md;
  ripgrep `### Vulnerability reporting`; beszel, yq, tailscale, fzf all ship SECURITY.md.
- *Release verification*: cli/cli devotes a subsection to it, showing both
  `gh at verify` and a `cosign verify-blob-attestation` invocation against the Sigstore
  attestation. restic's `# Reproducible Builds` states that binaries since 0.6.1 are
  byte-reproducible and links the builder repo.
- *Design principles in prose*: restic's `# Design Principles` is five named
  principles (Easy, Fast, Verifiable, Secure, Efficient), each a short paragraph; the
  Secure one states the threat model — the storage location "is assumed not to be a
  trusted environment".
- *Support expectations, stated bluntly*: portainer ("no official support channel…
  voluntary, best-effort"), caddy's `## Getting help` (advises companies to secure a
  support contract before help is needed; issue tracker is for bugs, not support),
  mise (issues are closed, use Discussions).
- *License*, always, and always last or near-last: 12 of the 15 have a `License`
  section at the end. Standard Readme lists License as Required.

**The README names its own audience split.** cli/cli's `## Documentation` is two
clauses: install below, usage in the manual. Its in-repo `docs/README.md` opens
"Shared guidance for anyone developing or reviewing `gh`. For installation and usage,
see the GitHub CLI manual." chezmoi's 17-line README does the same in two lines: users
→ chezmoi.io, contributors → the developer guide.

**Sibling repos and the boundary of this repo get stated early when the project is
bigger than one repo.** tailscale lists four sibling repos in the Overview. pi-hole
links out to `pi-hole/ftl` and `pi-hole/web` under `## Breakdown of Features`. caddy
names `caddyserver/website` as the docs source and `xcaddy` as the build tool.

---

## 3. Anti-patterns observed (READMEs that became manuals)

- **junegunn/fzf — 41,829 bytes, 1,153 lines.** The largest measured. It contains full
  shell-integration setup for bash, zsh, fish and Nushell, `_fzf_comprun` customization,
  preview-window internals and image previewing. It *does* have `ADVANCED.md`,
  `BUILD.md` and `README-VIM.md` at the repo root, so the material has somewhere to go;
  it just did not go there. Also: a merchandise banner occupies lines 13–22.
- **jesseduffield/lazygit — 38,288 bytes, 643 lines.** ~50 lines of sponsor banners
  before any project content; then an in-file `## Table of contents`; then ~300 lines of
  installation, with a `###` heading per package manager (Homebrew, MacPorts, Void,
  Scoop, Arch, Fedora, Solus, Debian, Funtoo, Gentoo, openSUSE, NixOS, Flox, FreeBSD,
  Termux, Conda, Go, Chocolatey, Winget, Manual…). The repo already has `docs/` with an
  index (`docs/README.md`) listing Config, Custom Commands, Keybindings, Undo/Redo,
  Range Select, Searching, Stacked Branches — none of the install content was moved
  there.
- **mikefarah/yq — 18,338 bytes, 480 lines.** `## Install` plus `## Community
  Supported Installation methods` together run roughly from line 76 to line 348 —
  more than half the file — while the actual usage reference is a single linked
  heading pointing at gitbook.
- **BurntSushi/ripgrep — 21,599 bytes, 541 lines.** Installation runs lines 233–450.
  Notably this README is *self-aware*: it opens with "Documentation quick links" and
  the second entry is `GUIDE.md`, so the split exists — the README simply also keeps
  the long-form content.
- **caddyserver/caddy — 12,661 bytes, 225 lines**, and mostly disciplined, but
  `## Build from source` is ~60 inline lines including `setcap`, a `sudoers` snippet,
  and an inline apology ("We are only qualified to document how to use Caddy, not Go
  tooling or your computer"). That is the shape of a section that should have moved to
  the docs site.
- **charmbracelet/bubbletea — 14,892 bytes, 402 lines.** `## Tutorial` +
  `## Initialization` are an entire Elm-architecture walkthrough inside the README —
  a Diátaxis *tutorial* living in an overview document.

The size threshold implied by the data: the READMEs that read as overviews are all
**under ~9 KB / ~210 lines**; every one over ~12 KB has an identifiable section that
belongs elsewhere. GitHub's own guidance states the intent directly: "A README should
only contain information necessary for developers to get started using and contributing
to your project. Longer documentation is best suited for wikis."

---

## 4. How the in-repo docs trees are laid out, and how Diátaxis maps onto them

**cli/cli — `docs/` is developer documentation, with a task-routing index.**
23 entries. `docs/README.md` is titled "Developing GitHub CLI" and its body is a
"Task | Guide" table: find the source package → `project-layout.md`; design command
syntax → `command-line-syntax.md` + `primer/`; implement commands → `command-development.md`;
write tests → `testing.md`; API clients and hosts → `api-and-hosts.md`; release →
`releasing.md` / `release-process-deep-dive.md`. The per-OS install pages
(`install_macos.md`, `install_linux.md`, `install_windows.md`, `install_source.md`) sit
in the same folder and are the targets of the README's Installation stubs. Diátaxis
read: `install_*.md` are **how-to**, `command-line-syntax.md` and `project-layout.md`
are **reference**, `gh-vs-hub.md` is **explanation**; there is no tutorial, because the
tutorial is the hosted manual.

**restic — `doc/`, numbered files, Sphinx.** `index.rst` plus `010_introduction`,
`020_installation`, `030_preparing_a_new_repo`, `040_backup`, `045_working_with_repos`,
`047_tuning_parameters`, `050_restore`, `060_forget`, `070_encryption`, `075_scripting`,
`077_troubleshooting`, `080_examples`, `090_participating`, `100_references`,
`110_talks`, plus `design.rst`, `faq.rst`, `developer_information.rst`, `man/`. The
numeric prefix enforces reading order. Diátaxis read: 010–050 are a **tutorial**
sequence; 060/075/077 are **how-to**; `100_references` and `man/` are **reference**;
`design.rst`, `cache.rst` and the README's own Design Principles are **explanation**.
This is the cleanest mapping of the set, and the numbering is what makes it visible.

**mise — `docs/` is the published site, with generated per-command reference.**
`docs/index.md`, `getting-started.md`, `installing-mise.md`, `configuration.md` +
`configuration/`, `dev-tools/`, `environments/`, `tasks/`, `faq.md`,
`troubleshooting.md` (linked from the README), `architecture.md`, `security.md`,
`paranoid.md`, `sandboxing.md`, `glossary.md`, and `docs/cli/` with one `.md` per
command (`install.md`, `doctor.md`, `run.md`, `settings.md`, …) plus subfolders for
commands with subcommands, plus `index.md`. Diátaxis read: `getting-started.md` =
**tutorial**; `continuous-integration.md`, `ide-integration.md`, `dotfiles.md` =
**how-to**; `docs/cli/` and `settings.toml` = **reference**; `architecture.md`,
`security.md`, `paranoid.md`, `mise-en-place.md` = **explanation**. `docs/cli/`
following the command tree is exactly Diátaxis's reference advice: "the structure of
the documentation should mirror the structure of the product."

**lazygit — `docs/` as flat topics with an index.** `docs/README.md` ("Documentation
Overview") is a nine-item bullet list; files are topic-named in Title_Case
(`Config.md`, `Custom_Command_Keybindings.md`, `Stacked_Branches.md`, `Undoing.md`,
`Range_Select.md`, `Searching.md`), with `docs/dev/` and `docs/keybindings/` as
subfolders. Flat, no ordering, no category split.

**chezmoi — the docs site lives inside the repo.** `assets/chezmoi.io/` holds
`mkdocs.yml` and `docs/` with `quick-start.md`, `install.md.tmpl`,
`what-does-chezmoi-do.md`, `why-use-chezmoi.md`, `migrating-from-another-dotfile-manager.md`,
`comparison-table.md`, and folders `user-guide/`, `reference/`, `developer-guide/`.
Diátaxis read: the folder names *are* the categories — `reference/` is reference,
`user-guide/` is how-to, `quick-start.md` is the tutorial, `why-use-chezmoi.md` and
`what-does-chezmoi-do.md` are explanation. This is the only repo in the set where the
top-level docs folders map one-to-one onto Diátaxis, and it is also the repo with the
shortest README (547 bytes) — the two go together.

**home-manager — `docs/` holds the manual build.** `manual/`, `release-notes/`,
`mdbook/`, `static/`, plus troff man pages (`home-manager.1`,
`home-configuration-nix-{header,footer}.5`). Reference is generated from module option
declarations, not hand-written.

**The repos with no `docs/`** (caddy, pi-hole, portainer, beszel, yq, ripgrep, fzf,
bubbletea) either keep a separate website repo (caddy → `caddyserver/website`,
pi-hole → docs.pi-hole.net, portainer → docs.portainer.io, beszel → beszel.dev) or use
root-level long-form files (ripgrep: `GUIDE.md`, `FAQ.md`; fzf: `ADVANCED.md`,
`BUILD.md`, `README-VIM.md`).

**Diátaxis definitions used above** (quoted from diataxis.fr):
- Tutorial — "an *experience* that takes place under the guidance of a tutor… always
  **learning-oriented**"; "not the place for explanation".
- How-to guide — "**directions** that guide the reader through a problem or towards a
  result… **goal-oriented**"; "wholly distinct from tutorials".
- Reference — "**technical descriptions** of the machinery and how to operate it…
  **information-oriented**"; should be "austere" and "wholly authoritative", with no
  instruction or opinion.
- Explanation — "a discursive treatment of a subject, that permits *reflection*…
  **understanding-oriented**"; covers "design decisions, historical reasons, technical
  constraints".

Diátaxis's own framing is that it "solves problems related to documentation *content*
(what to write), *style* (how to write it) and *architecture* (how to organise it)".
An overview README is none of the four types — it is the map that routes to them.

**Standard Readme** (spec.md) gives a required order, which is a useful skeleton but is
aimed at "open source libraries": Title (required) → Banner → Badges → Short
Description (required, <120 chars, own line) → Long Description → Table of Contents
(required above 100 lines) → Security → Background → Install (required) → Usage
(required) → Extra Sections → API → Maintainers → Thanks → Contributing (required) →
License (required). Note the TOC rule: required for READMEs over 100 lines — which is
itself an admission that a README over 100 lines needs navigation. The spec also
quotes perlmodstyle: "someone who's slightly familiar with your module should be able
to refresh their memory without hitting 'page down'."

---

## 5. Three candidate skeletons for the ctdev README

All three assume the README stops being the manual and a `docs/` tree absorbs what it
sheds. All three keep the current audience order (owner on a fresh machine → future
staff → client IT contact) in mind but weight it differently.

### Skeleton A — "The router" (modeled on mise + cli/cli)

Target: **150–200 lines, 6–8 KB.** Longest of the three; keeps one runnable path in
the README.

1. *(no heading)* Name, one-sentence description (<120 chars), badges (release, CI,
   license), one-line nav row: Docs · Releases · Recovery.
2. `## What ctdev is` — 4–6 linked bullets: components, profiles, doctor, backups,
   fleet (marked as in progress). Each bullet links to its docs page. ~12 lines.
3. `## Quickstart` — numbered: install the binary (the one `curl` line), `ctdev apply
   dev-workstation`, `ctdev status`. Stop there. ~25 lines.
4. `## Where to go next` — the two-column "I want to… / Read" table: set up a fresh
   machine, build a Pi-hole node, build an AI/MCP node, set up backups, diagnose a
   machine I don't manage, restore from a snapshot, add a component, contribute.
   ~12 rows. This is the load-bearing section.
5. `## What it does to your machine` — short prose: what needs root and when, where
   files land (`/etc/ctdev`, `~/.local/bin`, `~/<stack>/`), what is never committed,
   that `doctor` is read-only and never writes. Links to `docs/security.md`. ~15 lines.
6. `## Getting help / support` — states what is supported and by whom.
7. `## License`.

Docs layout:
```
docs/
  README.md              index: task → guide table (cli/cli's docs/README.md shape)
  getting-started.md     tutorial: laptop from zero
  install.md             the binary, all OSes, verification
  profiles.md            what a profile is, the four built-ins, writing your own
  components/README.md   the component index (one row per component)
  components/<name>.md   one page per non-trivial component (pihole, caddy, restic,
                         brain, mcp-email-server, beszel, portainer)
  doctor.md              what each check does, what --root/--deep unlock
  backups.md             + RECOVERY.md stays at root (disaster docs must be findable)
  cli/<command>.md       generated reference, one per command, + cli/index.md
  security.md            threat model, secrets, what is stored where
  architecture.md        explanation: registry, phases, Root modes, sysutil
  fleet.md               where this is going (agent, API, dashboard)
```
Trade-off: the most work up front (a real docs tree, ~15 files), and the README still
carries a quickstart that can drift. Best for the future-staff reader, because the
task table is the on-ramp and the per-command reference is generated.

### Skeleton B — "The front door" (modeled on tailscale + beszel + portainer)

Target: **70–100 lines, 3–4 KB.** No command examples at all except the install line.

1. *(no heading)* Name, one-line description, badges.
2. `## Overview` — 6–10 lines: what ctdev manages, what is *in* this repo vs what is
   not (the `AI` repo owns Claude config; `dotfiles` owns the machine), and the shape
   of a managed node.
3. `## Architecture` — 5 lines naming the pieces: the single binary, the component
   registry, machine profiles, the read-only doctor, and (marked clearly as not yet
   shipped) the agent/API/dashboard.
4. `## Getting started` — three links: install, quickstart, profiles. Plus the single
   `install.sh` line, because a fresh machine has no docs open.
5. `## Screenshots` — the `configure` TUI and a `doctor` report, if worth showing.
6. `## What it touches on your machine` — the trust section, as in A.
7. `## Support and scope` — portainer-style plain statement of what this is and is not
   intended for, and who to contact.
8. `## Security` → SECURITY.md · `## Contributing` · `## License`.

Docs layout: same as A, but `docs/getting-started.md` carries everything the README
drops, and `docs/README.md` carries the task table instead of the README.

Trade-off: shortest, ages best, reads most like a product. Costs the owner a round
trip — on a genuinely fresh machine with no browser, the README no longer tells you
what to type beyond installing the binary. Mitigate by keeping `install.sh`'s output
verbose, or by keeping one `ctdev apply <profile>` line in step 4.

### Skeleton C — "Overview plus trust dossier" (modeled on restic + cli/cli)

Target: **120–150 lines, 5–6 KB.** Pitched at the client IT contact first.

1. *(no heading)* Badges, name, 2-sentence description, link to docs.
2. `## Quick start` — a single copy-pasteable transcript (install → `apply` →
   `status`), restic-style, then stops.
3. `## What it manages` — linked bullet list of component categories and profiles.
4. `## Design principles` — restic's shape: 4–5 named principles with a short
   paragraph each. Candidates from the existing codebase's actual invariants:
   *Read-only by default* (doctor never writes, never requires root, prompts nobody);
   *Secrets never in the repo* (entered at configure time, stored only on the host that
   needs it, `${VAR}` references); *Declarative and diffable* (`apply` / `diff`, exit
   non-zero on drift); *Never silently replace* (install conflicts are probed and
   stated); *Reversible* (uninstall keeps data; RECOVERY.md).
5. `## What it changes on your machine` — the concrete list: systemd units, `/etc`
   files, `~/<stack>/` compose dirs, package installs, when root is used.
6. `## Release verification` — checksum/`SHA256SUMS` verification, and signing if it
   exists (cli/cli's shape). *Unverified: whether ctdev releases are currently signed
   or attested — that was not checked as part of this research.*
7. `## Documentation` — link to `docs/README.md`, plus RECOVERY.md and
   TROUBLESHOOTING.md called out by name.
8. `## Support` · `## License`.

Docs layout: same tree as A; additionally move the current README's long node recipes
(Pi-hole node, AI/MCP node) verbatim into `docs/nodes/pihole.md` and
`docs/nodes/ai-node.md`, since they are already how-to guides wearing a README's
clothes.

Trade-off: the trust material is the point, which suits the "client's IT contact
deciding whether to trust it" reader and matches an ambition to sell consulting. It is
the least conventional of the three for a CLI, and the Design Principles section needs
to stay true as the tool changes or it becomes marketing.

**Cross-cutting notes for any of the three**
- Every skeleton assumes the ~270 lines of node recipes currently in the README move
  out. They are the single largest block and the clearest Diátaxis how-to.
- The component list (53 names) belongs in `docs/components/README.md` as a table, not
  in the README. No studied repo lists its full surface in the README.
- A `docs/README.md` index is worth having on day one even if it links only four files
  — lazygit and cli/cli both show the index is what makes the folder navigable.
- If the fleet platform is announced in the README at all, follow tailscale: say what
  is in this repo and what is not, rather than a roadmap.

---

## 6. Sources

All fetched 2026-09-18.

READMEs (`gh api repos/<repo>/readme --jq .content | base64 -d`):
- cli/cli · restic/restic · junegunn/fzf · BurntSushi/ripgrep · charmbracelet/bubbletea
- jesseduffield/lazygit · twpayne/chezmoi · mikefarah/yq · tailscale/tailscale
- caddyserver/caddy · pi-hole/pi-hole · henrygd/beszel · jdx/mise · portainer/portainer
- nix-community/home-manager

Directory listings (`gh api repos/<repo>/contents/<path>`):
- `cli/cli/contents/docs` · `cli/cli/contents/docs/README.md`
- `restic/restic/contents/doc`
- `jdx/mise/contents/docs` · `jdx/mise/contents/docs/cli`
- `jesseduffield/lazygit/contents/docs` · `jesseduffield/lazygit/contents/docs/README.md`
- `junegunn/fzf/contents/` · `BurntSushi/ripgrep/contents/` · `henrygd/beszel/contents/`
- `tailscale/tailscale/contents/` · `tailscale/tailscale/contents/docs`
- `caddyserver/caddy/contents/` · `pi-hole/pi-hole/contents/` · `portainer/portainer/contents/`
- `twpayne/chezmoi/contents/` · `twpayne/chezmoi/contents/assets/chezmoi.io` ·
  `twpayne/chezmoi/contents/assets/chezmoi.io/docs`
- `mikefarah/yq/contents/` · `nix-community/home-manager/contents/` ·
  `nix-community/home-manager/contents/docs` · `charmbracelet/bubbletea/contents/`

Structure sources:
- https://diataxis.fr/ — four types and the content/style/architecture framing
- https://diataxis.fr/tutorials/ — tutorial definition
- https://diataxis.fr/how-to-guides/ — how-to definition
- https://diataxis.fr/reference/ — reference definition; "structure of the
  documentation should mirror the structure of the product"
- https://diataxis.fr/explanation/ — explanation definition
- `gh api repos/github/docs/contents/content/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-readmes.md`
  — GitHub, "About the repository README file": what READMEs typically include; README
  lookup order (`.github`, root, `docs`); 500 KiB truncation; "A README should only
  contain information necessary for developers to get started…"
- `gh api repos/RichardLitt/standard-readme/contents/spec.md` — required/optional
  section order and the Short Description and Table of Contents rules
- Local: `/home/thomas/Repos/github.com/ConnerTechnology/dotfiles/README.md` (24,021 B,
  446 lines) and the repo root listing

Could not verify:
- Whether ctdev release artifacts are cryptographically signed or attested (only
  `SHA256SUMS` verification is mentioned in the repo's CLAUDE.md; not checked against
  CI config).
- Star counts / "well-regarded" as a measured quantity — repos were chosen for
  comparability and reputation, not ranked by any fetched metric.
- Diátaxis quotes came through a summarizing fetch of each page rather than the raw
  HTML; the wording in quotation marks matches the pages' own phrasing as returned, but
  was not diffed character-by-character against the source HTML.
