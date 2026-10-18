# Configure

`ctdev configure` applies system settings. Where [components](components/README.md)
put software on a machine, `configure` changes how the machine behaves: the SSH
server, the firewall, whether it sleeps, what its locale is, which upstreams
Pi-hole uses.

```bash
ctdev configure                 # full-screen browser over every category
ctdev configure <category>      # one category, interactively
ctdev configure <category> --batch   # apply that category's defaults, no prompts
ctdev configure --show          # what is currently configured
```

Each setting is a **category**. Run with no arguments and you get a browser over
all of them; name one and you get just that one. `--batch` applies the
recommended defaults without asking, which is what you want in a script.

## The categories

They fall into three groups.

**Machine behavior** — `ssh`, `sleep`, `autoupdate`, `macos`, `gpu`. These
change the machine itself: remote access, suspend, automatic security updates,
macOS defaults (Dock, Finder, typing, faster animations), and the NVIDIA driver
and MOK signing. Key repeat speed is not in `macos`: it is in `keyboard`, as
sliders on both Cinnamon and macOS.

`ssh` is the one place for remote access. Its settings are the SSH server
(started on demand by `ssh.socket`, always running, or off), key-only login,
keep my programs running after I disconnect (systemd lingering), Mosh support
(the `en_US.UTF-8` locale), open this machine from vscode.dev (the VS Code
tunnel, which also turns on lingering), and only allow SSH and Mosh from my
local network (UFW). Each one can be turned off, which removes what ctdev
added. The firewall blocks every other incoming service unless you open its
ports first, so on a DNS or proxy host run `ctdev configure ssh` without
`--batch` and leave it off.

**Your identity on it** — `git`, `aws`. Name, email, signing key, AWS profile.

**Component wizards** — `pihole`, `caddy`, `restic`,
`mcp-email-server`. These belong to a component and mostly run for you when you install
it; running `configure` directly reconfigures without reinstalling. `restic` and
`mcp-email-server` ask for secrets and refuse `--batch`. `caddy --batch` skips its
prompts and takes the domain, email and token from `--domain`, `--acme-email`,
`CF_API_TOKEN` or the existing `~/caddy/.env`. `pihole --batch` applies the
category's defaults, which also set the upstream to Cloudflare, so run it
interactively.

**Claude Code** — `claude-code` isn't a wizard: `ctdev configure claude-code` diffs
`~/.claude/settings.json` and `CLAUDE.md` against ctdev's baseline and asks before replacing
either. It also runs after any install that includes `claude-code`.

Every category, with what each one does, is listed in [Commands](commands.md).

## Linear for Claude Code

`ctdev configure linear` acts on the git repo you run it in, not on the machine.
It points that repo's Claude Code at Linear's MCP server as an OAuth app, so what
a session does in Linear shows up as the app and notifies you. You create one app
per computer. The wizard prints the steps, then checks the client ID and secret
with Linear before it saves them.

- Credentials are kept per Linear workspace, in
  `~/.config/ctdev/linear/<workspace>.env` (owner-only). The app token is cached
  in `~/.cache/ctdev/linear/<workspace>.json`.
- The repo gets a `linear` server in `.mcp.json` whose `headersHelper` is
  `ctdev linear mcp-headers --workspace <workspace>`, and the server is
  pre-approved in `.claude/settings.local.json`.
- In your own repo you commit `.mcp.json`. In someone else's, the wizard adds it
  to the repo's git exclude file so it stays local.

`ctdev configure linear --show` reports the repo's workspace, whether its
credentials are saved, and who the app connects as. Restart Claude Code in the
repo after setting it up.

## Secrets

Anything secret a wizard asks for is stored on that machine only and is never
written to this repo. See [Security](security.md).

## Previewing

`--dry-run` works on `configure` as it does everywhere else: it prints what
would change and changes nothing.
