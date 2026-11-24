# Dev machine

The base every development machine gets, on Linux or macOS. It is a list of
`install` and `configure` steps, not a profile: once it has run, pick whatever
else this machine needs from `ctdev install`.

```bash
# after installing ctdev — see ../install.md

# every machine (installing git asks for your name, email and signing key)
ctdev install zsh git gh fonts tmux mosh jq ripgrep fd fzf bat btop direnv \
  zoxide shellcheck ffmpeg docker tailscale claude-code

# a machine with a display
ctdev install 1password chrome vscode
ctdev configure keyboard --batch
ctdev configure desktop --batch  # Dock and Finder on macOS, Nemo on Cinnamon

# Linux
ctdev install earlyoom smartmontools
ctdev configure storage --batch  # weekly TRIM, a swapfile only where there is no swap
ctdev configure ssh --batch      # SSH server, key-only login

# everything else
ctdev install                    # pick from the list
```

`claude-code` pulls in `ccstatusline`, which pulls in `nodenv`. When no global
Node version is set yet, the first npm-based component installs the latest LTS and
makes it global, so Node comes with the base. `ctdev configure node` picks
other versions, globally or per project. Other runtimes (`go`,
`python`, `ruby`) and role-specific tools (cloud CLIs, Kubernetes,
infrastructure stacks) are picked afterward. On Linux, `chrome` and
`1password` install only on amd64.

**Manual steps**

- `gh auth login`
- Join the tailnet: `sudo tailscale up` on Linux; on macOS, sign in from the
  Tailscale menu bar app
- Linux: add your client's SSH public key to `~/.ssh/authorized_keys`, then
  re-run `ctdev configure ssh --batch` (password login turns off only once a
  key is present)
- If the install ran with `--batch`, run `ctdev configure git` for your name,
  email and signing key
