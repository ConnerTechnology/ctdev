---
paths:
  - "ctdev/component/aws*.go"
  - "ctdev/component/configs/aws/**"
  - "ctdev/cmd/configure_tools*.go"
---
# AWS CLI

- `ctdev install aws` installs AWS CLI v2 from AWS's own installer, never apt: Ubuntu's
  `awscli` package is v1 on several releases, and `aws configure sso` needs v2. A v1
  `aws` on PATH doesn't count as installed (`awsIsV2` checks `aws --version`).
- **Linux:** the zip from `awscli.amazonaws.com` for amd64 or arm64, its `.sig`
  verified with `gpg` in a throwaway `--homedir` against the embedded AWS CLI Team key
  `ctdev/component/configs/aws/aws-cli-public-key.asc` (fingerprint
  `FB5D B77F D5C1 18B8 0511 ADA8 A631 0ACC 4672 475C`), then
  `sudo ./aws/install --bin-dir /usr/local/bin --install-dir /usr/local/aws-cli`, with
  `--update` when `/usr/local/aws-cli` already exists. `gnupg` and `unzip` are
  installed first when missing.
- **The key expires 2027-07-01.** When AWS publishes a renewed key, replace the `.asc`
  with the block from AWS's install page, verbatim, and check the fingerprint
  (`TestAWSEmbeddedKeyIsAWSCLITeam`).
- **macOS:** the `awscli` Homebrew formula.
- Uninstall follows AWS's documented steps: the `aws` and `aws_completer` symlinks and
  `/usr/local/aws-cli`. `~/.aws` stays, since every AWS SDK shares it.
- `ctdev update` doesn't update the Linux install; `ctdev install aws --force` does.
- `configure aws` and `configure git` call `ensureTool` in
  `ctdev/cmd/configure_tools.go`: a missing tool is installed after a [Y/n], or
  without asking under `--batch`. `--show` never installs. With no profiles,
  `configure aws` offers `aws configure sso`; under `--batch` it only prints the hint.
- Source of every URL and step above:
  https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html,
  `getting-started-version.html` (the `--update` flag) and `uninstall.html`, checked
  2026-10-18.
