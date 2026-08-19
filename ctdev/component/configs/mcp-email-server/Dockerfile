# mcp-email-server from PyPI rather than ghcr.io/ai-zerolab/mcp-email-server.
# Upstream's published container images stop at 0.16.0 (44 tags, no 1.x) while
# the Python package is on 1.x — so the image line is a major version behind and
# looks unmaintained. 1.x is also the version that makes headless setup a
# first-class path: `account add --password-stdin` (never argv) and `--json` on
# every command, which is what `ctdev configure mcp-email-server` drives.
FROM python:3.12-slim

# Pinned: a silent upgrade of the process holding mail credentials is not
# something to discover after the fact. Bump deliberately after reading the
# upstream release notes, then re-run `ctdev install mcp-email-server` (compose
# rebuilds with --pull).
#
# BUMPING THIS PIN REQUIRES RE-CHECKING THE TOOL LIST.
# ConnerTechnology/AI blocks dangerous email tools by EXACT NAME in
# config/settings.json (permissions.deny), and one of its agents runs an
# allowlist. A tool that is added or renamed by a version bump silently escapes
# both. After changing this version, run tools/list against the server and
# reconcile it with that deny list; the surface for the current pin is recorded
# in the doc comment at the top of ctdev/component/mcp_email_server.go.
RUN pip install --no-cache-dir mcp-email-server==1.4.1

ENTRYPOINT ["mcp-email-server"]
CMD ["streamable-http"]
