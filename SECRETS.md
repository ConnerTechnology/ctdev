# SECRETS.md — Encrypted secrets (SOPS + age)

Node secrets (API tokens, passwords, backup keys) are stored **encrypted in this
repo** with [SOPS](https://github.com/getsops/sops) using an
[age](https://github.com/FiloSottile/age) key. The encrypted files are safe to
commit; only the **age private key** can decrypt them, and that key is never
committed.

For restoring a machine from these, see **[RECOVERY.md](RECOVERY.md)**.

## The one key that matters: the age private key

| | |
|---|---|
| **Private key** (decrypts everything) | `~/.config/sops/age/keys.txt` on the node — **and a copy in 1Password** |
| **Public key** (recipient, used to encrypt) | in [`.sops.yaml`](.sops.yaml): `age1uer0729…aezgg8qz64tga` |

> 🔐 The private key is the master secret. It is **not** in git and **not** in
> any backup (by design). If you lose the Pi *and* the 1Password copy, every
> encrypted secret is unrecoverable. Save it now:
> ```bash
> cat ~/.config/sops/age/keys.txt      # store the whole file in 1Password (incl. the public-key comment)
> ```

`sops` finds the private key automatically at `~/.config/sops/age/keys.txt`. If
it's elsewhere: `export SOPS_AGE_KEY_FILE=/path/to/keys.txt`.

## What's encrypted

Each file lives next to its component's configs and decrypts to a node path.
Routing is by `path_regex` in [`.sops.yaml`](.sops.yaml).

| Encrypted file | Decrypts to | Holds |
|---|---|---|
| `ctdev/component/configs/restic/hosts/<node>.sops.env` | `/etc/restic/restic.env` | restic repo password, B2 keyID/key, repo paths |
| `ctdev/component/configs/caddy/hosts/<node>.sops.env` | `~/caddy/.env` | domain, ACME email, Cloudflare API token |
| `ctdev/component/configs/beszel/hosts/<node>.sops.env` | `~/beszel/.env` | Beszel agent KEY / TOKEN |
| `ctdev/component/configs/pihole/hosts/<node>.sops.env` | `~/pihole/.env` | Pi-hole admin password, TZ |
| `ctdev/component/configs/pihole/hosts/<node>.sops.json` | Pi-hole custom DNS | internal hostname → private IP records |

`<node>` is the hostname (e.g. `ctpi01`).

## Common tasks

All commands run from the repo root with the age key present.

**View a secret (read-only):**
```bash
sops -d ctdev/component/configs/restic/hosts/ctpi01.sops.env
```

**Edit a secret** (opens decrypted in `$EDITOR`, re-encrypts on save):
```bash
sops ctdev/component/configs/beszel/hosts/ctpi01.sops.env
```

**Deploy a secret onto the node** (decrypt into its real location):
```bash
# restic (root-owned):
sops -d ctdev/component/configs/restic/hosts/ctpi01.sops.env \
  | sudo install -m 600 /dev/stdin /etc/restic/restic.env

# caddy / beszel / pihole (user-owned ~/<svc>/.env):
sops -d ctdev/component/configs/beszel/hosts/ctpi01.sops.env > ~/beszel/.env && chmod 600 ~/beszel/.env
```

**Create secrets for a NEW node** (write plaintext at the path, then encrypt in place):
```bash
printf 'BESZEL_KEY=...\nBESZEL_TOKEN=...\n' \
  > ctdev/component/configs/beszel/hosts/<newnode>.sops.env
sops -e -i ctdev/component/configs/beszel/hosts/<newnode>.sops.env   # path_regex picks the recipient
git add ctdev/component/configs/beszel/hosts/<newnode>.sops.env      # safe: it's encrypted
```
Confirm it's encrypted before committing: `grep -c 'ENC\[' <file>` (should be > 0).

**Rotate the age key** (compromised / new recipient): update the `age:` recipient
in `.sops.yaml`, then re-encrypt every file to the new key:
```bash
sops updatekeys ctdev/component/configs/*/hosts/*.sops.*
```

## ⚠️ Rotating the restic repository password is special

The `RESTIC_PASSWORD` in `restic/hosts/<node>.sops.env` is the key the backup
repos are **encrypted with** — you can't just change it in the file, or you'll
lock yourself out of existing backups. Change it on the repo first, then sync
the file:
```bash
set -a; source <(sudo cat /etc/restic/restic.env); set +a
sudo -E restic -r "$RESTIC_REPO_B2"   key add      # set a new password
sudo -E restic -r "$RESTIC_REPO_B2"   key list     # find the old key's ID
sudo -E restic -r "$RESTIC_REPO_B2"   key remove <old-id>
# repeat for $RESTIC_REPO_LOCAL, then update RESTIC_PASSWORD in the sops file:
sops ctdev/component/configs/restic/hosts/ctpi01.sops.env
```

## Security model (why this is safe)

- Encrypted `*.sops.*` files are safe to commit, even to a public repo — without
  the age private key they're useless.
- **Never commit** `~/.config/sops/age/keys.txt` (it's git-ignored) or any
  plaintext `.env`. The restic backup also excludes the age key.
- Anyone with the age private key can read **all** node secrets — treat it like a
  root password. Its only off-device home should be your password manager.
