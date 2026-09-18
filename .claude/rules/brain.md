---
paths:
  - "ctdev/component/brain*.go"
  - "ctdev/component/configs/brain/**"
  - "ctdev/cmd/configure_brain.go"
---
# The brain (`brain` component)

`ctdev install brain` provisions **ConnerTechnology/AI** — the agent org — onto an
always-on node and runs its scheduled work there. A git checkout and two systemd
timers; **not** a compose stack. `ctdev configure brain` sets the checkout, the
schedule, and the Claude credential; `--show` reports state.

The reason it exists: a schedule living in a Claude Code session dies with the
window, and two laptops running scheduled agents write the same `memory/` files
and disagree. One always-on writer removes both.

**Paths are an API surface**, chosen so a later service (the tailnet app in the AI
repo's `docs/vision.md`) can find the brain without reading a systemd unit:

```
/srv/brain                          checkout, brain:brain, 2770 (setgid, no world bit)
/var/lib/brain                      state: runs/, brain.lock, .ssh/, the account's $HOME
/etc/ctdev/brain.conf               pointer file — 0644, shell-quoted, NO SECRETS
/etc/ctdev/brain-claude-token.cred  Claude token, host-encrypted
/etc/ctdev/brain-triage.prompt      default prompt; <repo>/scheduled/triage.md wins
/usr/local/bin/brain-run            the one entry point both timers call
```

**The service account is `brain`**, a system user — deliberately neither Thomas
nor Le'Anna, who are principals of equal standing. Its commits are attributable to
the node. A second service account joins group `brain` to read the checkout
without being the timer's user.

The load-bearing facts are in `.claude/rules/brain-invariants.md`.
