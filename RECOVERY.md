# RECOVERY.md — Homelab Disaster Recovery (ctpi01)

This is the step-by-step runbook for restoring the home Raspberry Pi (`ctpi01`)
and its services from backups. Follow it top to bottom for a full rebuild, or
jump to the scenario you need.

> **TL;DR of the model.** Recovery has two halves:
> 1. **Structure & config** comes from this **dotfiles repo** via `ctdev install …`
>    (compose files, Caddyfile, Pi-hole lists, systemd units — all version-controlled).
> 2. **Data & secrets** come from **restic backups** (Let's Encrypt certs,
>    Portainer/Beszel databases, Pi-hole history, and the `.env` files that hold
>    passwords/tokens — none of which are in git).
>
> You need **both** to come back to a working system.

---

## 0. What you MUST have before you can recover

There is now **one master secret** to protect off-device: the **age private
key**. Everything else — the restic repository password, the B2 keys, the
Cloudflare token, the Pi-hole password, the Beszel KEY/TOKEN — is stored
**encrypted in the dotfiles repo** (`*.sops.env` files) and is recoverable with
that key. So you only need two things:

| Item | Where it lives | Why it's needed |
|---|---|---|
| **age private key** | `~/.config/sops/age/keys.txt` (lost with the Pi) → **also in 1Password** | Decrypts every `*.sops.env` secret in the repo — including the restic repo password and B2 keys. **No age key = no recovery.** |
| GitHub access to `ConnerTechnology/dotfiles` | your account | Reinstall ctdev + decrypt the committed secrets |

> 🔐 **Do this now, before you ever need it:** copy the age private key into
> 1Password.
> ```bash
> cat ~/.config/sops/age/keys.txt        # save the whole file (incl. the "# public key:" line) in 1Password
> ```
> The age key is **not** in any backup or in git (by design). If you lose it
> *and* the Pi, the encrypted secrets are gone — so the 1Password copy is the
> safety net.

Encrypted secrets in the repo (decrypt with `sops -d <file>`):

| File | Decrypts to | Holds |
|---|---|---|
| `ctdev/component/configs/restic/hosts/ctpi01.sops.env` | `/etc/restic/restic.env` | restic password, B2 keyID/key, repo paths |
| `ctdev/component/configs/caddy/hosts/ctpi01.sops.env` | `~/caddy/.env` | domain, ACME email, Cloudflare token |
| `ctdev/component/configs/beszel/hosts/ctpi01.sops.env` | `~/beszel/.env` | Beszel agent KEY/TOKEN |
| `ctdev/component/configs/pihole/hosts/ctpi01.sops.env` | `~/pihole/.env` | Pi-hole admin password, TZ |

### What's in a backup
Each snapshot (host `ctpi01`, tag `homelab`) contains:

```
/home/ctadmin/caddy        Caddy stack + Caddyfile + .env (CF token)
/home/ctadmin/pihole       Pi-hole compose + .env + etc-pihole (gravity, history) + etc-dnsmasq.d
/home/ctadmin/portainer    Portainer compose
/home/ctadmin/beszel       Beszel compose + .env (agent KEY/TOKEN) + beszel_data
/var/lib/docker/volumes/caddy_caddy_data/_data         Let's Encrypt certs/account
/var/lib/docker/volumes/caddy_caddy_config/_data       Caddy autosave config
/var/lib/docker/volumes/portainer_portainer_data/_data Portainer users/settings/stacks
```

### The two repositories
| Name | Location | Use |
|---|---|---|
| **offsite (b2)** | `b2:ctpi01-backups:ctpi01` | Primary for disaster recovery (survives the house) |
| **local (usb)** | `/mnt/backup/restic/ctpi01` | Fast restores when the USB drive is attached |

---

## 1. Scenario A — Restore a single file or folder (most common)

You deleted/broke one file and the Pi is otherwise fine. Use the helper
(`restic-restore.sh` is installed by `ctdev install restic`):

```bash
# 1. See what snapshots exist (newest at the bottom)
sudo restic-restore.sh snapshots b2          # or: local

# 2. Look inside the latest snapshot
sudo restic-restore.sh ls latest b2 | less

# 3. Restore the whole latest snapshot into a scratch dir (nothing is overwritten)
sudo restic-restore.sh restore latest /tmp/restore b2

# 4. Copy out just what you need, e.g. the Caddyfile:
sudo cp /tmp/restore/home/ctadmin/caddy/Caddyfile ~/caddy/Caddyfile

# 5. Clean up
sudo rm -rf /tmp/restore
```

