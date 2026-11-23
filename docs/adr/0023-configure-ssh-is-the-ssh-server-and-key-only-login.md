---
status: accepted
date: 2026-11-23
---

# `configure ssh` is the SSH server and key-only login

`ctdev configure ssh` has two settings: the SSH server and key-only login. Thomas decided on
2026-11-23 in CTD-178 to remove the other four: keep my programs running after I disconnect
(systemd lingering), Mosh support (the `en_US.UTF-8` locale), open this machine from vscode.dev
(the VS Code tunnel), and only allow SSH and Mosh from my local network (UFW).

He connects over the home LAN or Tailscale with plain SSH and tmux. Tailscale keeps each
machine's address stable, and WireGuard follows the laptop across networks, so Mosh's roaming
isn't needed. tmux keeps a session through a connection that drops after a long sleep. Ubuntu
doesn't kill user processes at logout, so tmux doesn't need lingering. `ctdev install mosh`
stays as a component.

## Consequences

- A removed setting that ctdev turned on would stay on with no off switch, so a run of
  `ctdev configure ssh` or a bare `ctdev configure` that can change the machine (not `--show`)
  runs the old off path of each one ctdev turned on (`setup.RetireRemovedSSHSettings`). After
  a pass with no failure it records that in ctdev's state directory and never runs again.
- The locale has a marker in ctdev's state directory and the firewall has ctdev's six LAN allow
  rules, so the cleanup touches only what ctdev added. UFW stays on if other rules remain.
- Lingering and the tunnel record no owner. Both defaulted to on in `--batch`, the tunnel
  wherever VS Code was installed, so the cleanup treats them as ctdev's and turns them off
  wherever they are on. Someone who turned either on by hand turns it back on once, and the
  cleanup, having run, leaves it alone.
