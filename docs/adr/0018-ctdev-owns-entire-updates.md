---
status: accepted
date: 2026-11-19
---

# ctdev installs Entire and owns its updates; telemetry stays with each repo

The `entire` component installs the Entire CLI, `entire` and `git-remote-entire`. On Linux they
come from the release archive, checked against the release's `checksums.txt`, into `~/.local/bin`.
On macOS they come from the Homebrew cask `entireio/tap/entire`. `ctdev update` keeps them
current, and the component turns off Entire's own update check. It leaves telemetry, sign-in and
every repo's Entire settings alone. Decided on CTD-159.

- **`~/.local/bin` on Linux.** It is where Entire's own installer and its direct-download steps
  put both binaries, and where ctdev and restic already live. No root is needed, and a hand
  install already there is the same files in the same place. ctdev counts it as installed and
  replaces it in place on update, so no second copy appears. Another `entire` elsewhere on PATH
  gets a one-line hint and is left alone. On macOS the cask's copy is the install. Every
  `ctdev install entire` with the cask in place removes a copy that `install.sh` left in
  `~/.local/bin`, if it answers as Entire. Otherwise it would shadow the cask, and with Entire's
  updater off it would never update.
- **Two origins.** GitHub first, then Entire's mirror at `install.entire.com`, the same order and
  layout (`<base>/v<version>/<file>`) as Entire's `install.sh`. The archive and its checksums
  always come from the same origin. An origin that can't serve the files, or whose `checksums.txt`
  doesn't list the archive, is skipped. A checksum mismatch stops the install rather than trying
  the mirror, because bytes that disagree with their own origin's checksums mean that origin is
  broken.
- **One updater.** When Entire's update prompt is accepted, Entire runs `install.sh` or
  `brew upgrade` itself. That would be a second updater that ctdev's scan can't see. The
  component sets `"no_auto_update": true` in Entire's `config.json` (`ENTIRE_CONFIG_DIR`, or
  `~/.config/entire`) and keeps the file's other keys. The file is used rather than
  `ENTIRE_NO_AUTO_UPDATE`, because every invocation reads it, including git hooks started by apps
  that never load a shell profile. A file that isn't valid JSON is left as it is.
- **No machine-wide telemetry opt-out.** Entire's analytics are off while the `telemetry` setting
  is unset. The setting lives in each repo's `.entire/` files, and `entire enable` asks for consent
  when a repo is set up. So the choice belongs to each repo, like the rest of its Entire setup.
  This repo sets `"telemetry": false`.
- **ctdev's own calls send nothing.** When ctdev runs `entire version` (doctor, the update scan),
  it sets `ENTIRE_TELEMETRY_OPTOUT=1` and `ENTIRE_NO_AUTO_UPDATE=1`. Those runs are ctdev reading a
  version, not someone using Entire, and doctor sends nothing beyond its probes (ADR-0012).

## Considered options

- **`ENTIRE_NO_AUTO_UPDATE=1` in the shell config.** Rejected: hooks run by a GUI app or an agent
  don't read `~/.zshrc`, so Entire's prompt would still appear there.
- **`ENTIRE_TELEMETRY_OPTOUT=1` for every shell.** Rejected for the same reason, and because it
  would override a repo that chose to send analytics.
- **Run Entire's `install.sh`.** Rejected. ctdev would then fetch and run a script on every
  install, and its update scan would still need its own version check.
- **`/usr/local/bin`, like herdr and git-spice.** Rejected. It needs root and would leave a hand
  install in `~/.local/bin` shadowing it on PATH.

## Consequences

- `entire enable`, `entire login` and `entire configure` stay per-repo steps
  (`docs/agents/goal-loop.md`, "Entire").
- doctor's row reports the `entire` first on PATH, which is the one git hooks run. The update
  scan reads the copy ctdev manages, and the install's hint names any other copy.
- Uninstalling removes the two binaries only. Login contexts and credentials in
  `~/.config/entire`, and each repo's `.entire/` and checkpoints, stay.
