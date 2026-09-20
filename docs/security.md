# Security

What ctdev does to a device, in full. The short version is in the README; this
page is the one to read before allowing ctdev onto a device you are responsible
for.

ctdev is Conner Technology's internal tool. It runs on our own devices today and
on clients' devices later. It is published so that a machine can install it, not
as a product with a support commitment.

## Privilege

ctdev runs as you. It escalates with `sudo`, for the steps that need it, and
asks for the password once at the point it is first needed rather than at
startup.

What needs root is the work that touches the machine rather than your home
directory: a package manager, a systemd unit, a file under `/etc` or
`/usr/local`, a firewall rule. Each component declares which of three cases it
is in — needs root only while being installed (the common case), does privileged
work on every run, or never needs root at all because it installs entirely
inside `$HOME` or through the Docker socket. ctdev asks for a password only when
something in the run is actually going to use it, so a container that forbids
escalation can still install everything that lives in `$HOME`.

Reaching root is best-effort. Where there is no `sudo`, where the environment
forbids escalation, or where nothing can type a password, ctdev warns and
carries on with the part of the run that never needed root; whatever genuinely
needed it fails with its own error.

## Where files land

| Path | What |
| --- | --- |
| `~/.local/bin/ctdev` | The binary, as the install script places it |
| `~/.config/ctdev/` | Configuration, including your own machine profiles |
| `~/.local/state/ctdev/` | State ctdev keeps between runs |
| `~/.cache/ctdev/` | Disposable working files, when a run needs any |

Those are what `uninstall.sh` removes. Individual components put their own
files where that software normally lives — a package manager's paths, a
`docker-compose.yml` under `$HOME`, a systemd unit under `/etc`. Each
component's page or `--help` says which.

The XDG variables are honored: set `XDG_CONFIG_HOME` or `XDG_STATE_HOME` and
ctdev follows them, as the uninstaller does for `XDG_CACHE_HOME`.

## Secrets

**No secret is ever stored in this repository.** Every secret ctdev handles —
a Cloudflare API token, a restic repository password and its backend
credentials, a Pi-hole password, a Beszel key, a mailbox password, a Claude
token — is entered at its `configure` step and written only to the device that
needs it, with a file mode that restricts it to its owner.

The consequence is deliberate: lose a device and you re-enter its secrets from
your password manager rather than recovering them from somewhere central. A
restic restore brings back the rendered `.env` files a node had, which is why
the restic repository password itself belongs in your password manager —
restic cannot restore its own credentials.

Secrets are not passed as command-line flags where a wizard can prompt for them
instead, because flags land in shell history and in `ps` output.

## Diagnostics are read-only

`ctdev doctor` changes nothing on the device it is pointed at. Every check
reads; root is never required, and a check that would need it reports Skipped
and says so. No telemetry is sent, and no data leaves the machine beyond the
diagnostic probes themselves.

The vendor integrations that `--deep` uses — UniFi, Synology, Proxmox — are
read-only by construction: the interface they implement has no action method.
Their credentials come from a flag, an environment variable, or a one-shot
prompt held in memory, are sent only to private addresses, and are **never
written to the device being diagnosed**. A generated report drops
secret-looking values before rendering, and `--redact` additionally masks the
SSID, MAC addresses and the public IP.

## Verifying a release

**Releases ship a checksum file and are not signed.** Each release publishes
`SHA256SUMS` alongside the binaries, and `install.sh` verifies the download
against it before moving it into place — failing closed if the sums file, the
entry for this platform, or `sha256sum` itself is missing.

Be clear about what that does and does not prove. It proves the file you have is
the file the release page lists, so a truncated or corrupted download is caught.
It does not prove who built it: anyone who could replace a binary on the release
page could replace the checksum beside it. Signed provenance — an attestation
that ties a binary to this repository's CI — is on the [roadmap](roadmap/README.md)
and does not exist yet. Nothing here claims a verification you cannot perform
today.

## What ctdev does not do

- It does not configure network gear. It diagnoses any network, and it builds
  the machines that serve one, but it does not log into a router, a switch or an
  access point to change settings.
- It does not report anywhere. There is no central service, no telemetry, and no
  remote control channel; a machine running ctdev talks to package repositories
  and to the services you configured, and to nothing of ours.
- It does not manage devices remotely. That is the [fleet](roadmap/fleet.md), and
  it is an idea, not a feature.
