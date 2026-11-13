# Dev machine

The base every development machine gets, on Linux or macOS. It is a list of
`install` and `configure` steps, not a profile: once it has run, pick whatever
else this machine needs from `ctdev install`.

```bash
# after installing ctdev — see ../install.md

# every machine
ctdev install zsh git gh fonts tmux mosh jq ripgrep fd fzf bat btop direnv \
  zoxide shellcheck ffmpeg docker tailscale claude-code
ctdev configure git              # name, email, signing key (asks)
ctdev configure ssh --batch      # SSH server, key-only login, Mosh, keep programs
                                 # running, LAN-only firewall

# a machine with a display
ctdev install 1password chrome vscode
ctdev configure keyboard --batch
ctdev configure desktop --batch  # Dock and Finder on macOS, Nemo on Cinnamon

# Linux
ctdev install earlyoom smartmontools
ctdev configure storage --batch  # weekly TRIM, a swapfile only where there is no swap

# everything else
ctdev install                    # pick from the list
```

`claude-code` pulls in `ccstatusline`, which pulls in `node`, so Node.js comes
with the base. Other runtimes (`go`, `python`, `ruby`) and role-specific tools
(cloud CLIs, Kubernetes, infrastructure stacks) are picked afterward.

**Manual steps**

- `gh auth login`
- `sudo tailscale up` to join the tailnet
- Add your client's SSH public key to `~/.ssh/authorized_keys`, then re-run
  `ctdev configure ssh --batch` (password login turns off only once a key is present)
