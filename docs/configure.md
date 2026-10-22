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

**Machine behavior** — `ssh`, `power`, `macos`, `gpu`. These change the
machine itself: remote access, power (the power profile, the display,
idle-suspend and screensaver timers in minutes or never, and "Never suspend,
even when asked"), macOS defaults (Dock, Finder, typing, faster animations),
and the NVIDIA driver and MOK signing. Key repeat speed is
not in `macos`: it is in `keyboard`, as sliders on both Cinnamon and macOS.

**Desktop** — `audio`, `bluetooth`. `bluetooth` installs bluez and Blueman and
turns on `bluetooth.service`; `audio` installs PipeWire, WirePlumber and the
PulseAudio tools, plus Bluetooth audio (PipeWire's Bluetooth codecs and an LDAC
config), and switches alert sounds. Neither installs the other's packages.
Turning a setting off removes only what ctdev added: ctdev records the packages it
installed and the service changes it made under
`~/.local/state/ctdev/configure-added/`, and leaves anything the machine already had.
If removing those packages would also remove one ctdev didn't install (something
that depends on them), it refuses and names it. Off leaves the dependencies those
packages pulled in; `sudo apt autoremove` cleans them up.

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
Each needs its tool: on a machine without git or the AWS CLI, it asks "Git isn't
installed. Install it now? [Y/n]" (or "AWS CLI isn't…") and installs the `git` or
`aws` component before going on. `--batch` installs without asking, a no stops the
wizard and says how to install later, and `--show` never installs. The check is the
component's own install check, so an AWS CLI v1 counts as missing. When
`~/.aws/config` has no profiles yet, `configure aws` offers to run
`aws configure sso` to create one. `ctdev install git` and `ctdev install aws` run
these wizards afterward.

**Component wizards** — `pihole`, `caddy`. These belong to a component and
mostly run for you when you install it; running `configure` directly
reconfigures without reinstalling. `caddy --batch` skips its prompts and takes
the domain, email and token from `--domain`, `--acme-email`, `CF_API_TOKEN` or
the existing `~/caddy/.env`. `pihole --batch` applies the category's defaults, which also set
the upstream to Cloudflare, so run it interactively.

**Claude Code** — `claude-code` isn't a wizard: `ctdev configure claude-code` diffs
`~/.claude/settings.json` and `CLAUDE.md` against ctdev's baseline and asks before replacing
either. It also runs after any install that includes `claude-code`.

Every category, with what each one does, is listed in [Commands](commands.md).

## Claude Code in a repo

Three commands act on the git repo you run them in, not on the machine. None of
them writes a file the repo commits, so they work the same in your repos and in
anyone else's. What Claude Code loads in a repo (skills, plugins, MCP servers,
settings) is a preference, and the engineering library's setup skill decides
it. ctdev only installs and connects.

### Setting a repo up

`ctdev repo setup` installs git, gh and Claude Code when they're missing, signs
gh in to GitHub if it isn't, and installs the `connertechnology` plugin from the
private `ConnerTechnology/engineering` marketplace, pinned to a tag. The
marketplace and the plugin are both at local scope, recorded in the repo's
`.claude/settings.local.json`, which ctdev adds to `.git/info/exclude` unless
something already ignores it. Then it starts Claude Code on
`/connertechnology:setup`, which asks how the repo should be set up.

Running it again changes nothing, unless a new ctdev release moved the pinned
tag; then it updates the plugin. With `--batch` it stops before starting Claude
Code and prints the command to run.

### Linear

`ctdev configure linear` points the repo's Claude Code at Linear's MCP server as
an OAuth app, so what a session does in Linear shows up as the app and notifies
you. You create one app per computer. The wizard prints the steps, then checks
the client ID and secret with Linear before it saves them.

- Credentials are kept per Linear workspace, in
  `~/.config/ctdev/linear/<workspace>.env` (owner-only). The app token is cached
  in `~/.cache/ctdev/linear/<workspace>.json`.
- Claude Code gets a `linear` server at local scope (kept in `~/.claude.json`
  for this repo only) whose `headersHelper` is
  `ctdev linear mcp-headers --workspace <workspace>`.
- A repo set up before this kept the server in `.mcp.json`. The wizard offers
  that workspace, adds the server locally, and says the `.mcp.json` entry can
  go. Claude Code uses the local one when both exist.

`ctdev configure linear --show` reports the repo's workspace, whether its
credentials are saved, and who the app connects as. Restart Claude Code in the
repo after setting it up.

### Jira

`ctdev configure jira` adds Atlassian's Rovo MCP server (`atlassian`) at local
scope. Claude Code signs in to it as you, with OAuth, the first time you run
`/mcp` there. Scripts that call Jira's REST API use your Atlassian API token
instead: the wizard gives the steps to create one, checks it with the site, and
saves it in `~/.config/ctdev/jira/<site>.env` (owner-only), where `<site>` is the
name before `.atlassian.net`. A script reads it as a header:

```bash
curl -H "$(ctdev jira auth-header --site example)" \
  https://example.atlassian.net/rest/api/3/myself
```

Atlassian API tokens expire within a year. When one does, the header stops
working; run `ctdev configure jira` again to save a new one.
`ctdev configure jira --show` reports the repo's server and whether each saved
site's token still works.

## Secrets

Anything secret a wizard asks for is stored on that machine only and is never
written to this repo. See [Security](security.md).

## Previewing

`--dry-run` works on `configure` as it does everywhere else: it prints what
would change and changes nothing.
