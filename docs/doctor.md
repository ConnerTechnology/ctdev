# Doctor

`ctdev doctor` diagnoses a machine and the network it sits on: hardware, OS,
security posture, DNS, routing, the internet path. It is the one command that
assumes nothing about the machine, because it is built for machines ctdev did
not set up — a family member's laptop, a client's desktop.

```bash
ctdev doctor                    # the report
ctdev doctor --network          # network and internet checks only
ctdev doctor --deep             # + vendor APIs, Wi-Fi scan, path trace
ctdev doctor --root             # prompt once for sudo so root-only checks run
ctdev doctor --report [path]    # also write a shareable Markdown report
ctdev doctor --redact           # mask SSID, MACs and public IP before sharing
ctdev doctor --strict           # exit non-zero on failure, for cron
```

## It only reads

Every check is read-only, and doctor changes nothing on the machine it is
pointed at. Root is never required: a check that needs it reports Skipped and
says which one. `--root` is how you unlock those — it asks for a password once,
up front — and it is off by default, because doctor is pointed at machines
nobody asked us to manage and demanding a stranger's password uninvited is not
the behavior it promises.

## Diagnose a machine without installing anything

For a machine you're only visiting — a family member's laptop, a client's
desktop. Downloads to a temp directory, runs the report, and deletes itself.
Nothing is installed, no PATH is changed, and sudo is never used.

```bash
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/ConnerTechnology/ctdev/main/install.sh | bash -s -- --doctor
```

```powershell
# Windows — `irm | iex` cannot pass arguments, so wrap it in a scriptblock
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/ConnerTechnology/ctdev/main/install.ps1))) -Doctor
```

## Networks, not just machines

Doctor reads the network the machine is on, and correlates: a fault has one root
cause, and everything downstream of it is noise, so the network verdicts are
layered and the first match wins. It reports on DNS resolution and DNSSEC
validation, whether a resolver reaches the root servers directly — a transparent
ISP DNS proxy answering in their place is what silently breaks both — routing,
and the path out.

`--deep` additionally reads vendor APIs for the unmanaged devices it cannot be
installed on:
UniFi, Synology, Proxmox. Those integrations are read-only by construction and
credentials are **never written to the machine being diagnosed**. Give it a
read-only UniFi key and it will report radar events, airtime and mesh uplinks:

```bash
CTDEV_UNIFI_API_KEY=<key> ctdev doctor --deep
ctdev doctor --deep --unifi https://10.2.2.1   # when it isn't the gateway
```

Create that key in the UniFi console under Settings → Control Plane →
Integrations. `--no-integrations` refuses to call a vendor API at all, even when
a credential is present.

## Sharing a report

`--report` writes the report as Markdown. `--redact` masks the SSID, MAC
addresses and the public IP first, so it can be pasted somewhere.
