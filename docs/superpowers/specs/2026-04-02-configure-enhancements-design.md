# Configure Enhancements — Design Spec

## Overview

Enhance `ctdev configure` with a full TUI for git configuration (SSH key picker, GitHub integration), add AWS profile configuration, and auto-offer the `lc` alias when colorls is installed.

## 1. `ctdev configure git` — Enhanced TUI

### Scope detection

- If CWD contains `.git/`: prompt "Configure for this repo or globally?" with a two-option picker
- If no `.git/`: default to global, no prompt
- `--local` flag forces local, `--global` flag forces global (both skip the prompt)

### Interactive flow

1. **Scope** (only if in git repo and no scope flag): picker with "Global" / "This repo only"
2. **Name**: text input, pre-filled with current `git config user.name`
3. **Email**: text input, pre-filled with current `git config user.email`
4. **SSH Signing Key**: picker listing:
   - Each `~/.ssh/*.pub` file found on the system (show the filename and key type)
   - "Generate new ed25519 key"
   - "Enter custom path"
   - "Skip (no signing)"
5. If **"Generate new key"** selected:
   - Run `ssh-keygen -t ed25519 -C <email> -f ~/.ssh/id_ed25519` (prompts for passphrase via stdin passthrough)
   - Display the public key content for the user
6. If **key selected or generated** and `gh auth status` succeeds:
   - Prompt: "Add this key to GitHub? [Y/n]"
   - If yes: `gh ssh-key add <pubkey-path> --title "ctdev-<hostname>"` 
   - If `gh` not installed or not authenticated: print the public key and instructions:
     ```
     Add this key to GitHub:
       1. Go to https://github.com/settings/ssh/new
       2. Title: ctdev-<hostname>
       3. Key type: Signing Key
       4. Paste your public key
     ```
7. **Apply all settings**: `git config <scope> user.name/email/signingKey`

### Non-interactive mode

With flags: `ctdev configure git --name "..." --email "..." --signing-key ~/.ssh/id_ed25519.pub`

Batch mode: requires all flags, no prompts.

### `--show` enhanced

Display signing key path and whether `gh ssh-key list` shows it on GitHub.

### Implementation

- Replace the current `fmt.Scanln` prompts in `cmd/configure.go` with a Bubble Tea model
- New file: `ctdev/tui/configure/git.go` — the TUI model
- SSH key scanning: `filepath.Glob(home + "/.ssh/*.pub")`
- Key generation: `exec.Command("ssh-keygen", ...)` with stdin/stdout passthrough
- GitHub integration: `exec.Command("gh", "auth", "status")` to check, `exec.Command("gh", "ssh-key", "add", ...)` to upload

## 2. `ctdev configure aws`

### Interactive flow

1. Parse `~/.aws/config` for `[profile <name>]` sections (also handle `[default]`)
2. If no profiles found: print error with instructions to set up AWS CLI first
3. Show Bubble Tea picker with profile names
4. Write or update `export AWS_PROFILE=<selected>` in `~/.oh-my-zsh/custom/exports.local.zsh`:
   - If file has existing `AWS_PROFILE` line: replace it
   - Otherwise: append after a blank line
5. Print success message: "AWS_PROFILE set to <name>. Restart your shell or run: source ~/.oh-my-zsh/custom/exports.local.zsh"

### Non-interactive mode

`ctdev configure aws --profile <name>` — writes directly, no picker.

### Implementation

- New Cobra subcommand under `configureCmd`
- Profile parsing: read `~/.aws/config`, regex for `\[profile (.+)\]` and `\[default\]`
- Reuse existing picker TUI or simple list
- `exports.local.zsh` editing: read file, find/replace `AWS_PROFILE` line, write back using `sysutil.DeployFile` pattern (but in-place edit, not full replace — only the AWS_PROFILE line changes)

## 3. Ruby + colorls alias

### Flow

After the ruby component installer successfully installs the `colorls` gem:

1. Check if `lc` alias already exists in `~/.oh-my-zsh/custom/exports.local.zsh`
2. If not present AND not in batch mode:
   - Prompt: "Add alias 'lc' for colorls to your shell? [Y/n]"
   - Default: Y
   - If yes: append `alias lc='colorls -lA --sd'` to exports.local.zsh
3. If batch mode: skip silently

### Implementation

- Modify `ctdev/component/ruby.go` after the `gem install colorls` step
- Helper function: `appendToExportsLocal(line string) error` in component package or sysutil
- Check for existing line with `strings.Contains` before appending

## 4. `exports.local.zsh` template update

Update the template in `ctdev/component/configs/zsh/exports.local.zsh` to include helpful comments:

```zsh
# Local/personal environment variables
# This file is per-machine — edit freely, it won't be overwritten.
#
# Examples:
# export AWS_PROFILE=my-profile
# export GOPRIVATE='github.com/yourorg'
# alias lc='colorls -lA --sd'
```

## Out of Scope

- Full AWS CLI setup/installation (just profile selection)
- GitHub CLI authentication (just using existing auth)
- GPG key management (SSH signing only)
- Multiple signing keys per scope