To restore only specific paths instead of everything, use restic directly:

```bash
set -a; source <(sudo cat /etc/restic/restic.env); set +a   # loads + exports repo/creds into THIS shell
sudo -E restic -r "$RESTIC_REPO_B2" restore latest \
  --target /tmp/restore \
  --include /home/ctadmin/pihole/etc-pihole
```

---

## 2. Scenario B — Recover one service's data on the existing machine

Example: Pi-hole's database got corrupted but the OS is fine.

```bash
# 1. Stop the stack so files aren't being written during the restore
docker compose -f ~/pihole/docker-compose.yml down

# 2. Restore that stack's data in place from the latest snapshot
set -a; source <(sudo cat /etc/restic/restic.env); set +a
sudo -E restic -r "$RESTIC_REPO_B2" restore latest --target / \
  --include /home/ctadmin/pihole

# 3. Bring it back up
docker compose -f ~/pihole/docker-compose.yml up -d
docker logs --tail 20 pihole
```

> `--target /` writes files back to their **original absolute paths**. Always
> stop the affected containers first so you don't restore over open databases.

For a Docker volume (e.g. Portainer's data):

```bash
docker compose -f ~/portainer/docker-compose.yml down
set -a; source <(sudo cat /etc/restic/restic.env); set +a
sudo -E restic -r "$RESTIC_REPO_B2" restore latest --target / \
  --include /var/lib/docker/volumes/portainer_portainer_data/_data
docker compose -f ~/portainer/docker-compose.yml up -d
```

---

## 3. Scenario C — Full bare-metal rebuild (dead SD card / new Pi)

This rebuilds the whole node from scratch. Estimated time: ~30–45 min.

### Step 1 — Flash and boot a fresh OS
1. Flash **Raspberry Pi OS Lite (64-bit)** to a new SD card (or SSD) with
   Raspberry Pi Imager. In the imager's settings, set:
   - hostname: `ctpi01`
   - username: `ctadmin`
   - enable SSH (your public key)
   - your Wi-Fi/locale if needed
2. Boot the Pi, then SSH in:
   ```bash
   ssh ctadmin@ctpi01.local        # or its LAN IP
   ```
3. Update the base system:
   ```bash
   sudo apt-get update && sudo apt-get -y upgrade
   ```

### Step 2 — Install ctdev
```bash
curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/install.sh | bash
# ensure ~/.local/bin is on PATH (the installer prints this if not):
export PATH="$HOME/.local/bin:$PATH"
ctdev --version
```

### Step 3 — Core components + join the tailnet
```bash
ctdev install docker tailscale
sudo tailscale up                  # authenticate in the browser link it prints
```
Log out and back in once (so your user picks up the `docker` group), then verify:
```bash
docker ps
```

### Step 4 — Restore the restic config from the encrypted repo
The restic password + B2 keys are committed (encrypted) in the repo. Restore the
**age private key** first (from 1Password), then decrypt them into place.

```bash
# 1. Put the age private key back (paste the value saved in 1Password):
mkdir -p ~/.config/sops/age
nano ~/.config/sops/age/keys.txt          # paste, save; must include the AGE-SECRET-KEY line
chmod 600 ~/.config/sops/age/keys.txt

# 2. Get the dotfiles repo (for the encrypted secrets + sops):
ctdev install sops
git clone https://github.com/ConnerTechnology/dotfiles.git ~/src/dotfiles

# 3. Decrypt the restic secrets straight into /etc/restic/restic.env:
sudo install -d -m 700 /etc/restic
sops -d ~/src/dotfiles/ctdev/component/configs/restic/hosts/$(hostname).sops.env \
  | sudo install -m 600 /dev/stdin /etc/restic/restic.env
```

Install restic + the backup/restore tooling and confirm you can read the repo:
```bash
ctdev install restic                               # auto-enables the timer now that config exists
set -a; source <(sudo cat /etc/restic/restic.env); set +a
sudo -E restic -r "$RESTIC_REPO_B2" snapshots      # should list your snapshots
```

> While you have the age key + repo, decrypt the other node secrets too (Caddy
> token, Pi-hole password, Beszel KEY/TOKEN) — see **[SECRETS.md](SECRETS.md)**.
> The stack `.env` files are also included in the restic backup, so Step 5 below
> restores them as well; decrypt manually only if you're rebuilding a stack
> before restoring its data.

### Step 5 — Restore all data in place
```bash
sudo restic-restore.sh restore-in-place latest b2
# type YES when prompted
```
This recreates `~/caddy`, `~/pihole`, `~/portainer`, `~/beszel` (including their
`.env` secrets) and the Docker volume data dirs (certs, Portainer/Beszel state).

Fix ownership of the restored home dirs (restic restores as root):
```bash
sudo chown -R ctadmin:ctadmin /home/ctadmin/caddy /home/ctadmin/pihole \
  /home/ctadmin/portainer /home/ctadmin/beszel
```

### Step 6 — Bring the stacks up (order matters: DNS first)
```bash
docker compose -f ~/pihole/docker-compose.yml up -d
docker compose -f ~/caddy/docker-compose.yml up -d --build   # caddy image builds locally
docker compose -f ~/portainer/docker-compose.yml up -d
docker compose -f ~/beszel/docker-compose.yml up -d
```

> The restored Docker volume data lives at `/var/lib/docker/volumes/<name>/_data`;
> compose adopts those dirs when it (re)creates the named volumes, so the certs
> and Portainer/Beszel databases come back automatically. If a volume comes up
> empty, restore it explicitly with the Scenario B volume command, then
> `up -d` again.

### Step 7 — Re-point DNS and verify
```bash
# Caddy frees :443 and wires the wildcard DNS record; re-run its config wizard:
ctdev configure caddy
```
Then check everything:
```bash
docker ps --format 'table {{.Names}}\t{{.Status}}'       # all Up / healthy
dig @127.0.0.1 -p 53 pi.hole +short                      # DNS resolves
curl -sk --resolve pihole.home.connertechnology.io:443:127.0.0.1 \
  -o /dev/null -w '%{http_code}\n' https://pihole.home.connertechnology.io/
```
In the Tailscale admin console, confirm the **Global Nameserver** points at this
node's (possibly new) Tailscale IP (Override DNS = on).

### Step 8 — Re-enable backups on the rebuilt node
`ctdev install restic` (Step 4) already enabled the daily timer since
`/etc/restic/` is populated. Confirm and take a fresh snapshot:
```bash
systemctl list-timers restic-backup.timer --no-pager
sudo systemctl start restic-backup.service       # run one now
journalctl -u restic-backup.service -n 30 --no-pager
```

You're back. 🎉

---

## 4. Restoring from the USB drive instead of B2

Identical to the above but pass `local` and make sure the drive is mounted:

```bash
mount | grep /mnt/backup || sudo mount /mnt/backup     # fstab mounts it by UUID
sudo restic-restore.sh snapshots local
sudo restic-restore.sh restore latest /tmp/restore local
```

If `/mnt/backup` isn't in `/etc/fstab` on a fresh machine, find the drive and add it:
```bash
lsblk -o NAME,SIZE,FSTYPE,UUID            # identify the partition + UUID
sudo mkdir -p /mnt/backup
echo "UUID=<uuid>  /mnt/backup  ext4  defaults,nofail,x-systemd.device-timeout=10s  0  2" | sudo tee -a /etc/fstab
sudo systemctl daemon-reload && sudo mount -a
```

---

## 5. Verify your backups regularly (don't wait for a disaster)

Once a month or so:

```bash
# Integrity check of the offsite repo (verifies structure; add --read-data for a full check)
sudo restic-restore.sh check b2

# Prove a restore actually works
sudo restic-restore.sh restore latest /tmp/verify b2
sudo ls -la /tmp/verify/home/ctadmin/caddy
sudo rm -rf /tmp/verify
```

Beszel shows `/mnt/backup` free space on the dashboard, and the
`restic-backup.service` status (`journalctl -u restic-backup.service`) tells you
whether the nightly run succeeded.

---

## 6. Command cheat sheet

```bash
sudo restic-restore.sh snapshots [b2|local]                  # list snapshots
sudo restic-restore.sh ls <snap|latest> [b2|local]           # list files in a snapshot
sudo restic-restore.sh restore <snap|latest> <dir> [b2|local]# restore into <dir> (safe)
sudo restic-restore.sh restore-in-place <snap|latest> [b2|local] # restore to original paths
sudo restic-restore.sh check [b2|local]                      # verify repo integrity
sudo systemctl start restic-backup.service                   # run a backup now
systemctl list-timers restic-backup.timer                    # when does it next run
journalctl -u restic-backup.service -n 50                    # last run's log
```

Raw restic (when the helper isn't available, e.g. mid-rebuild):
```bash
set -a; source <(sudo cat /etc/restic/restic.env); set +a
sudo -E restic -r "$RESTIC_REPO_B2" snapshots
sudo -E restic -r "$RESTIC_REPO_B2" restore latest --target /tmp/restore
```
