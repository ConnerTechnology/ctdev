---
status: accepted
date: 2026-10-22
---

# Backups are locked by a Secret Key and a master password, with a key per computer

A destination's restic repository is opened by keys derived the way 1Password derives an account
unlock key: from two secrets. The **Secret Key** is 128 random bits that ctdev generates and stores on
each computer that is set up. The **master password** is chosen by the person and never stored. The
account key's password is Argon2id(master password, Secret Key). So a stolen copy of the bucket can't
be guessed offline: the password alone, or the Secret Key alone, opens nothing. Thomas decided this on
CTD-87 (2026-10-22), after a check against 1Password's support docs and Security Design white paper.

Setting up a computer is the gate. It takes the **Emergency Kit** (destination address, backend
credentials, Secret Key) and the master password. The computer then adds its own random restic key,
labeled with its hostname, and keeps that key, not the master password, so scheduled backups run
unattended. Removing a lost computer removes only its key. Account changes (show the kit, change the
master password, regenerate the Secret Key, remove a computer) ask for the master password first. A
**Recovery Code**, a separate key shown once and kept apart from the kit, resets a forgotten master
password.

## Considered options

- **One repository password copied to every computer** (the old `/etc/restic/restic.env`). That
  password is the only protection: whoever has it, or can guess it offline from the bucket, reads
  everything. Revoking one computer means changing it everywhere.
- **The derived account key stored on every computer.** This was simpler, but removing a lost computer
  would mean re-keying all the others.
- **Asking for the master password on every run**, as 1Password does on unlock. Nobody is there to
  type it for a 2 AM backup.

## Consequences

restic gives every key full power over the repository. The master-password gating is ctdev's, not
restic's, so a stolen computer can read, delete or remove keys until its key is removed and the
backend credentials are revoked, and nothing can make it forget what it already read. The lost-computer
flow therefore also regenerates the Secret Key and rotates the backend credentials, and every other
computer pastes the new kit once.

Lose the kit and the master password, with no Recovery Code and no computer still set up, and the
backups can't be decrypted by anyone, ctdev included.
