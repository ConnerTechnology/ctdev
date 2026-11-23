# Configure

`ctdev configure` applies system settings. Where [components](components/README.md)
put software on a machine, `configure` changes how the machine behaves: the SSH
server, the firewall, whether it sleeps, which upstreams Pi-hole uses.

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

**Machine behavior** — `ssh`, `power`, `gpu`, `storage`. These change the machine itself:
remote access, power (the power profile, the display, idle-suspend and
screensaver timers in minutes or never, "Never suspend, even when asked", and on
macOS a password right after the screensaver), and the NVIDIA driver and MOK
signing. Categories are topics, not operating systems: each setting shows only
on the platform it applies to, so there is no `macos` category. Typing is in
`keyboard`, which shows the repeat rate in characters per second on both
Cinnamon and macOS and holds the macOS text-substitution and press-and-hold
settings.

`storage` is the machine's disks and swap: weekly SSD TRIM (only with an SSD),
and on Linux a swapfile and swappiness. The swapfile is `/swapfile`, 1 GiB by
default, with one `/etc/fstab` line so it comes back after a reboot; 0 removes
it. Swappiness defaults to 10, written to
`/etc/sysctl.d/zz-ctdev-swappiness.conf`, which sorts after every other sysctl
file so its value is the one that holds at boot. ctdev's fstab line carries a
comment saying ctdev owns it.

`--batch` only creates a swapfile where there is no swap at all. It never
resizes one, ctdev's included, unless `--swapfile-size` names the size, and
leaves alone any swap ctdev didn't set up,
switched on or not: a partition or zram, a swap line in `/etc/fstab`, or any
file already at `/swapfile`. On such a machine `--show` reports that swap as
"not needed", and swappiness is left alone too. The wizard, `ctdev configure
storage` without `--batch`, can still resize ctdev's swapfile or take over a
hand-made `/swapfile` that is switched on, after you say yes; a matching size
leaves the file as it is. A new size refuses to start when the swap in use
wouldn't fit in free memory or the disk lacks room, and on a filesystem other
than ext4, XFS or btrfs there is no swapfile.

`ctdev configure storage --batch --swapfile-size N` is for a server's boot
script, which knows the size its node needs: it creates a swapfile of N GiB
where there is no swap, and resizes ctdev's own swapfile to N GiB, with the
same memory and disk checks. `--swapfile-size 0` removes ctdev's own swapfile
and its fstab line. Swap ctdev didn't set up is still left alone. It works
only with `--batch` given as a flag, not piped stdin alone, and not with
`--show`.

**Desktop** — `audio`, `bluetooth`, `desktop`. `bluetooth` installs bluez and Blueman and
turns on `bluetooth.service`; `audio` installs PipeWire, WirePlumber and the
PulseAudio tools, plus Bluetooth audio (PipeWire's Bluetooth codecs and an LDAC
config), and switches alert sounds; on macOS it has the click when you change
the volume. Neither installs the other's packages.
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

`desktop` holds how the desktop looks and behaves, on whichever platform you run
it. On macOS: an auto-hiding Dock, faster animations, Finder's
list view with path and status bars, no `.DS_Store` files on network and USB
drives, and expanded save and print dialogs, each one on or off. On Cinnamon: Nemo's default view, and Hide drives,
which lists this machine's partitions and hides the ones you check from the file
manager. It writes `/etc/udev/rules.d/99-hide-drives.rules` keyed by filesystem
UUID; checking nothing removes it. `--batch` leaves Hide drives alone, since
which partitions to hide is this machine's choice.

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

Every category, with what each one does, is listed in [Commands](commands.md).

## Secrets

Anything secret a wizard asks for is stored on that machine only and is never
written to this repo. See [Security](security.md).

## Previewing

`--dry-run` works on `configure` as it does everywhere else: it prints what
would change and changes nothing.
