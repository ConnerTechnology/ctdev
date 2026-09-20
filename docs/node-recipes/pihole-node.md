# Pi-hole / Homelab Node

There's no "homelab mode" — you compose a node from individual components and
`configure` categories. To turn a freshly flashed **Raspberry Pi OS Lite** (or
Ubuntu/Debian) box into a Pi-hole node behind a Caddy reverse proxy serving
`https://*.<your-domain>` with a Let's Encrypt **wildcard** cert (Cloudflare
DNS-01, nothing exposed to the internet):

```bash
# after SSHing in and installing ctdev — see ../install.md:
ctdev apply pihole-node                  # ← the whole block below in one command,
                                         #    then follow its printed next steps
# — or step by step: —
ctdev install zsh git tailscale          # whatever base tools you want
sudo tailscale up                        # join the tailnet
ctdev install pihole                     # network-wide DNS ad blocker
ctdev configure pihole                   # upstreams, listening mode, blocking, host resolver
ctdev install docker                     # caddy needs docker
ctdev configure caddy --domain example.com --acme-email you@example.com
                                         # prompts for the Cloudflare token (masked); or pass it
                                         # via env: CF_API_TOKEN=<token> ctdev configure caddy ...
                                         # (avoid --cf-token — flags land in shell history and ps)
ctdev install caddy                      # deploy the proxy stack + bring it up
ctdev install portainer                  # optional: Docker management web UI
ctdev install beszel                      # optional: server/container monitoring
ctdev install restic                      # optional: daily restic backups
ctdev configure restic                    # set repo + credentials, init, enable timer
```

`ctdev install portainer` brings up Portainer CE (a web UI to view and manage
the host's containers, images, volumes, and compose stacks). Caddy serves it at
`https://portainer.<domain>`; it's also reachable directly at `https://<node>:9443`.
Create the admin user on first login. It mounts the Docker socket, so keep it
off any public network.

`ctdev install beszel` brings up Beszel (lightweight server/container
monitoring — a hub web UI plus an agent that reports this host's CPU, memory,
disk, network, temps, and per-container stats with alerting). Caddy serves the
hub at `https://beszel.<domain>`. The install starts the hub first; create the
admin user, click "Add System", put the issued KEY/TOKEN in `~/beszel/.env`,
then re-run `ctdev install beszel` to start the agent. Keep it off any public
network (Tailscale only).

`ctdev install restic` installs restic, a daily backup timer, and the backup/restore
helper scripts. Then `ctdev configure restic` prompts for the repository (any restic
backend — Backblaze B2, S3, SFTP, or a local/USB path), backend credentials, and a
repository password (it can generate one), writes `/etc/restic/restic.env` (root-only,
never committed), seeds default exclude patterns, runs `restic init`, and enables the
timer. Snapshots are tagged with the hostname, so each machine backs up to its own repo
and sees only its own snapshots. Backups are **opt-in** — nothing is snapshotted until you
choose what to include with `ctdev backup paths`, a local web UI that browses the
filesystem with folder sizes and include/exclude buttons. (Includes are the trees to back
up; excludes carve junk out of them, e.g. include `~/Repos`, exclude `**/node_modules`.) `ctdev backup now` snapshots immediately; `ctdev backup
snapshots` lists them; `ctdev restore …` restores — **see [RECOVERY.md](../../RECOVERY.md)
for the complete disaster-recovery runbook.**

Secrets are **never stored in the repo**. Each (Cloudflare token, restic repo password
+ backend keys, Beszel KEY/TOKEN, Pi-hole password) is entered at its `configure` step
and stored only on the host; if lost, you reconfigure. restic backs up the rendered
`~/<svc>/.env` files, so a restore brings them back — keep the restic repo password
itself in your password manager, since restic can't restore its own credentials.

`ctdev configure caddy` writes `~/caddy/.env` (mode 600) and, when Pi-hole is
present, frees port 443 and points `*.<domain>` at this node's Tailscale IP. Then
set that Tailscale IP as a Global Nameserver (Override on) in the Tailscale admin
console, and `sudo pihole setpassword` for the admin UI.

**Say yes to "Pi-hole host resolver" in `ctdev configure pihole`.** A node that
accepts Tailscale DNS resolves through MagicDNS, whose nameserver is its own
Pi-hole — so while the container is down the host can't pull the image to fix
it or reach its backups. The setting points the host at Pi-hole on loopback
with Quad9 as an instant fallback, and forwards tailnet (MagicDNS) names
through Pi-hole so they keep resolving on the node and, as a side effect, on
every LAN device. `ctdev doctor` then checks that the resolver validates DNSSEC
and that Unbound reaches the root servers directly (an ISP transparent DNS
proxy answering in their place is what silently breaks both).

**Skip `ctdev configure ufw` on a DNS/proxy host** — UFW's default-deny blocks
DNS (53) and the proxy (80/443) unless you open those ports first.

## Pi-hole lists in git

Every domain you allow, deny, or match with a regex — and every adlist you
subscribe to — lives in Pi-hole's `gravity.db` and nowhere else. restic backs
that file up, but a snapshot won't tell you *why* `stats.gc.apple.com` is on the
allowlist. So the lists are also kept as text, in
`ctdev/component/configs/pihole/lists.toml`: one entry per line with its comment
and its group, embedded in the binary.

```bash
ctdev pihole sync --dry-run   # what differs between the file and this Pi-hole
ctdev pihole sync             # pick what to apply
ctdev pihole export           # record this Pi-hole's lists into lists.toml
```

`sync` opens a picker, grouped by list. **Additions and updates are checked;
removals are not** — an entry that is on the Pi-hole but not in the file is
offered for deletion *unchecked*, so pressing Enter keeps it. That is the case
that matters: a domain someone allowed from the query log in the web UI is not
silently undone by a sync. When you leave removals unchecked, sync tells you to
run `export` and commit, which is how those entries get recorded.

Applying is one SQL transaction against `gravity.db`, followed by
`pihole reloadlists` — or a full `pihole -g` gravity rebuild only when an adlist
changed, since that re-downloads every list and is slow on a Pi.

**Adding a service:** add the container to `~/caddy/docker-compose.yml` and a
route snippet in `~/caddy/sites/<svc>.caddy`, then `ctdev install caddy` (or
`sudo docker compose -f ~/caddy/docker-compose.yml up -d`). The wildcard DNS +
cert already cover it.

**Secrets.** A node's secrets are entered into its `configure` wizard (or `.env`) and
live only on that host — nothing secret is committed to the repo. Restoring a node from
its restic backup brings the `.env` files back; standing up a brand-new node means
re-entering them from your password manager. **Never commit a secret.**
