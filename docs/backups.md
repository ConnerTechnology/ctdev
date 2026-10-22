# Backups

Backup is about the folder you're in. A folder is set up once, then backed up whole, as you, with
[restic](https://restic.net). There is no sudo and nothing outside your home is involved.

Per-folder backup is arriving in steps under
[CTD-87](https://linear.app/conner-technology/issue/CTD-87). This page describes what ships today:
the status screen, Configure, Back up now, List backups, Stop backing up, Delete all backups,
scheduled backups with their health, and `ctdev backup --all` with its computers, Emergency Kit
and password actions.

## Running `ctdev backup`

`ctdev backup` looks up from the current directory for the nearest `.ctdev-backup.json`, the way
git finds its repository. Its first line says what it found:

```text
Backed up folder: ~/Projects/website
Backed up folder: ~/Projects/website  (you're in ~/Projects/website/docs)
Not backed up: ~/Downloads
```

In a subfolder, the status and menu are the backed-up folder's. Then come the status lines:

```text
  Destination    example-backups (Backblaze B2)
  Size now       1.2 GB · 4,812 files
  Last backup    Oct 21, 9:12 PM · succeeded
  Schedule       daily · next due Oct 22, 9:12 PM
                 runs only while you're logged in
  Keeping        24 hourly, 7 daily, 4 weekly, 12 monthly, 3 yearly
  Backups        14 · oldest Sep 2 · 1.6 GB stored
```

These show in red:

- **A failed last backup**, with restic's error.
- **Overdue**: no successful backup on this computer within twice the schedule, such as 48 hours
  for a daily folder.
- **"not scheduled on this computer"**: the folder isn't on this computer's list, as with a git
  clone, or this computer's hourly runner isn't installed. Run Configure here to fix either.

"runs only while you're logged in" shows on Linux only ([Scheduled backups](#scheduled-backups)
says why).

Below the status is a menu you move through with the arrow keys. Items that don't apply are dimmed
and can't be chosen.

| Folder | Menu |
| --- | --- |
| Configured | Back up now · List backups · Configure · Stop backing up · Delete all backups · Quit |
| Not configured | Configure · Back up now (once) · List backups · Stop backing up · Delete all backups · Quit |

List backups and Delete all backups are dimmed until the folder has a backup. Stop backing up is
always dimmed in a folder that isn't configured.

### Without a terminal

Piped (`ctdev backup | cat`) or with `--batch`, `ctdev backup` prints the status and no menu. It
exits non-zero when the folder isn't backed up, or its last backup failed or is overdue, so it works
as a check.

## Configure

Configure sets up the folder in one walk-through: a destination, a schedule and retention.
Reconfiguring a folder starts from its current values.

- **Destination.** Pick one this computer already has, create a new one, or connect to an existing
  one (below). A destination is one restic repository, shared by every folder and computer that
  backs up to it: Backblaze B2, S3 or S3-compatible, SFTP, or a local path or USB drive.
- **Schedule.** Hourly, Daily (the default) or Weekly: a backup is due that long after the last
  successful one. There's no time of day.
- **Retention.** How many hourly, daily, weekly, monthly and yearly backups to keep. The defaults
  are 24, 7, 4, 12 and 3.

Configure writes `.ctdev-backup.json` at the folder's root, adds the folder to this computer's
list, and installs this computer's hourly runner ([Scheduled backups](#scheduled-backups)). The
file holds only the folder's random ID, the destination's name, the schedule and the
retention, so it's safe to commit; in a git repo, commit it, and a clone on another computer finds
the folder's backups. ctdev never edits `.gitignore`.

Configuring a folder for the first time, when the destination already holds other folders' backups,
offers "Connect to an existing folder's backups". You pick a folder from the same list, grouped by
computer, that `ctdev backup --all` shows, and this folder's `.ctdev-backup.json` takes that
folder's ID, so its backups continue that history. Use it for a folder rebuilt on a new computer, or
one whose backing up was stopped. Only folders with an ID can be continued, not one-off backups,
and the list leaves out any folder this computer still backs up, since two folders sharing an ID
would share their keep rules and deletes. Going Back from the list (or q or esc) starts a new
history instead; Ctrl+C cancels Configure.

The list can't tell whether another computer still backs a folder up. Linking to one that it does
makes this folder share that folder's keep rules and deletes: retention counts both folders'
backups together, and Delete all backups in either removes both. Link only a folder that was rebuilt
or moved here, not one that is still in use elsewhere.

In a subfolder, Configure reconfigures the backed-up folder above it; a folder inside a backed-up
folder can't be set up as one of its own. Configuring a folder that contains a backed-up folder is
allowed, with a warning that the inner folder will be in both folders' backups.

### Creating a destination

You name it, choose the backend and give its credentials, and type a master password twice. ctdev
generates a Secret Key, creates the restic repository, and shows two things once:

- **The Emergency Kit**: the destination's name and address, the backend credentials, the Secret
  Key, and a blank line to write the master password on. Save it in your password manager or print
  it.
- **The Recovery Code**: keep it apart from the kit. It's for resetting a forgotten master password.

The master password and the Secret Key together unlock the destination; neither opens anything
alone, and ctdev never stores the master password. Each computer gets its own key, labeled with its
hostname, so backups run without you. [ADR-0016](adr/0016-backup-encryption-like-1password.md) has
the reasoning. Lose the kit and the master password, with no Recovery Code and no computer still
set up, and the backups can't be decrypted by anyone, ctdev included.

### Connecting to an existing destination

On another computer, choose "Connect to an existing destination", paste the Emergency Kit, and type
the master password. ctdev opens the repository with them before saving anything; a wrong password
or kit saves nothing. It then adds this computer's own key.

## Back up now

Back up now says how much it will back up and asks first:

```text
  This backs up all of ~/Projects/website: 1.2 GB, 4,812 files. Continue? [Y/n]
```

The whole folder is backed up from inside it, with paths relative to it. Each backup is tagged with
the folder's ID, its path as shown on this computer (`~/Projects/website`), and the hostname.

In a folder that isn't configured, Back up now (once) backs it up to this computer's destination,
setting one up first if there isn't one. It writes no config file and sets no schedule. Such a
one-off backup has no folder ID and is found by its path and hostname; if you configure the folder
later, Configure adds the new ID to those backups so they join its history.

### Retention

After each successful backup of a configured folder, ctdev forgets the folder's backups that its
keep rules no longer cover, counting the folder's backups from every computer together. Other
folders' backups in the same destination are never touched. One-off backups of a folder that isn't
configured have no keep rules and stay until you delete them. Forgetting a backup doesn't free its
space right away: that happens at the destination's weekly maintenance.

## List backups

List backups shows the folder's backups, newest first, under a header with how many there are and
the space they take in the destination after deduplication:

```text
Backups of ~/Projects/website · 14 backups · 1.6 GB stored

  > Oct 21, 9:12 PM     1.2 GB     12.0 MB new   4,812 files  latest
    Oct 20, 9:10 PM     1.2 GB      3.1 MB new   4,809 files
    ...
    Back
```

Choosing a backup shows its size, file count, new data, the files changed and added since the
backup before it, how long it took, the computer that made it and its short ID. Then:

- **Download as zip.** restic writes the backup as a zip into a temporary file, which is renamed
  into place once complete, and ctdev prints the path. The zip holds the folder's contents at their
  paths inside the folder. It's named after the folder and the backup's time in UTC, with no colons
  so the name is safe everywhere, for example `website-20261022T141500Z.zip`. It goes beside the folder, in
  `~/Projects`, and never inside a backed-up folder: when the folder sits inside another backed-up
  folder, the zip goes beside the outer one. The zip is readable only by you (mode 0600), since it
  holds the folder's contents. ctdev doesn't restore; open the zip and copy back what you need.
- **Delete this backup** asks `[y/N]`, then forgets that one backup.
- **Back** returns to the list.

## Delete all backups

Delete all backups asks `[y/N]`, then has you type the folder's name, and forgets every backup of
the folder: by its ID once configured, or, for a folder that isn't, its one-offs, the backups with
no folder ID that this computer made of its path. A backup that carries a folder ID always belongs
to that folder, so a new folder made where a backed-up one used to be never claims the old one's
backups. After any delete, ctdev says "Space is freed at the next weekly maintenance."

## Stop backing up

Stop backing up asks `[y/N]`, deletes the folder's `.ctdev-backup.json` and takes it off this
computer's list. It then asks whether to also delete all the folder's backups, from every computer;
saying yes has you type the folder's name, as Delete all backups does, and forgets every backup with
the folder's ID before its config file goes.

Otherwise ctdev says "Its N backups are kept under this folder's ID", and that `ctdev backup
--all` lists them and Configure's "Connect to an existing folder's backups" resumes the history.
`ctdev backup` then shows the folder as not backed up, and since the folder no longer has that ID,
List backups there shows only its one-offs.

Stop backing up refuses when the destination can't be read or isn't set up on this computer, so the
chance to delete the backups is never skipped without saying so. When it takes the last folder off
this computer's list, it also removes the hourly runner ([Scheduled backups](#scheduled-backups)).

## Scheduled backups

Each computer has one hourly runner, installed by Configure and running as you:

- **Linux:** a systemd user timer, `~/.config/systemd/user/ctdev-backup.timer`, with its service
  `ctdev-backup.service`. It first runs at the top of the next hour. `Persistent=true` runs a
  missed hour once the computer is back on.
- **macOS:** a launchd agent, `~/Library/LaunchAgents/io.connertechnology.ctdev.backup.plist`,
  logging to `~/Library/Logs/ctdev-backup.log`. It runs once right after Configure loads it, then
  at the top of every hour; an hour missed while the Mac slept runs when it wakes.

Both run `ctdev backup run-scheduled` by ctdev's absolute path, since the runner has no shell PATH.
Configuring again rewrites them, which picks up a ctdev that has moved. On macOS, reloading the
agent would stop a backup it's making, so an agent already loaded with this ctdev and this plist is
left alone, and while a scheduled backup is running Configure changes nothing and says to choose
Configure again after it finishes. If systemd won't enable the timer, Configure puts back whatever
units were there before: a scheduler that worked keeps working, and on a computer that had none the
status shows "not scheduled on this computer". Only one scheduled run happens at a time; one that
starts while another is running exits without doing anything. Configure and Back up now never wait
for a scheduled backup to finish.

Every hour, the runner backs up each folder on this computer's list whose schedule is due: an hour,
a day or a week after its last successful backup. A computer that was asleep catches up on its next
run. After each backup it applies the folder's keep rules ([Retention](#retention)); if that fails,
the backup still counts, but the failure is the run's error and shows in red. Each folder's result
is recorded in `state.json`.

A listed folder that has been moved or deleted, or whose `.ctdev-backup.json` is gone or now holds
another folder's ID, is skipped and recorded as missing. Run `ctdev backup` in its new place and
choose Configure: once a scheduled run has recorded the old place as missing, Configure removes it
from the list and says so. A folder on a drive that isn't mounted, which was fine at its last run,
stays on the list.

**On Linux, scheduled backups run only while you're logged in.** A systemd user timer stops at
logout unless lingering is on, and ctdev never turns lingering on. A computer you stay logged in to,
or log back in to, catches up. If you've turned lingering on yourself, the status leaves out "runs
only while you're logged in".

On Linux, `systemctl --user list-timers ctdev-backup.timer` shows when the runner last ran and runs
next, and `journalctl --user -u ctdev-backup.service` shows what it did.

### When a backup fails

When a scheduled folder starts failing, or goes missing, you get one desktop notification:
`notify-send` on Linux, `osascript` on macOS. You get another when it's backing up again. A folder
that keeps failing doesn't notify every hour. A computer with no desktop session, or without the
tool, gets no notification; `ctdev backup` and `ctdev doctor` still show the problem.

### In `ctdev doctor`

`ctdev doctor` has a Backups section with a row for each folder on this computer's list: OK, failed
(with the error), overdue or missing. It reads only `folders.json` and `state.json`, so it makes no
network calls, doesn't run restic and needs no root. A computer with nothing scheduled has no
Backups section.

## Every folder: `ctdev backup --all`

`ctdev backup --all` is the whole destination rather than the folder you're in. It works from any
directory and writes nothing into it, except a download that falls back there (below). Its first
line names the destination and what it holds, with the space stored measured once when `--all`
starts:

```text
example-backups (Backblaze B2) · 2 computers · 3 folders · 1.6 GB stored
```

The menu is List backups · Computers · Emergency Kit and password · Quit. With several destinations
on this computer, `--all` asks which one first. "Computers" counts hostnames, so a computer set up again under the same name counts once.

List backups shows every backed-up folder in the destination, grouped by computer, with this
computer first. Each entry is the folder's path as it was on that computer, with its latest backup,
that backup's size and the number of backups:

```text
Folders in example-backups

  example-desktop (this computer)
  > ~/Documents/notes   Oct 21, 9:12 PM    52.0 kB  3 backups

  example-laptop
    ~/Downloads/scans   Oct 2, 8:05 AM      3.4 MB  1 backup
    ~/Projects/website  Oct 20, 9:10 PM     1.2 GB  14 backups

    Back
```

A folder backed up from two computers appears under each. One-off backups, which have no folder ID,
are grouped by path and computer. Picking a folder opens the same backup list as List backups in a
folder, with Download as zip and Delete this backup; a folder with an ID lists its backups from
every computer.

A download under `--all` goes beside where the folder would be on this computer, when that parent
folder exists here: `~/Projects/website`'s zip goes in `~/Projects`. Otherwise it goes in the current
directory, or, when that is inside a backed-up folder, beside that folder. The same fallback applies
when you can't write to that parent, as for a folder recorded as `~`. It never lands inside a
backed-up folder.

Piped or with `--batch`, `ctdev backup --all` prints each destination's first line and its folders.

### Setting up a new computer

On a computer with no destination, `ctdev backup --all` first offers "Connect to an existing
destination" or "Create a new destination". To connect, paste the Emergency Kit and type the master
password. ctdev checks them by opening the repository before it saves anything; a wrong password or
kit saves nothing. It then adds this computer's own key, labeled with its hostname, and saves the
destination to `~/.config/ctdev/backup/destinations/<name>.env`.

Creating a local destination here needs an absolute path or one starting with `~/`, since `--all`
runs from any directory: `~/Backups` works, `Backups` doesn't. As in Configure, the path can't be
inside a backed-up folder and must be a new or empty folder.

### Finding a folder's backups from another computer

1. Install restic with `ctdev install restic`.
2. Run `ctdev backup --all` and connect to the destination with its Emergency Kit and master
   password.
3. Choose List backups, then the folder: under the computer that backed it up, by its path there.
4. Choose a backup, then Download as zip, and copy what you need out of the zip.

To keep backing the folder up on this computer, run `ctdev backup` in it, choose Configure, and
pick the folder under "Connect to an existing folder's backups". A git clone of a folder whose
`.ctdev-backup.json` is committed already has the folder's ID, so Configure continues its history
without that step.

### Computers

Computers lists each computer with a key on the destination, by hostname and the date its key was
added, with this computer marked. The account key and the Recovery Code's key aren't computers and
aren't listed. Pick one for Remove this computer · I lost this computer · Back.

Every action here and under Emergency Kit and password asks for the master password first, and
checks it by opening the destination with the account key. A wrong one changes nothing. The two
exceptions are Use a Recovery Code and Paste an updated Emergency Kit.

- **Remove this computer** removes its key, so it can no longer open the destination. This
  computer's own key can't be removed from here: do it from another computer that's set up.
- **I lost this computer** is one guided flow:
  1. It removes the lost computer's key.
  2. It makes a new Secret Key and saves it on this computer.
  3. It asks for new backend credentials, with numbered steps for making them at Backblaze B2 or
     AWS, checks this computer opens the destination with them, and then gives the steps for
     revoking the old ones. For an S3-compatible provider other than AWS, the steps are general,
     and that provider's documentation has the exact clicks. For SFTP it gives the steps for taking
     the lost computer's SSH key off the server and then checks the connection. A local path or USB
     drive has nothing to revoke.
  4. It shows the new Emergency Kit.
  5. You paste the new kit once on each other computer (below).

  If it stops partway, after a failure or Ctrl-C, it says what it has done and which items under
  Emergency Kit and password finish it: the lost computer is no longer listed, so the flow can't
  be started again for it.

### Emergency Kit and password

- **Show Emergency Kit** shows the kit this computer has.
- **Change master password** asks for the new one twice. The Secret Key and the kit stay the same,
  and every computer's key keeps working.
- **Regenerate Secret Key** makes a new Secret Key and account key, saves the Secret Key here, and
  shows the new kit. The old kit stops working. The other computers' keys keep backing up, but
  paste the new kit on each before using its account actions there.
- **Replace backend credentials** is step 3 of I lost this computer on its own: the provider's
  steps for new credentials, checked by opening the destination with them, then the steps for
  revoking the old ones, and the new kit. Use it to finish that flow, or to rotate the credentials
  any time.
- **Use a Recovery Code** takes the Recovery Code and a new master password, typed twice, and makes
  a new Secret Key. It shows the new kit. The used code keeps working unless you also choose to
  make a new Recovery Code, which replaces it and is shown once. Connecting a new computer offers
  it too, after a wrong master password.
- **Paste an updated Emergency Kit** saves a new kit's credentials and Secret Key on this computer.
  It asks for no master password, since this computer's own key is what opens the destination:
  ctdev checks that key opens it as the kit describes before saving anything. If this computer
  can't read the destination any more, because its credentials were revoked, `--all` offers the
  paste straight away. Choosing "Connect to an existing destination" with a kit for a destination
  this computer already has does the same. When the kit's Secret Key differs from the one saved
  here, ctdev says so: that's expected after Regenerate Secret Key or I lost this computer on another
  computer, and a sign the kit isn't yours otherwise.

A kit holds only the destination's name, address, backend credentials and Secret Key. ctdev refuses
a kit with any other line, such as a variable restic would act on.

Changing keys never leaves the destination without a working account key. The new key is added and
checked first, and the old one is removed last. If that removal fails, ctdev says so, and both keys
still work until you try again.

## When something goes wrong

restic gives every key full power over the repository: reading, deleting and removing other keys.
The master password guards ctdev's account actions, but restic doesn't enforce it. A lost or stolen
computer can act on the destination with its key until that key is removed, and reach the storage
with its backend credentials until they're revoked. It also keeps whatever it already read.
Removing its key doesn't change the repository's master key, which every restic key unlocks, so a
computer that had it can still decrypt whatever it can reach: revoking the storage credentials is
the real barrier, and a local path or an SFTP login shared by every computer has none to revoke.

- **A new computer**: install restic (`ctdev install restic`), run `ctdev backup --all`, and connect
  with the Emergency Kit and the master password.
- **A lost or stolen computer**: on any other computer that's set up, run `ctdev backup --all`,
  choose Computers, the lost computer, then I lost this computer. Then paste the new kit on each
  other computer.
- **A lost Emergency Kit**: on a computer that's set up, choose Emergency Kit and password, then
  Show Emergency Kit, and save it again.
- **A forgotten master password**: choose Use a Recovery Code, on a set-up computer under Emergency
  Kit and password, or on a new one after a wrong master password while connecting. Without the
  Recovery Code, computers that are set up keep backing up, but no new computer can connect and no
  account action works. The fix is then a new destination.
- **Losing everything**: with no computer still set up, and the kit or the master password gone with
  no Recovery Code, the backups can't be decrypted by anyone, ctdev included.

## Where things are kept

| Where | What |
| --- | --- |
| `.ctdev-backup.json` in the folder | `id`, `destination`, `schedule`, `keep`; no secrets |
| `~/.config/ctdev/backup/destinations/<name>.env` (0600) | The destination's address, backend credentials, Secret Key and this computer's key |
| `~/.config/ctdev/backup/folders.json` | The folders this computer backs up on a schedule, with each one's schedule and when it was added |
| `~/.config/ctdev/backup/state.json` | Each folder's last attempt, last success, error and whether it's missing on this computer, and whether a "failing" notification is out |

restic gives every key full power over the repository, so whoever has a computer's
`destinations/<name>.env` can read and delete that destination's backups. Keep the file private.

## restic

`ctdev install restic` installs the restic binary and nothing else, with no root: on Linux the
official 0.19.1 release, checked against its `SHA256SUMS` and put in `~/.local/bin` (apt's restic is
too old for per-folder backup); on macOS, Homebrew's. ctdev runs `~/.local/bin/restic` when it's
there, since a scheduled run doesn't get your shell's PATH, and restic on PATH otherwise.

ctdev needs restic 0.17 or newer and says so when it finds an older one; `ctdev install restic`
gets a new enough one.
