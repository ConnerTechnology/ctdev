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

The two installers take the same options and change together: `install.sh` accepts
`--doctor [doctor args]` and `-h`/`--help`; `install.ps1` accepts `-Doctor` and `-DoctorArgs`.
