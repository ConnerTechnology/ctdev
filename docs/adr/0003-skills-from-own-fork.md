# Skills from our own fork

The engineering skills live in a private `thomasconner/skills` repo, a de-branded fork of
`mattpocock/skills`. The template ships only `setup-skills`, which installs the rest with
`npx skills add thomasconner/skills --copy`, so each repo commits real files plus a
`skills-lock.json` that points at the fork. A skill fix is one commit in the fork, and a repo takes
it with `npx skills update` when someone chooses to, reviewed as a diff.

## Considered options

Copying the skills into the template gave no way to fix an existing repo short of editing it by
hand. Packaging the fork as a plugin updated without a diff and put the skills out of reach of
`skillOverrides`.

## Consequences

The fork must be a real skills source: skills under `skills/<bucket>/<name>/`, and no
`skills-lock.json` that attributes them to another repo. The CLI skips skills a lock file records as
installed from elsewhere (tested 2026-09-23: it listed 2 of 40 skills in `thomasconner/finances`).
Private repos install fine over HTTPS or SSH with the local `gh` credentials.

Matt Pocock's MIT license requires his copyright and permission notice in all copies, so the fork
keeps it, and every repo that installs from the fork carries it in `THIRD_PARTY_LICENSES.md`.
