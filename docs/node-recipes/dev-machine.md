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

# Linux (after vscode, so the ssh step can set up the VS Code tunnel)
ctdev install earlyoom smartmontools
ctdev configure storage --batch  # weekly TRIM, a swapfile only where there is no swap
ctdev configure ssh --batch      # SSH server, key-only login, Mosh, keep programs
                                 # running, VS Code tunnel, LAN-only firewall

# everything else
ctdev install                    # pick from the list
```

`claude-code` pulls in `ccstatusline`, which pulls in `node`, so Node.js comes
with the base. Other runtimes (`go`, `python`, `ruby`) and role-specific tools
(cloud CLIs, Kubernetes, infrastructure stacks) are picked afterward. On Linux,
`chrome` and `1password` install only on amd64.

**Manual steps**

- `gh auth login`
- Join the tailnet: `sudo tailscale up` on Linux; on macOS, sign in from the
  Tailscale menu bar app
- Linux: add your client's SSH public key to `~/.ssh/authorized_keys`, then
  re-run `ctdev configure ssh --batch` (password login turns off only once a
  key is present), and authenticate the VS Code tunnel once with
  `code tunnel user login`
- If the install ran with `--batch`, run `ctdev configure git` for your name,
  email and signing key
