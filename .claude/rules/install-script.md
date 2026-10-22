---
paths:
  - "install.sh"
  - "install.ps1"
---
# Install

`install.sh` installs only the `ctdev` binary from the latest release; there is no source-build
path and no all-in-one machine bootstrap, so a machine is composed from individual
`ctdev install <component>` and `ctdev configure <category>` calls (README "Fresh Machine
Setup"). `install.ps1` is the Windows equivalent, where ctdev runs `doctor` only.

The two installers offer the same modes: install, or a one-off doctor run with its arguments
passed through. `install.sh` accepts `--doctor [doctor args]` and `-h`/`--help`; `install.ps1`
accepts `-Doctor` and `-DoctorArgs`.
