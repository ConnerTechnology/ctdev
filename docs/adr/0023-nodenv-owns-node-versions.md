---
status: accepted
date: 2026-11-23
---

# ctdev no longer chooses or bumps Node versions; the user does, through nodenv

The `nodenv` component (formerly `node`) installs nodenv and node-build and no Node version.
`ctdev update` pulls the nodenv and node-build clones on Linux and never installs a Node or runs
`nodenv global`. Which Node runs, for every folder or for one project, is the user's call through
`ctdev configure node install`, `update` and `uninstall`. Decided on goal node-versions, which has
no Linear issue.

Before this, `ctdev update` installed the newest even major and made it global. Every global npm
package stayed behind on the old version, so `claude`'s status line, `codex` and `gemini` stopped
resolving. The `node` component pinned 24.0.0 and ran `nodenv global 24.0.0` on every install and
`--force`, downgrading the user. "Newest even major" is also not LTS: an even major is "Current"
from April until it enters LTS in October.

- **LTS comes from nodejs.org.** `https://nodejs.org/dist/index.json` marks each release with
  `lts: false` or its codename. ctdev picks the highest major whose newest release is LTS, then
  installs node-build's newest release of that major, since nodenv can only install what
  node-build defines.
- **npm components use the global Node.** They are machine-wide tools, so they install, uninstall
  and rehash under `nodenv global`, whatever `.node-version` or `NODENV_VERSION` says where ctdev
  runs. With no global version, the first one installs that LTS and makes it global. An existing
  global version is never changed, and a global of `system` is the user's choice: the install
  fails and says how to pick a nodenv Node.
- **Update moves packages and deletes the old version.** `configure node update` goes to the newest
  release of the same major. It reinstalls the old version's global npm packages at the same
  versions, then deletes the old version, unless it is still the global version or this folder's
  `.node-version` pins it. Packages linked with `npm link`, and any without a version in their
  `package.json`, can't be reinstalled from the registry: the plan names them as not moved. A pin
  that is not an exact version (`22`, `lts`, an alias) is nodenv's to resolve, so update leaves
  it. Other folders are not scanned, so a project elsewhere pinned to the old version needs
  `configure node install` there.
- **Installed means the global Node has the package.** A nodenv shim stays on PATH after the
  Node it came from stops being global, so an npm component checks the global version's
  `lib/node_modules`, not PATH.

## Considered options

- **Keep bumping the global version in `ctdev update`, and copy packages along.** Rejected: a
  global version is a choice with project-wide effects (native modules, engines fields), and
  update runs unattended with `-y`.
