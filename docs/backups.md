# Backups

Backup is about the folder you're in. A folder is set up once, then backed up whole, as you, with
[restic](https://restic.net). There is no sudo and nothing outside your home is involved.

Per-folder backup is arriving in steps under
[CTD-87](https://linear.app/conner-technology/issue/CTD-87). This page describes what ships today:
the status screen, Configure, Back up now, and scheduled backups with their health. List backups,
Stop backing up, Delete all backups and `ctdev backup --all` come next; until then they show as
"coming soon".

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

## Scheduled backups

Each computer has one hourly runner, installed by Configure and running as you:

- **Linux:** a systemd user timer, `~/.config/systemd/user/ctdev-backup.timer`, with its service
  `ctdev-backup.service`. It first runs at the top of the next hour. `Persistent=true` runs a
  missed hour once the computer is back on.
- **macOS:** a launchd agent, `~/Library/LaunchAgents/io.connertechnology.ctdev.backup.plist`,
  logging to `~/Library/Logs/ctdev-backup.log`. It runs once right after Configure loads it, then
  at the top of every hour; an hour missed while the Mac slept runs when it wakes.

Both run `ctdev backup run-scheduled` by ctdev's absolute path, since the runner has no shell PATH.
Configuring again rewrites them, which picks up a ctdev that has moved. On macOS an agent that's
loaded and unchanged is left alone, and a changed one isn't reloaded while a scheduled backup is
running: Configure says so, and the new one takes effect at your next login. If systemd won't enable
the timer, Configure removes the units it wrote and the status shows "not scheduled on this
computer". Only one scheduled run happens at a time; one that starts while another is running exits
without doing anything. Every hour, the runner backs
up each folder on this computer's list whose schedule is due: an hour, a day or a week after its
last successful backup. A computer that was asleep catches up on its next run. Each folder's result
is recorded in `state.json`.

A listed folder that has been moved or deleted, or whose `.ctdev-backup.json` is gone or now holds
another folder's ID, is skipped and recorded as missing. Run `ctdev backup` in its new place and
choose Configure: that replaces the old place on the list, so it stops showing as missing.

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
