---
paths:
  - "ctdev/component/aws*.go"
  - "ctdev/component/configs/aws/**"
  - "ctdev/cmd/configure_aws*.go"
---
# AWS CLI

- `ctdev install aws` installs AWS CLI v2 (CTD-90 decision 3), never from apt: Ubuntu's
  `awscli` package is v1 on several releases.
- **Detection is v2-aware in one place:** the registry entry's `Detect` hook,
  `awsV2Installed`, wants an `aws` on PATH whose `aws --version` starts `aws-cli/2.`.
  `IsInstalled`, `InstallNeedsRoot`, the installer's skip and `configure aws` all go
  through it, so apt's v1 counts as missing and installing over it caches sudo first.
- **Linux:** the zip from `awscli.amazonaws.com` for amd64 or arm64, its `.sig`
  verified with `gpg` in a throwaway `--homedir` against the embedded AWS CLI Team key
  `ctdev/component/configs/aws/aws-cli-public-key.asc` (fingerprint
  `FB5D B77F D5C1 18B8 0511 ADA8 A631 0ACC 4672 475C`), then
  `sudo ./aws/install --bin-dir /usr/local/bin --install-dir /usr/local/aws-cli`, with
  `--update` when `/usr/local/aws-cli` already exists. `gnupg` and `unzip` are
  installed first when missing. Other OSes are unsupported, for uninstall too.
- **The key expires 2027-07-01.** When AWS publishes a renewed key, replace the `.asc`
  with the block from AWS's install page, verbatim, and check the fingerprint
  (`TestAWSEmbeddedKeyIsAWSCLITeam`).
- **macOS:** the `awscli` Homebrew formula. AWS's docs don't cover Homebrew; it's the
  route Thomas chose in CTD-90 decision 3 ("the pkg or Homebrew").
- Uninstall follows AWS's documented steps: the `aws` and `aws_completer` symlinks and
  `/usr/local/aws-cli`. `~/.aws` stays, since every AWS SDK shares it.
- `ctdev update` doesn't update the Linux install; `ctdev install aws --force` does.
- With no profiles in `~/.aws/config`, `configure aws` offers `aws configure sso`
  (`offerAWSConfigureSSO` in `ctdev/cmd/configure_aws.go`); under `--batch` it only
  prints the hint.
- Source of the Linux URLs and steps above:
  https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html,
  `getting-started-version.html` (the `--update` flag) and `uninstall.html`, checked
  2026-10-18.
