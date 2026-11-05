---
paths:
  - "install.sh"
  - "install.ps1"
---
# Install

`install.sh` installs only the `ctdev` binary, from the latest release or the `vX.Y.Z` tag in
`CTDEV_VERSION`; there is no source-build path and no all-in-one machine bootstrap, so a machine
is composed from individual `ctdev install <component>` and `ctdev configure <category>` calls
(README "Fresh Machine Setup"). `install.ps1` is the Windows equivalent, where ctdev runs `doctor` only.

The two installers offer the same modes: install, or a one-off doctor run with its arguments
passed through. `install.sh` accepts `--doctor [doctor args]` and `-h`/`--help`; `install.ps1`
accepts `-Doctor` and `-DoctorArgs`.

`install.sh` is also what `ctdev update` runs to update itself (`ctdev/cmd/update_apply.go`): it
fetches the script from the new release's tag and passes `INSTALL_DIR` (the running binary's
directory) and `CTDEV_VERSION`. A release's `install.sh` must keep honoring both, and must not treat
its own `INSTALL_DIR` as an old install to remove. Root boot scripts run it on every boot, so the
"already at this version, exit 0" short-circuit has to keep running before any download.
