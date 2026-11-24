# Configure

`ctdev configure` applies system settings. Where [components](components/README.md)
put software on a machine, `configure` changes how the machine behaves: the SSH
server, whether it sleeps, which upstreams Pi-hole uses.

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

`ssh` is the one place for remote access: your SSH keys and this machine's
SSH server. `ctdev configure ssh` opens a menu with three entries.

- **Your keys** lists the key pairs in `~/.ssh`. Pick one by its number, or
  several (`1 3`). For one key you can copy it to a server, upload it to
  GitHub, copy it to the clipboard, or delete it. For several, copy each one
  to a server, upload each one to GitHub, or delete them; the clipboard holds
  one key, so it isn't offered.
  - Copy to a server asks once for `user@host` and an optional port, then runs
    `ssh-copy-id` for each key, which asks for the server's password itself.
  - Upload to GitHub asks whether the keys are for authentication, signing, or
    both, then runs `gh ssh-key add`. Without `gh` signed in, it shows each key
    and the steps to add it on github.com.
  - The clipboard is `pbcopy` on macOS, and `wl-copy` or `xclip` on a Linux
    desktop. In an SSH session with no display, ctdev sends the key to the
    terminal you're typing in (OSC 52), and prints it too in case the terminal
    blocks that. Inside tmux, OSC 52 needs `set -g set-clipboard on`.
  - Delete names the private and the public file and asks first; Enter keeps
    them. Copies on GitHub and on servers stay.
- **Create a key** asks the type (`ed25519` by default, `ed25519-sk` for a
  hardware security key, `ecdsa`, or `rsa` at 4096 bits), the file name
  (`~/.ssh/id_<type>`, or `id_<type>_<purpose>` after asking what the key is
  for when that is taken), and the comment (`user@hostname`). Then
  `ssh-keygen` asks for the passphrase itself, so ctdev never holds it. ctdev
  never overwrites a key: a name either half of an existing key uses is
  refused. On macOS the new key goes into the Keychain
  (`ssh-add --apple-use-keychain`). Then you get the one-key actions for it.
- **Server settings** are the settings below, asked one at a time.

`--batch` and `--show` skip the menu and apply or show the server settings.
`--dry-run` prints each command the menu would run and changes nothing.
Apart from the one-time cleanup below, only Server settings asks for sudo.

The server settings are the SSH server
(started on demand by `ssh.socket`, always running, or off) and key-only login.
Earlier versions also offered lingering, Mosh support (the `en_US.UTF-8`
locale), the VS Code tunnel and a LAN-only UFW firewall. The first `ctdev
configure ssh` or bare `ctdev configure` run, except with `--show`, turns off
whichever of those ctdev turned on, once. A locale that was already there stays, and so does a
firewall without ctdev's rules; a firewall with other rules stays on once
ctdev's are deleted. Lingering and the tunnel record no owner, so they are
turned off wherever they are on; turned back on by hand, they stay on.
`ctdev install ssh` installs the SSH server and then opens this menu.

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

**Component wizards** — `caddy` belongs to a component and mostly runs for you
when you install it; running `configure caddy` reconfigures without
reinstalling. `caddy --batch` skips its prompts and takes the domain, email and
token from `--domain`, `--acme-email`, `CF_API_TOKEN` or the existing
`~/caddy/.env`.

**Pi-hole** has no `configure` subcommand. `ctdev install pihole` asks its
settings after installing, and again whenever it runs on a node that has
Pi-hole; a bare `ctdev configure` lists them too. Nothing applies them without
asking: `ctdev install pihole --batch` installs Pi-hole and skips its settings.

Every category, with what each one does, is listed in [Commands](commands.md).

## Secrets

Anything secret a wizard asks for is stored on that machine only and is never
written to this repo. See [Security](security.md).

## Previewing

`--dry-run` works on `configure` as it does everywhere else: it prints what
would change and changes nothing.
