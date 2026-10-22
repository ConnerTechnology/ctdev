# Backups

Backup is about the folder you're in. A folder is set up once, then backed up whole, as you, with
[restic](https://restic.net). There is no sudo and nothing outside your home is involved.

Per-folder backup is arriving in steps under
[CTD-87](https://linear.app/conner-technology/issue/CTD-87). This page describes what ships today:
the status screen, Configure, Back up now, List backups, Stop backing up and Delete all backups.
Scheduled runs and `ctdev backup --all` come next.

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
  Keeping        24 hourly, 7 daily, 4 weekly, 12 monthly, 3 yearly
  Backups        14 · oldest Sep 2 · 1.6 GB stored
```

A failed last backup shows in red. A folder whose config file came from elsewhere, such as a git
clone, shows "not scheduled on this computer" until you run Configure on it here.

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
exits non-zero when the folder isn't backed up or its last backup failed, so it works as a check.

## Configure

Configure sets up the folder in one walk-through: a destination, a schedule and retention.
Reconfiguring a folder starts from its current values.

- **Destination.** Pick one this computer already has, create a new one, or connect to an existing
  one (below). A destination is one restic repository, shared by every folder and computer that
  backs up to it: Backblaze B2, S3 or S3-compatible, SFTP, or a local path or USB drive.
- **Schedule.** Hourly, Daily (the default) or Weekly: a backup is due that long after the last
  successful one. There's no time of day. Scheduled runs arrive in a later release; until then,
  use Back up now.
- **Retention.** How many hourly, daily, weekly, monthly and yearly backups to keep. The defaults
  are 24, 7, 4, 12 and 3.

Configure writes `.ctdev-backup.json` at the folder's root and adds the folder to this computer's
list. The file holds only the folder's random ID, the destination's name, the schedule and the
retention, so it's safe to commit; in a git repo, commit it, and a clone on another computer finds
the folder's backups. ctdev never edits `.gitignore`.

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
  folder, the zip goes beside the outer one. ctdev doesn't restore; open the zip and copy back what
  you need.
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

Otherwise the backups are kept, still tagged with the folder's ID. `ctdev backup` then shows the
folder as not backed up, and since the folder no longer has that ID, List backups there shows only
its one-offs. `ctdev backup --all` lists the kept backups. To back the folder up again with its
history, choose Configure, then "Connect to an existing folder's backups".

## Where things are kept

| Where | What |
| --- | --- |
| `.ctdev-backup.json` in the folder | `id`, `destination`, `schedule`, `keep`; no secrets |
| `~/.config/ctdev/backup/destinations/<name>.env` (0600) | The destination's address, backend credentials, Secret Key and this computer's key |
| `~/.config/ctdev/backup/folders.json` | The folders this computer backs up on a schedule |
| `~/.config/ctdev/backup/state.json` | Each folder's last attempt, last success and error on this computer |

restic gives every key full power over the repository, so whoever has a computer's
`destinations/<name>.env` can read and delete that destination's backups. Keep the file private.

## restic

`ctdev install restic` installs the restic binary and nothing else, with no root: on Linux the
official 0.19.1 release, checked against its `SHA256SUMS` and put in `~/.local/bin` (apt's restic is
too old for per-folder backup); on macOS, Homebrew's. ctdev runs `~/.local/bin/restic` when it's
there, since a scheduled run doesn't get your shell's PATH, and restic on PATH otherwise.

ctdev needs restic 0.17 or newer and says so when it finds an older one; `ctdev install restic`
gets a new enough one.
